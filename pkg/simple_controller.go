package main

import (
	"context"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	coreLister "k8s.io/client-go/listers/core/v1"
	networkingV1 "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"log"
)

const (
	exposeIngressKey    = "simple-controller.nameof.github.com/exposeIngress"
	ownerServiceNameKey = "simple-controller.nameof.github.com/ownerServiceName"
)

type SimpleController struct {
	client        *kubernetes.Clientset
	factory       informers.SharedInformerFactory
	ingressLister networkingV1.IngressLister
	serviceLister coreLister.ServiceLister
}

func NewSimpleController(client *kubernetes.Clientset, factory informers.SharedInformerFactory) *SimpleController {
	serviceInformer := factory.Core().V1().Services()
	ingressInformer := factory.Networking().V1().Ingresses()
	c := &SimpleController{
		client:        client,
		factory:       factory,
		ingressLister: ingressInformer.Lister(),
		serviceLister: serviceInformer.Lister(),
	}

	serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.ServiceAdd,
		UpdateFunc: c.ServiceUpdate,
		DeleteFunc: c.ServiceDelete,
	})

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// TODO 处理Ingress更新事件
		DeleteFunc: IngressDelete,
	})
	return c
}

func (c *SimpleController) Run() {
	stopChan := make(chan struct{})

	c.factory.Start(stopChan)
	c.factory.WaitForCacheSync(stopChan)

	log.Println("controller started!")
}

func (c *SimpleController) ServiceAdd(obj interface{}) {
	c.ServiceAddOrUpdate(obj)
}

func (c *SimpleController) ServiceUpdate(old interface{}, updated interface{}) {
	c.ServiceAddOrUpdate(updated)
}

func (c *SimpleController) ServiceAddOrUpdate(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		panic(err)
	}
	log.Printf("ServiceAdd/ServiceUpdate：%s\n", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		panic(err)
	}

	service, err := c.serviceLister.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Println("service not exists, skip...")
			return
		}
		return
	}

	value, ok := service.GetAnnotations()[exposeIngressKey]
	if ok && "true" == value {
		c.syncIngerss(namespace, name)
		return
	}
	log.Printf("ignore service: %s\n", key)
}

func (c *SimpleController) syncIngerss(namespace string, name string) {
	log.Printf("check and create/update ingress")
	// TODO
}

func (c *SimpleController) ServiceDelete(obj interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	log.Printf("ServiceDelete：%s\n", key)
	namespace, name, _ := cache.SplitMetaNamespaceKey(key)
	ingresses, err := c.ingressLister.Ingresses(namespace).List(labels.Everything())
	if err != nil {
		log.Printf("error List All Ingress %s\n", err)
		return
	}
	for _, ingress := range ingresses {
		value, ok := ingress.GetAnnotations()[ownerServiceNameKey]
		if ok && value == name {
			log.Printf("delete ingress %s\n", ingress.GetName())
			err = c.client.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), ingress.GetName(), metav1.DeleteOptions{})
			if err != nil {
				log.Printf("error delete ingress %s %s\n", ingress.GetName(), err)
				continue
			}
		}
	}
}

func IngressDelete(obj interface{}) {
	// 检查是否需要恢复Ingress
	log.Printf("IngressDelete：%s\n", obj)
}
