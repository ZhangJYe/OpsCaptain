# Argo CD 自动同步、自愈与清理手册

## 适用场景

- 想把发布权限从 CI/CD 流水线收敛到 GitOps
- 想知道 auto-sync、prune、self-heal 怎么配
- 想知道为什么 Git 变更提交后集群会自动更新

## 自动同步的核心价值

Argo CD 会比较：

- Git 中期望的 manifests
- 集群中的实时状态

当两边有差异时，可以自动执行同步。  
这意味着 CI/CD 不一定要直接调用 Argo CD API，只要提交代码到 Git，就能触发交付。

## 常见配置项

### 开启自动同步

```bash
argocd app set <APPNAME> --sync-policy automated
```

### 自动清理 prune

默认出于安全原因，自动同步不会删除 Git 中已不存在的资源。  
如果你希望 Git 删掉后集群也自动删，需要开启 prune。

### 自动自愈 self-heal

默认情况下，直接在集群里改 live state 不一定会触发自动修正。  
如果开启 self-heal，集群偏离 Git 定义时，Argo CD 会尝试拉回期望状态。

## 使用建议

1. 新团队先开 automated，不急着开 prune
2. prune 适合资源治理比较成熟之后再开
3. self-heal 很适合防止“手工改线上”漂移
4. 多源应用要特别注意 autosync / self-heal 的副作用

## 一个重要理解

GitOps 不是“把 kubectl 换个入口”，而是：

- Git 变成 source-of-truth
- Argo CD 负责持续对齐 desired state 和 live state

这类知识非常适合你的项目，因为你已经做了：

- 发布
- rollback
- opscaption
- skills

Argo CD 文档补进来以后，知识库就更像真实平台工程。

## 面试时可以怎么讲

“我在知识库里补了 Argo CD 自动同步、自愈和 prune 的文档，因为 GitOps 场景下，故障排查不只是看 Pod，还要判断 live state 为什么偏离 Git，系统才能给出更平台化的排障建议。”

## 来源

- Argo CD: `Automated Sync Policy`
  - https://argo-cd.readthedocs.io/en/stable/user-guide/auto_sync/

## 备注

本文件为基于官方文档整理的学习型摘要，不是原文镜像。
