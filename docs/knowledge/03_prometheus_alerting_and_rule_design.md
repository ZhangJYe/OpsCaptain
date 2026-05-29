# Prometheus 告警链路与规则设计手册

## 适用场景

- 想搞清楚 Prometheus 告警链路是怎么工作的
- 想知道 alert rule、Alertmanager、runbook 应该怎么配合
- 想把 Prometheus 官方知识变成项目里的 metrics / knowledge source

## 告警链路怎么分层

Prometheus 告警大致分两层：

1. Prometheus Server 负责评估规则
2. Alertmanager 负责聚合、抑制、静默和通知发送

所以 Prometheus 更像“发现问题”，Alertmanager 更像“管理告警投递”。

## alerting rule 的核心字段

- `alert`
- `expr`
- `for`
- `keep_firing_for`
- `labels`
- `annotations`

其中最重要的设计点是：

- `for`：避免瞬时抖动立刻 page
- `keep_firing_for`：避免数据缺失或抖动造成误恢复
- `annotations`：非常适合放 runbook link 和排障描述

## 为什么规则要和 runbook 一起设计

只写 PromQL 不够。  
一个高质量告警至少应该回答：

- 这个问题是什么
- 严重度是什么
- 建议先看什么
- runbook 在哪里

所以你的项目里非常适合把：

- Prometheus rule 设计知识
- Alertmanager 告警链路知识
- kube-prometheus runbook

一起沉淀进知识库。

## 规则工程的最小实践

1. 规则文件用 YAML 管理
2. 发布前用 `promtool check rules` 校验
3. `annotations` 里放 summary / description / runbook
4. 高噪声规则必须设计好 `for`
5. 需要预计算的查询，优先做 recording rules

## 面试时可以怎么讲

“我把 Prometheus 知识库分成了链路层和规则层。链路层解释 Prometheus 与 Alertmanager 的职责边界，规则层解释 `for`、`annotations`、`keep_firing_for` 等设计点，这样系统回答的不只是‘看哪个指标’，而是‘怎么设计可运维的告警’。”

## 来源

- Prometheus: `Alerting overview`
  - https://prometheus.io/docs/alerting/latest/overview/
- Prometheus: `Alerting rules`
  - https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/
- Prometheus: `Defining recording rules`
  - https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/

## 备注

本文件为基于官方文档整理的学习型摘要，不是原文镜像。
