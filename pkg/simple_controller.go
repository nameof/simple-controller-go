package controller

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	apiNetworkV1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	coreLister "k8s.io/client-go/listers/core/v1"
	networkingLister "k8s.io/client-go/listers/networking/v1"
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
	ingressLister networkingLister.IngressLister
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
		DeleteFunc: c.ServiceDeleted,
	})

	ingressInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		DeleteFunc: c.IngressDeleted,
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
		c.syncIngerss(service)
	} else {
		c.deleteIngerss(namespace, name)
	}
}

func (c *SimpleController) syncIngerss(service *corev1.Service) {
	log.Printf("check and create/update ingress")
	ingress := c.getIngressByService(service.GetNamespace(), service.GetName())
	if ingress != nil {
		log.Printf("ingress %s/%s already exists\n", ingress.GetNamespace(), ingress.GetName())
		return
	}
	c.createIngress(service)
}

func (c *SimpleController) createIngress(service *corev1.Service) {
	ingressClass := "nginx"
	pathTypePrefix := apiNetworkV1.PathTypePrefix
	ingress := &apiNetworkV1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "networking.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      service.GetName() + "-ingress",
			Namespace: service.GetNamespace(),
			Annotations: map[string]string{
				ownerServiceNameKey: service.GetName(),
			},
		},
		Spec: apiNetworkV1.IngressSpec{
			IngressClassName: &ingressClass,
			Rules: []apiNetworkV1.IngressRule{
				{
					Host: "simple-controller.nameof.com",
					IngressRuleValue: apiNetworkV1.IngressRuleValue{
						HTTP: &apiNetworkV1.HTTPIngressRuleValue{
							Paths: []apiNetworkV1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathTypePrefix,
									Backend: apiNetworkV1.IngressBackend{
										Service: &apiNetworkV1.IngressServiceBackend{
											Name: service.GetName(),
											Port: apiNetworkV1.ServiceBackendPort{
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
	ingress.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(service, corev1.SchemeGroupVersion.WithKind("Service")),
	}
	_, err := c.client.NetworkingV1().Ingresses(service.GetNamespace()).Create(context.Background(), ingress, metav1.CreateOptions{})
	if err != nil {
		log.Printf("error create ingress: %s\n", err)
		return
	}
	log.Printf("ingress %s/%s created\n", ingress.GetNamespace(), ingress.GetName())
}

func (c *SimpleController) deleteIngerss(namespace string, serviceName string) {
	log.Printf("delete ingress")
	ingress := c.getIngressByService(namespace, serviceName)
	if ingress != nil {
		log.Printf("delete ingress %s\n", ingress.GetName())
		err := c.client.NetworkingV1().Ingresses(namespace).Delete(context.TODO(), ingress.GetName(), metav1.DeleteOptions{})
		if err != nil {
			log.Printf("error delete ingress %s %s\n", ingress.GetName(), err)
		}
	}
}

func (c *SimpleController) ServiceDeleted(obj interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	log.Printf("service delete：%s\n", key)
	// 无需手动删除ingress，k8s ownerReferences自动级联删除
}

func (c *SimpleController) getIngressByService(namespace string, name string) *apiNetworkV1.Ingress {
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

// IngressDeleted 检查是否需要恢复Ingress
func (c *SimpleController) IngressDeleted(obj interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	log.Printf("ingress delete：%s\n", key)

	namespace, _, _ := cache.SplitMetaNamespaceKey(key)
	ingress := obj.(*apiNetworkV1.Ingress)
	serviceName, ok := ingress.GetAnnotations()[ownerServiceNameKey]
	// 非本controller管理
	if !ok {
		log.Printf("ignore deleted ingress：%s\n", key)
		return
	}

	service, err := c.serviceLister.Services(namespace).Get(serviceName)
	if err != nil {
		log.Printf("error get service：%s\n", err)
		return
	}
	value, ok := service.GetAnnotations()[exposeIngressKey]
	if ok && "true" == value {
		log.Printf("recovrey ingress：%s\n", key)
		c.createIngress(service)
	} else {
		log.Printf("service not enable ingress：%s\n", key)
	}
}
