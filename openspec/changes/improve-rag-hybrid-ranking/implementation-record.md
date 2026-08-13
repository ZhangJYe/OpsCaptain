# 实施与评测记录

## 证据边界

- 本轮只使用 Development 数据设计和选择候选。
- `holdout-v2-hybrid-retrieve-frozen.json` 属于上一代 sealed holdout，不进入本轮比较命令，不查看其逐案例结果，不用于特征或权重选择。
- Development 指标只能说明当前离线数据、语料版本和依赖环境下的效果；不能表述为生产收益或泛化证明。

## v3 误差基线

报告：`evals/rag/reports/development-v3-hybrid-retrieve.json`

| 指标 | 数值 |
|---|---:|
| Cases / Failed | 60 / 0 |
| MRR | 0.7061 |
| Recall@1 / @3 / @5 | 0.4806 / 0.7389 / 0.7944 |
| Hit Rate@5 | 0.8500 |
| P95 | 172 ms |

Category Recall@5：exact_entity `1.0000`、semantic_paraphrase `0.8667`、cross_language `0.8611`、multi_document `0.7083`、hard_negative `0.6000`。主要瓶颈是 hard-negative 与 multi-document。

Difficulty Recall@5：easy `1.0000`、medium `0.8417`、hard `0.6944`。Language Recall@5：zh `0.8750`、mixed `0.7683`、en `0.7222`。

Recall@5 未完全命中 16 例：`DEV-013`、`DEV-018`、`DEV-030`、`DEV-032`、`DEV-033`、`DEV-034`、`DEV-035`、`DEV-039`、`DEV-040`、`DEV-044`、`DEV-048`、`DEV-050`、`DEV-051`、`DEV-052`、`DEV-056`、`DEV-060`。

高频缺失文档：`helm-history` 3 次；`prometheus-query-log`、`prometheus-alerting-practices`、`opentelemetry-metrics`、`opentelemetry-logs`、`helm-rollback`、`argocd-auto-sync` 各 2 次。该分布支持先增强稳定 ID、标题、Tags、Provider 的确定性字段信号，再评估有界覆盖。

## 预声明实验顺序

1. v4 兼容基线：Dense/Lexical `1.0/1.0`，字段增强关闭，覆盖关闭。
2. 字段候选：只开启字段化 BM25 与字段精排。
3. 仅字段候选通过 Gate 时测试 `1.0/1.25` 与 `1.0/1.5` 两个 RRF 候选。
4. 仅 multi-document Recall@5 仍为瓶颈时测试一个覆盖候选。

候选总数不超过四个，不追加临时网格搜索。

## 实施内容

- Markdown 首个 H1、Provider、Tags、稳定文档 ID、Source 使用同一解析入口进入生产 loader 与离线 BM25 预热。
- BM25 正文 token 与字段 token 分离；字段不参与正文长度归一化。
- 加权 RRF、知识字段精排和有界覆盖全部可配置；兼容默认值保持等权且关闭新增排序能力。
- 文档 trace 增加融合、字段精排、覆盖与最终位置；报告 schema v3 增加 category/difficulty/language 分层及缺失文档频次。
- Gate 增加 multi-document 与 hard-negative Recall@5 非回归检查，并拒绝 Development 候选使用 holdout 基线。

## Development v4 评测结果

所有报告使用 Development 指纹 `35eec6f8c266156e96ea7ec1f2eed48452da16e7458dad415690b2454bfe0d3d`、语料版本 `knowledge-seed-v1`、schema v3、60 个案例、0 失败。上一代 holdout 未进入命令。

| 配置 | MRR | Recall@5 | multi-document R@5 | hard-negative R@5 | P95 | Gate |
|---|---:|---:|---:|---:|---:|---|
| v4 兼容基线 | 0.6978 | 0.7556 | 0.6806 | 0.5333 | 200ms | 基线 |
| 字段增强 | 0.7097 | 0.8111 | 0.7500 | 0.6000 | 205ms | 通过、冻结 |
| 字段 + RRF 1.0/1.25 | 0.4958 | 0.5806 | 0.6944 | 0.5333 | 227ms | 拒绝 |
| 字段 + RRF 1.0/1.5 | 0.4958 | 0.5806 | 0.6944 | 0.5333 | 184ms | 拒绝 |
| 字段 + 有界覆盖 | 0.7042 | 0.8000 | 0.6944 | 0.6000 | 189ms | 过基线 Gate，但弱于字段候选 |

报告位置：

- `evals/rag/reports/development-v4-hybrid-retrieve.json`
- `evals/rag/reports/development-v4-field-boost.json`
- `evals/rag/reports/development-v4-field-rrf-1.25.json`
- `evals/rag/reports/development-v4-field-rrf-1.5.json`
- `evals/rag/reports/development-v4-field-coverage.json`

## 冻结结论

冻结 Development 候选：Dense/Lexical RRF `1.0/1.0`、字段增强开启、有界覆盖关闭。两个 Lexical 偏置候选均显著降低整体质量，因此拒绝且不追加权重搜索；覆盖候选虽超过 v4 基线，但 MRR、Recall@5 和 multi-document Recall@5 都低于字段单独候选，因此不冻结。

冻结候选相对同代基线：MRR `+0.0119`、Recall@5 `+0.0556`、multi-document Recall@5 `+0.0694`、hard-negative Recall@5 `+0.0667`，P95 `+5ms`，失败率和空结果率均为 0。

默认配置继续保持字段增强和覆盖关闭；本结果仅是离线 Development 证据。仍需创建新一代独立 holdout 后才能验证泛化，仍缺生产与线上验证。
