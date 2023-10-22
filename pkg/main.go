package main

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		log.Println("fallback to InClusterConfig")
		inClusterConfig, err := rest.InClusterConfig()
		if err != nil {
			log.Println("cannot get config")
			panic(err)
		}
		config = inClusterConfig
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Println("cannot get clientset")
		panic(err)
	}

	controller := NewSimpleController(clientset)
	controller.Run()
}
