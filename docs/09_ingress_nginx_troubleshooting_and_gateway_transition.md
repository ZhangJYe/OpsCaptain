# ingress-nginx 故障排查与 Gateway API 迁移提醒

## 适用场景

- 入口流量 404 / 502 / 504
- Ingress 规则已下发，但服务不可达
- 控制器 Pod 正常，业务还是不通
- 面试中被问到“入口层怎么排查”

## 先说一个重要背景

截至 2026 年 4 月，`kubernetes/ingress-nginx` 仓库已经归档，官方说明不建议新项目继续选它，而是建议转向 Gateway API 实现。

这不代表存量集群立刻不能用，而是：

- 老集群可以继续维护
- 新项目不建议再把它作为长期路线

## ingress-nginx 故障排查主线

### 1. 先确认 Ingress 事件

优先看资源事件，而不是一上来只看浏览器报错。

常用命令：

```bash
kubectl describe ing <ingress-name> -n <namespace>
kubectl get ing -A
```

你要看的是：

- 控制器有没有接收到资源变更
- host / path 是否正确
- backend service 是否存在

### 2. 看 controller 日志

```bash
kubectl get pods -n <ingress-namespace>
kubectl logs -n <ingress-namespace> <controller-pod>
```

重点关注：

- 配置生成失败
- backend service 找不到
- endpoint 为空
- 权限问题

### 3. 检查 Nginx 实际配置

官方 troubleshooting 文档明确建议在 Pod 内检查生成后的 Nginx 配置。

```bash
kubectl exec -it -n <ingress-namespace> <controller-pod> -- cat /etc/nginx/nginx.conf
```

如果业务说“规则明明配了”，而实际生成配置里没有对应 upstream / server / location，问题通常还在：

- annotation
- pathType
- service/endpoint
- ingressClass

### 4. 检查 Service 和 Endpoints

```bash
kubectl get svc -A
kubectl get endpoints -A
kubectl get endpointslice -A
```

高频问题：

- Service 名字配错
- Service selector 不匹配
- Pod readiness 不通过，导致 endpoint 为空

### 5. 无法监听 80/443 端口

官方文档把这类问题单独列出来，常见原因是：

- 容器能力缺失
- 镜像层异常
- 安全上下文不符合预期

也就是说，入口层问题不一定是路由规则错误，也可能是控制器本身没正确绑定端口。

## 常见面试答法

如果面试官问“入口层 502 你怎么查”，你可以这样答：

1. 先确认 Ingress 事件是否正常
2. 再看 ingress controller 日志
3. 然后检查生成的 nginx 配置
4. 再核对 Service、Endpoints、readiness
5. 如果仍有问题，再看 LB、安全组、节点端口和容器绑定能力

## 为什么现在要提 Gateway API

因为如果面试时只讲 ingress-nginx，而不提它已经退役，面试官会觉得你的技术判断过时。

更稳的说法是：

> 我把 ingress-nginx 当成存量系统排障知识保留在知识库里，但对新项目路线，我会优先关注 Gateway API 实现。

## 来源

- Source URL: https://kubernetes.github.io/ingress-nginx/troubleshooting/
- Source URL: https://github.com/kubernetes/ingress-nginx
