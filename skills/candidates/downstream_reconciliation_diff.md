# downstream_reconciliation_diff

## 来源

- `docs/告警处理手册.md`

## 场景

与下游对账发现差异，怀疑是数据同步异常或计算错误。

## 核心思路

1. 搜索 `error` 和 `reconciliation`
2. 根据日志内容分析差异原因

## 为什么适合做 skill

- 对账问题是很典型的业务场景
- 检索关键词和分析流程稳定
- 既可以走日志，也可以走知识库补充背景

## 更适合的运行时归属

- `logs`
- `knowledge`

## 下一步怎么实现

- logs 侧聚焦：同步异常、重复消费、补偿失败、计算偏差
- knowledge 侧补充：对账 SOP、补偿流程、人工校验步骤
