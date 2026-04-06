# Docs Knowledge Base

> 说明：`docs/` 目录用于存放 RAG 原始知识语料。  
> 当前内容以官方开源文档的中文摘要整理版为主，目标是提高检索相关性、可解释性和面试表达质量。

## 当前主题

- `01_kubernetes_crashloop_runbook.md`
- `02_kubernetes_deployment_release_and_rollback.md`
- `03_prometheus_alerting_and_rule_design.md`
- `04_helm_release_history_and_rollback.md`
- `05_argocd_autosync_prune_selfheal.md`
- `06_opentelemetry_observability_and_log_correlation.md`
- `08_open_source_knowledge_ingestion_strategy.md`
- `09_ingress_nginx_troubleshooting_and_gateway_transition.md`
- `10_etcd_snapshot_backup_and_restore.md`
- `11_mysql_innodb_deadlock_and_lock_wait.md`
- `12_redis_distributed_lock_boundaries.md`

## 为什么不是整站镜像

因为对 RAG 来说，高相关、低噪声、来源清晰，比单纯堆大量网页更重要。

当前采用的策略是：

1. 优先官方来源
2. 优先 runbook / troubleshooting / concepts
3. 每篇保留来源链接
4. 先做高质量摘要，再逐步导入许可清晰的官方 Markdown 子集

## 下一步扩展方向

- Redis 热 key / 缓存击穿与降级
- Kubernetes probe / readiness / liveness 常见误配
- Gateway API 基础模型与迁移路径
- etcd quorum 丢失与成员替换
- MySQL 慢 SQL 与索引失效
