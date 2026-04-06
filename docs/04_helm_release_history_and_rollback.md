# Helm 发布历史与回滚手册

## 适用场景

- Helm 发布后需要快速回滚
- 想知道 revision 是什么
- 想知道回滚前后应该观察哪些点

## Helm 回滚的核心概念

Helm 把每次发布记录成 revision。  
`helm rollback` 的本质，是把某个 release 回退到之前的 revision。

如果不指定 revision，或者写 `0`，一般就表示回到上一个版本。

## 常用命令

```bash
helm history <release>
helm rollback <release> [revision]
helm rollback <release> 0
```

## 关键参数

- `--cleanup-on-fail`
  回滚失败时，允许清理这次回滚中创建的新资源
- `--dry-run`
  模拟执行，不落地
- `--force`
  必要时通过删除/重建方式强制更新资源
- `--history-max`
  限制保留的 revision 数量
- `--no-hooks`
  回滚时不执行 hooks

## 生产环境建议

1. 回滚前先看 `helm history`
2. 尽量明确目标 revision，而不是盲目回到上一个
3. 对有 hooks 的 chart，要提前评估回滚副作用
4. 回滚后不要结束，继续看 Pod、Service、日志和业务指标

## 和 Kubernetes 原生回滚的区别

- `kubectl rollout undo` 更偏 Deployment/工作负载层
- `helm rollback` 更偏 release 层，会受到 chart、hooks、values 的影响

所以如果你的系统是 Helm 管理的，知识库里最好同时有：

- Kubernetes rollout 文档
- Helm rollback 文档

## 面试时可以怎么讲

“我没有只把 Kubernetes 的 `rollout undo` 放进知识库，还单独补了 Helm rollback，因为生产里很多团队是 chart 级发布，不是直接手改 Deployment。这个区分能体现我知道 release 层和 workload 层不是一回事。”

## 来源

- Helm: `helm rollback`
  - https://helm.sh/docs/helm/helm_rollback/
  - 中文页：https://helm.sh/zh/docs/v3/helm/helm_rollback/

## 备注

本文件为基于官方文档整理的学习型摘要，不是原文镜像。
