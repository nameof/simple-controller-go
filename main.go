package main

import (
	controller "github.com/nameof/simple-controller-go/pkg"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"time"
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		log.Println("fallback to InClusterConfig")
		config, err = rest.InClusterConfig()
		if err != nil {
			log.Println("cannot get config")
			panic(err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Println("cannot get clientset")
		panic(err)
	}

	factory := informers.NewSharedInformerFactory(clientset, 0)
	controller := controller.NewSimpleController(clientset, factory)
	controller.Run()

	wait.Until(func() {}, time.Second, wait.NeverStop)
}
