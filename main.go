package main

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"log"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

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
