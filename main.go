package main

import (
	"fmt"
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
	"log"
	"kmodules.xyz/resource-metadata/pkg/graph"
	"path/filepath"
	"strings"
)

type ObjectLocator struct {
	Start *ObjectRef                     `json:"start"`
	Path  []v1alpha1.ResourceConnection `json:"connections,omitempty"`
}

type ObjectRef struct {
	Target       metav1.TypeMeta       `json:"target"`
	// Namespace always same as Workflow
	Selector     *metav1.LabelSelector `json:"selector,omitempty"`
	NameTemplate string                `json:"nameTemplate,omitempty"`
}

func Process(f dynamicfactory.Factory, r discovery.ResourceMapper, locator *ObjectLocator) (*unstructured.Unstructured, []*v1alpha1.Edge, error) {
	src, err := Locate(f, r, locator.Start)
	if err != nil {
		return nil, nil, err
	}

	finder := graph.ObjectFinder{
		f: f,
		r: r,
	}
	finder.List()

	// f.ForResource(gvr).Get()

	return nil, nil, nil
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
