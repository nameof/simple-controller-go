package main

import (
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"log"
)

type SimpleController struct {
	client *kubernetes.Clientset
}

func NewSimpleController(client *kubernetes.Clientset) *SimpleController {
	return &SimpleController{
		client: client,
	}
}

func (c *SimpleController) Run() {
	informerFactory := informers.NewSharedInformerFactory(c.client, 0)
	serviceInformer := informerFactory.Core().V1().Services()
	ingressInformer := informerFactory.Networking().V1().Ingresses()

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ServiceAdd,
		UpdateFunc: ServiceUpdate,
		DeleteFunc: ServiceDelete,
	})

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// TODO 处理Ingress更新事件
		DeleteFunc: IngressDelete,
	})
	stopChan := make(chan struct{})
	informerFactory.Start(stopChan)
	log.Println("controller started!")
	<-stopChan
}

func ServiceAdd(obj interface{}) {
	log.Printf("ServiceAdd：%s\n", obj)
}

func ServiceUpdate(old interface{}, updated interface{}) {
	log.Printf("ServiceUpdate：%s\n", updated)
}

func ServiceDelete(obj interface{}) {
	log.Printf("ServiceDelete：%s\n", obj)
}

func IngressDelete(obj interface{}) {
	// 检查是否需要恢复Ingress
	log.Printf("IngressDelete：%s\n", obj)
}
