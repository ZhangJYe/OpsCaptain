# Kubernetes CrashLoop 与服务下线排查手册

## 适用场景

- Pod 反复重启
- 服务不可用，怀疑容器启动即崩溃
- 发布后出现 `CrashLoopBackOff`
- 应用日志中出现 panic / fatal / OOM / probe fail

## 核心判断

`CrashLoopBackOff` 更像是 kubectl 呈现给人的“状态提示”，不是 Pod phase 本身。  
真正需要关注的是：

- Pod 是否反复重启
- 容器退出码是什么
- 是应用崩溃、探针误配、依赖未就绪，还是资源不足

## 官方排查主线

1. 先看 Pod 当前状态和重启次数
2. 再看 `describe` 事件，确认是拉镜像、调度、探针还是容器退出
3. 再看容器日志，定位 panic / 配置错误 / 依赖失败
4. 再检查模板参数：资源、探针、挂载、权限、启动命令

## 常用命令

```bash
kubectl -n <ns> get pod <pod>
kubectl -n <ns> describe pod <pod>
kubectl -n <ns> logs <pod> -c <container>
kubectl -n <ns> logs <pod> -c <container> --previous
```

## 高频原因

- 启动参数或配置错误
- ConfigMap / Secret / volume 缺失
- 读写权限不对
- 就绪探针或存活探针配置错误
- CPU / 内存不足，启动阶段太慢
- 应用启动时强依赖数据库、MQ、下游服务
- 镜像工作目录或入口命令与运行环境不匹配

## 面试时可以怎么讲

如果被问“你知识库里有没有 Kubernetes 故障文档”，你可以说：

“我把 CrashLoop 这类高频故障整理成统一 runbook，先看状态和事件，再看日志，再检查探针、资源和依赖。这样 RAG 检索到的不是泛泛概念，而是可执行的排障步骤。”

## 来源

- kube-prometheus runbooks: `KubePodCrashLooping`
  - https://runbooks.prometheus-operator.dev/runbooks/kubernetes/kubepodcrashlooping/
- Kubernetes: `Pod Lifecycle`
  - https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/

## 备注

本文件为基于官方文档整理的学习型摘要，不是原文镜像。
