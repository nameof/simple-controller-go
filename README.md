### simple-controller-go
一个简单的k8s controller，根据service变更事件读取注解配置，协调对应ingress资源的同步。 例如
```go
if service create/update && service.annotations["simple-controller.nameof.github.com/exposeIngress"] == "true" 
    create ingress
else
    delete ingress
```