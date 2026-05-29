# Kubernetes Deployment 发布、观察与回滚手册

## 适用场景

- 新版本发布后需要观察 rollout
- 发布卡住，想知道如何判断是否失败
- 发布后业务异常，需要快速回滚
- 想看历史 revision 和变更记录

## Deployment 的核心价值

Deployment 用声明式方式管理 Pod 和 ReplicaSet。  
每次真正改变 `.spec.template`，都会产生新的 revision。  
这意味着你可以：

- 观察 rollout 是否完成
- 查看 rollout history
- 回滚到之前的 revision

## 日常发布观察命令

```bash
kubectl rollout status deployment/<name>
kubectl rollout history deployment/<name>
kubectl rollout undo deployment/<name>
kubectl rollout undo deployment/<name> --to-revision=<n>
```

## 你真正应该看的信号

- rollout 是否持续 progressing
- 新 ReplicaSet 是否创建成功
- 新 Pod 是否 ready
- 旧 Pod 是否有序退出
- revision history 是否还保留

## 排障思路

1. 发布后先执行 `rollout status`
2. 如果卡住，看 Pod 是否 crash、探针失败、资源不足
3. 如果确认当前版本不稳定，先看 `rollout history`
4. 然后用 `rollout undo` 回滚到上一个或指定 revision
5. 回滚后继续观察 rollout 状态，不要只执行不验证

## 一个重要坑

如果把 `.spec.revisionHistoryLimit` 设为 `0`，历史会被清掉，回滚能力会大幅受限。  
这类配置在生产环境要谨慎。

## 面试时可以怎么讲

“我不是只会 `kubectl apply`。我把 Deployment 的 rollout status、history 和 undo 都整理成知识库，让系统能回答发布观察、失败判断和回滚步骤，这比只存‘发布 SOP’更接近真实生产。”

## 来源

- Kubernetes: `Deployments`
  - https://kubernetes.io/docs/concepts/workloads/controllers/deployment/
- Kubernetes: `kubectl rollout status`
  - https://kubernetes.io/docs/reference/kubectl/generated/kubectl_rollout/kubectl_rollout_status/
- Kubernetes: `kubectl rollout undo`
  - https://kubernetes.io/docs/reference/kubectl/generated/kubectl_rollout/kubectl_rollout_undo/

## 备注

本文件为基于官方文档整理的学习型摘要，不是原文镜像。
