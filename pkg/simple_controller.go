package main

import (
	"context"
	v1 "k8s.io/api/networking/v1"
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
	c.serviceAddOrUpdate(obj)
}

func (c *SimpleController) ServiceUpdate(old interface{}, updated interface{}) {
	c.serviceAddOrUpdate(updated)
}

func (c *SimpleController) serviceAddOrUpdate(obj interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	log.Printf("service add/update：%s\n", key)

	namespace, name, _ := cache.SplitMetaNamespaceKey(key)
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
	} else {
		c.deleteIngerss(namespace, name)
	}
}

func (c *SimpleController) syncIngerss(namespace string, name string) {
	log.Printf("check and create/update ingress")
	ingress := c.getIngressByService(namespace, name)
	if ingress != nil {
		log.Printf("ingress %s/%s already exists\n", ingress.GetNamespace(), ingress.GetName())
		return
	}

	// TODO创建Ingress
	ingressClass := "nginx"
	pathTypePrefix := v1.PathTypePrefix
	ingress = &v1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-ingress",
			Namespace: namespace,
			Annotations: map[string]string{
				ownerServiceNameKey: name,
			},
		},
		Spec: v1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []v1.IngressRule{
				{
					Host: "simple-controller.nameof.com",
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathTypePrefix,
									Backend: v1.IngressBackend{
										Service: &v1.IngressServiceBackend{
											Name: "http",
											Port: v1.ServiceBackendPort{
												Number: 80,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	_, err := c.client.NetworkingV1().Ingresses(namespace).Create(context.Background(), ingress, metav1.CreateOptions{})
	if err != nil {
		log.Printf("error create ingress: %s\n", err)
		return
	}
	log.Printf("ingress %s/%s created\n", ingress.GetNamespace(), ingress.GetName())
}

func (c *SimpleController) deleteIngerss(namespace string, name string) {
	log.Printf("delete ingress")
	ingress := c.getIngressByService(namespace, name)
	if ingress != nil {
		log.Printf("delete ingress %s\n", ingress.GetName())
		err := c.client.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), ingress.GetName(), metav1.DeleteOptions{})
		if err != nil {
			log.Printf("error delete ingress %s %s\n", ingress.GetName(), err)
		}
	}
}

func (c *SimpleController) ServiceDelete(obj interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	log.Printf("service delete：%s\n", key)
	namespace, name, _ := cache.SplitMetaNamespaceKey(key)
	c.deleteIngerss(namespace, name)
}

func (c *SimpleController) getIngressByService(namespace string, name string) *v1.Ingress {
	ingresses, err := c.ingressLister.Ingresses(namespace).List(labels.Everything())
	if err != nil {
		log.Printf("error list all ingress %s\n", err)
		return nil
	}

	for _, ingress := range ingresses {
		value, ok := ingress.GetAnnotations()[ownerServiceNameKey]
		if ok && value == name {
			return ingress
		}
	}
	return nil
}

func IngressDelete(obj interface{}) {
	// 检查是否需要恢复Ingress
	log.Printf("IngressDelete：%s\n", obj)
}
