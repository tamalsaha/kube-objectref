/*
Copyright AppsCode Inc. and Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package discovery

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	"k8s.io/klog/v2"
)

func APIResourceForGVK(client discovery.DiscoveryInterface, gvk schema.GroupVersionKind) (metav1.APIResource, error) {
	resourceList, err := client.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if discovery.IsGroupDiscoveryFailedError(err) {
		klog.Errorf("Skipping failed API Groups: %v", err)
	} else if err != nil {
		return metav1.APIResource{}, err
	}
	var resources []metav1.APIResource
	for _, resource := range resourceList.APIResources {
		if resource.Kind == gvk.Kind { // match kind
			resource.Group = gvk.Group
			resource.Version = gvk.Version
			resources = append(resources, resource)
		}
	}
	resources = FilterAPISubResources(resources) // ignore sub-resources
	switch len(resources) {
	case 0:
		return metav1.APIResource{}, fmt.Errorf("APIResource not found for gvk=%v", gvk)
	case 1:
		return resources[0], nil
	default:
		var matches []schema.GroupVersionResource
		for _, resource := range resources {
			matches = append(matches, schema.GroupVersionResource{
				Group:    resource.Group,
				Version:  resource.Version,
				Resource: resource.Name,
			})
		}
		return metav1.APIResource{}, &meta.AmbiguousResourceError{PartialResource: matches[0], MatchingResources: matches}
	}
}

func ResourceForGVK(client discovery.DiscoveryInterface, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	return NewResourceMapper(NewRestMapper(client)).GVR(gvk)
}

func FilterAPISubResources(resources []metav1.APIResource) []metav1.APIResource {
	var filtered []metav1.APIResource
	for _, res := range resources {
		if !strings.ContainsRune(res.Name, '/') {
			filtered = append(filtered, res)
		}
	}
	return filtered
}

func FilterSubResources(gvrs []schema.GroupVersionResource) []schema.GroupVersionResource {
	var filtered []schema.GroupVersionResource
	for _, res := range gvrs {
		if !strings.ContainsRune(res.Resource, '/') {
			filtered = append(filtered, res)
		}
	}
	return filtered
}

func NewRestMapper(client discovery.DiscoveryInterface) meta.RESTMapper {
	return restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(client))
}

type ResourceMapper interface {
	GVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error)
	TypeMeta(gvr schema.GroupVersionResource) (metav1.TypeMeta, error)
	GVK(gvr schema.GroupVersionResource) (schema.GroupVersionKind, error)
	IsNamespaced(gvr schema.GroupVersionResource) (bool, error)
	IsPreferred(gvr schema.GroupVersionResource) (bool, error)
	Preferred(gvr schema.GroupVersionResource) (schema.GroupVersionResource, error)
}

type resourcemapper struct {
	mapper meta.RESTMapper
}

var _ ResourceMapper = &resourcemapper{}

func NewResourceMapper(mapper meta.RESTMapper) ResourceMapper {
	return &resourcemapper{mapper: mapper}
}

func (m *resourcemapper) GVR(gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	mapping, err := m.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return mapping.Resource, nil
}

func (m *resourcemapper) TypeMeta(gvr schema.GroupVersionResource) (metav1.TypeMeta, error) {
	gvk, err := m.mapper.KindFor(gvr)
	if err != nil {
		return metav1.TypeMeta{}, err
	}
	return metav1.TypeMeta{
		APIVersion: gvr.GroupVersion().String(),
		Kind:       gvk.Kind,
	}, nil
}

func (m *resourcemapper) GVK(gvr schema.GroupVersionResource) (schema.GroupVersionKind, error) {
	gvk, err := m.mapper.KindFor(gvr)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return gvk, nil
}

func (m *resourcemapper) IsNamespaced(gvr schema.GroupVersionResource) (bool, error) {
	gvk, err := m.mapper.KindFor(gvr)
	if err != nil {
		return false, err
	}
	mapping, err := m.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, err
	}
	return mapping.Scope == meta.RESTScopeNamespace, nil
}

func (m *resourcemapper) IsPreferred(gvr schema.GroupVersionResource) (bool, error) {
	gvrs, err := m.mapper.ResourcesFor(gvr.GroupResource().WithVersion(""))
	if err != nil {
		return false, err
	}
	return gvrs[0] == gvr, nil
}

func (m *resourcemapper) Preferred(gvr schema.GroupVersionResource) (schema.GroupVersionResource, error) {
	gvrs, err := m.mapper.ResourcesFor(gvr.GroupResource().WithVersion(""))
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return gvrs[0], nil
}
