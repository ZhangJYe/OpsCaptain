# 实施与评测记录

## 证据边界

- 本轮只使用 `development-v4-field-boost.json` 的汇总与逐案例结果定位问题。
- 不运行、不查看任何 sealed holdout 的逐案例结果；holdout 不能作为 Development Gate 基线。
- 禁止依据案例 ID、relevant_ids、distractor_ids 生成专属规则或词表；不追加 RRF 权重网格；不更新生产默认开关。

## 只读误差输入

字段增强冻结候选：60 个 Development 案例，MRR `0.7097`、Recall@5 `0.8111`、0 失败。Recall@5 未完全命中 14 例：

`DEV-013`、`DEV-018`、`DEV-032`、`DEV-034`、`DEV-035`、`DEV-037`、`DEV-039`、`DEV-044`、`DEV-048`、`DEV-050`、`DEV-051`、`DEV-052`、`DEV-056`、`DEV-060`。

Category 分布：hard-negative 6、multi-document 5、semantic-paraphrase 2、cross-language 1。高频缺失文档：`argocd-auto-sync`、`helm-history`、`opentelemetry-logs`、`opentelemetry-metrics`、`prometheus-query-log` 各 2 次；其余各 1 次。

该记录只用于确定“阶段诊断 + 通用对比意图”这一能力范围，不用于构造答案导向词表。

## Development v5 限定实验

两份报告使用同一 Development 数据指纹 `35eec6f8c266156e96ea7ec1f2eed48452da16e7458dad415690b2454bfe0d3d`、语料版本 `knowledge-seed-v1`、DenseTopK `50`、LexicalTopK `50`、CandidateTopK `20`、FinalTopK `5` 和单请求超时 `15000ms`。

| 指标 | v5 关闭基线 | 唯一意图候选 | 结论 |
|---|---:|---:|---|
| MRR | 0.7125 | 0.7181 | +0.0056 |
| Recall@5 | 0.8056 | 0.8056 | 持平 |
| multi-document Recall@5 | 0.7222 | 0.7222 | 持平 |
| hard-negative Recall@5 | 0.6000 | 0.6000 | 持平 |
| failure / empty | 0 / 0 | 0 / 0 | 持平 |
| P95 | 285ms | 187ms | 本次离线运行更低，不解释为稳定性能收益 |

候选解析 13/60 个查询（21.67%），13 个均产生精排信号，共记录 209 个候选降权。全部 Gate 通过；没有追加连接词、案例专属规则、权重候选或网格搜索。

## 阶段诊断结论

关闭基线的阶段 Recall@5：Dense `0.8778`、Lexical `0.5750`、Fusion `0.7944`、Candidate/Final `0.8056`。83 个相关文档身份均进入 CandidateTopK，其中 66 个到达 Final、17 个在 FinalTopK 边界丢失。这说明本轮剩余主缺口集中在最终预算裁剪，而不是 CandidateTopK 前完全未召回；不据此扩大预算。

## 冻结结论

- 冻结为 **Development 通过候选**，报告为 `development-v5-stage-baseline.json` 和 `development-v5-intent-candidate.json`。
- 生产默认 `intent_refinement_enabled: false` 保持不变；未运行或查看 sealed holdout。
- 上线或修改默认值前，必须建立新的独立 holdout 验证，当前结论不构成生产效果证明。
