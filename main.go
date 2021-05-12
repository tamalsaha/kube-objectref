package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"kmodules.xyz/client-go/discovery"
	dynamicfactory "kmodules.xyz/client-go/dynamic/factory"
	"kmodules.xyz/resource-metadata/apis/meta/v1alpha1"
	"kmodules.xyz/resource-metadata/pkg/graph"
)

type ObjectLocator struct {
	Start    *ObjectRef     `json:"start"`
	Paths    []string       `json:"paths"` // sequence of DirectedEdge names
	EdgeList []DirectedEdge `json:"edge_list"`
}

type DirectedEdge struct {
	Name       string
	Src        metav1.TypeMeta
	Dst        metav1.TypeMeta
	Connection v1alpha1.ResourceConnectionSpec
}

type ObjectRef struct {
	Target metav1.TypeMeta `json:"target"`
	// Namespace always same as Workflow
	Selector     *metav1.LabelSelector `json:"selector,omitempty"`
	NameTemplate string                `json:"nameTemplate,omitempty"`
}

func Process(f dynamicfactory.Factory, r discovery.ResourceMapper, locator *ObjectLocator) (*unstructured.Unstructured, error) {
	src, err := Locate(f, r, locator.Start)
	if err != nil {
		return nil, err
	}

	m := make(map[string]*DirectedEdge)
	for i, entry := range locator.EdgeList {
		m[entry.Name] = &locator.EdgeList[i]
	}

	from := locator.Start.Target
	edges := make([]*graph.Edge, 0, len(locator.Paths))
	for _, path := range locator.Paths {
		e, ok := m[path]
		if !ok {
			return nil, fmt.Errorf("path %s not found in edge list", path)
		}

		srcGVR, err := r.GVR(schema.FromAPIVersionAndKind(e.Src.APIVersion, e.Src.Kind))
		if err != nil {
			return nil, err
		}
		dstGVR, err := r.GVR(schema.FromAPIVersionAndKind(e.Dst.APIVersion, e.Dst.Kind))
		if err != nil {
			return nil, err
		}
		if e.Src == from {
			edges = append(edges, &graph.Edge{
				Src:        srcGVR,
				Dst:        dstGVR,
				W:          0,
				Connection: e.Connection,
				Forward:    true,
			})
			from = e.Dst
		} else if e.Dst == from {
			edges = append(edges, &graph.Edge{
				Src:        dstGVR,
				Dst:        srcGVR,
				W:          0,
				Connection: e.Connection,
				Forward:    false,
			})
			from = e.Src
		} else {
			return nil, fmt.Errorf("edge %s has no connection with resource %v", path, from)
		}
	}

	finder := graph.ObjectFinder{
		Factory: f,
		Mapper:  r,
	}
	objects, err := finder.List(src, edges)
	if err != nil {
		return nil, err
	}

	switch len(objects) {
	case 0:
		last := edges[len(edges)-1]
		return nil, apierrors.NewNotFound(last.Dst.GroupResource(), "")
	case 1:
		return objects[0], nil
	default:
		names := make([]string, 0, len(objects))
		for _, obj := range objects {
			name, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				return nil, err
			}
			names = append(names, name)
		}
		last := edges[len(edges)-1]
		return nil, fmt.Errorf("multiple matching %v objects found %s", last.Dst, strings.Join(names, ","))
	}
}

func Locate(f dynamicfactory.Factory, r discovery.ResourceMapper, ref *ObjectRef) (*unstructured.Unstructured, error) {
	gvk := schema.FromAPIVersionAndKind(ref.Target.APIVersion, ref.Target.Kind)
	gvr, err := r.GVR(gvk)
	if err != nil {
		return nil, err
	}
	if ref.Selector == nil {
		sel, err := metav1.LabelSelectorAsSelector(ref.Selector)
		if err != nil {
			return nil, err
		}
		objects, err := f.ForResource(gvr).List(sel)
		if err != nil {
			return nil, err
		}
		switch len(objects) {
		case 0:
			return nil, apierrors.NewNotFound(gvr.GroupResource(), "")
		case 1:
			return objects[0], nil
		default:
			names := make([]string, 0, len(objects))
			for _, obj := range objects {
				name, err := cache.MetaNamespaceKeyFunc(obj)
				if err != nil {
					return nil, err
				}
				names = append(names, name)
			}
			return nil, fmt.Errorf("multiple matching %v objects found %s", gvr, strings.Join(names, ","))
		}
	}

	// TODO: convert name template to name
	object, err := f.ForResource(gvr).Get(ref.NameTemplate)
	if err != nil {
		return nil, err
	}
	return object, nil
}

func main() {
	masterURL := ""
	kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
	if err != nil {
		log.Fatalf("Could not get Kubernetes config: %s", err)
	}

	client := kubernetes.NewForConfigOrDie(config)

	var mapper meta.RESTMapper
	mapper = restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(client.Discovery()))

	gvr := schema.GroupVersionResource{
		Group:    "admissionregistration.k8s.io",
		Version:  "",
		Resource: "mutatingwebhookconfigurations",
	}
	gvrs, err := mapper.ResourcesFor(gvr)
	if err != nil {
		panic(err)
	}
	fmt.Println(gvrs)
}
