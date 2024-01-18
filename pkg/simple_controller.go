package controller

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	apiNetworkV1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	coreLister "k8s.io/client-go/listers/core/v1"
	networkingLister "k8s.io/client-go/listers/networking/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"log"
	"time"
)

const (
	exposeIngressKey = "simple-controller.nameof.github.com/exposeIngress"
	workNum          = 5
)

type SimpleController struct {
	client        *kubernetes.Clientset
	factory       informers.SharedInformerFactory
	ingressLister networkingLister.IngressLister
	serviceLister coreLister.ServiceLister
	queue         workqueue.Interface
}

func NewSimpleController(client *kubernetes.Clientset, factory informers.SharedInformerFactory) *SimpleController {
	serviceInformer := factory.Core().V1().Services()
	ingressInformer := factory.Networking().V1().Ingresses()
	c := &SimpleController{
		client:        client,
		factory:       factory,
		ingressLister: ingressInformer.Lister(),
		serviceLister: serviceInformer.Lister(),
		queue:         workqueue.New(),
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

func (c *SimpleController) Run(stopChan <-chan struct{}) {
	c.factory.Start(stopChan)
	c.factory.WaitForCacheSync(stopChan)

	for i := 0; i < workNum; i++ {
		go wait.Until(c.doWork, time.Second, stopChan)
	}
	log.Println("controller started!")
}

func (c *SimpleController) ServiceAdd(obj interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	c.queue.Add(key)
}

func (c *SimpleController) ServiceUpdate(old interface{}, updated interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(updated)
	c.queue.Add(key)
}

func (c *SimpleController) doWork() {
	for c.process() {

	}
}

func (c *SimpleController) process() bool {
	item, shutdown := c.queue.Get()
	if shutdown {
		return false
	}
	defer c.queue.Done(item)

	key := item.(string)
	log.Printf("service add/update：%s\n", key)

	namespace, name, _ := cache.SplitMetaNamespaceKey(key)
	service, err := c.serviceLister.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Println("service not exists, skip...")
		}
		return true
	}

	value, ok := service.GetAnnotations()[exposeIngressKey]
	if ok && "true" == value {
		c.syncIngerss(service)
	} else {
		c.deleteIngerss(service)
	}
	return true
}

func (c *SimpleController) syncIngerss(service *corev1.Service) {
	log.Printf("check and create/update ingress")
	ingress := c.getIngressByService(service)
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
			Name:      c.getIngressNameByServiceName(service.GetName()),
			Namespace: service.GetNamespace(),
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

func (c *SimpleController) deleteIngerss(service *corev1.Service) {
	log.Printf("delete ingress")
	ingress := c.getIngressByService(service)
	if ingress != nil {
		log.Printf("delete ingress %s\n", ingress.GetName())
		err := c.client.NetworkingV1().Ingresses(service.GetNamespace()).Delete(context.TODO(), ingress.GetName(), metav1.DeleteOptions{})
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

func (c *SimpleController) getIngressByService(service *corev1.Service) *apiNetworkV1.Ingress {
	ingress, err := c.ingressLister.Ingresses(service.GetNamespace()).Get(c.getIngressNameByServiceName(service.GetName()))
	if err != nil {
		log.Printf("%s\n", err)
		return nil
	}
	return ingress
}

func (c *SimpleController) getServiceNameByIngress(ingress *apiNetworkV1.Ingress) *string {
	ownerReference := metav1.GetControllerOf(ingress)
	if ownerReference == nil {
		return nil
	}
	if ownerReference.Kind != "Service" {
		return nil
	}
	return &ownerReference.Name
}

func (c *SimpleController) getIngressNameByServiceName(name string) string {
	return name + "-ingress"
}

// IngressDeleted 检查是否需要恢复Ingress
func (c *SimpleController) IngressDeleted(obj interface{}) {
	key, _ := cache.MetaNamespaceKeyFunc(obj)
	log.Printf("ingress delete：%s\n", key)

	namespace, _, _ := cache.SplitMetaNamespaceKey(key)
	ingress := obj.(*apiNetworkV1.Ingress)

	serviceName := c.getServiceNameByIngress(ingress)
	// 非本controller管理
	if serviceName == nil {
		log.Printf("ignore deleted ingress：%s\n", key)
		return
	}

	c.queue.Add(namespace + "/" + *serviceName)
}
