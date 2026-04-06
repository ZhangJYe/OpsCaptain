# Ops Knowledge Index

> 说明：本目录中的这批文档不是官方原文复制，而是基于官方开源文档整理的中文摘要版知识库。
> 目的：更适合当前项目做 RAG 检索、skills 路由和面试复盘。

## 当前整理的官方主题

1. Kubernetes Pod CrashLoop / 服务下线排查
2. Kubernetes Deployment 发布、观察和回滚
3. Prometheus 告警链路与规则设计
4. Helm 发布历史与回滚
5. Argo CD 自动同步、自愈和清理
6. OpenTelemetry 可观测性与日志关联

## 为什么先整理这些

- 都是典型的 oncall / ops 高频场景
- 和你当前项目里的 `logs` / `metrics` / `knowledge` skills 高度匹配
- 非常适合面试时解释“知识库为什么这样建”

## 如何使用这些文档

- 给 RAG 做原始检索语料
- 给 skills 提供高质量 source-of-truth
- 给面试讲项目时提供“我做过知识标准化”的证据

## 建议的扩展顺序

第一批已经补的是“平台基础知识”。下一批建议补：

- 登录鉴权失败排查
- API 失败率升高排查
- Pod 探针 / readiness / liveness 常见误配
- Prometheus 告警降噪与分级
- Kubernetes ConfigMap / Secret / Volume 缺失排查
- 下游对账差异与地域不匹配

## 来源说明

这批文档主要整理自：

- Kubernetes 官方文档
- Prometheus 官方文档
- kube-prometheus runbooks
- Helm 官方文档
- Argo CD 官方文档
- OpenTelemetry 官方文档

后续如果你要继续补，我建议仍然坚持：

1. 优先官方文档
2. 优先 runbook / troubleshooting / concepts
3. 原始来源可以很多，但落地知识库最好统一成结构化 Markdown
