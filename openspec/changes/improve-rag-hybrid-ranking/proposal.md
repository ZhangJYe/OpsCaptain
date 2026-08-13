## Why

上一轮 RAG 评测已经建立了可信的生产路径基线，但 Development 上仍有 16/60 条查询在 Recall@5 未完全命中：多文档查询 Recall@5 仅 `0.7083`，困难干扰查询仅 `0.6000`。逐案例结果显示，细粒度上游文档的标题、Provider、Tags 和稳定文档 ID 没有被字段化利用，RRF 也只能固定等权融合 Dense/BM25 排名，因此下一阶段应优先优化无模型调用的 Hybrid 召回和排序，而不是重新开启高延迟的 LLM Rewrite/Rerank。

## What Changes

- 为知识文档解析稳定的标题、Provider、Tags 和文档 ID 元数据，并让 BM25 与确定性精排分别使用这些字段，而不是把所有元数据无差别拼入正文词频。
- 将 Dense/BM25 的 RRF 权重和标题、标签、文档 ID 的精确/词项匹配加分做成 `config.yaml` 可配置项，并记录在评测报告的有效配置中。
- 在 Hybrid trace 和逐文档 trace 中记录字段命中、融合来源和排序变化，能够解释候选为什么进入或离开 FinalTopK。
- 增加按 category、difficulty、language 和缺失相关文档聚合的 Development 误差报告，重点观察 multi-document、hard-negative 和英文/混合查询。
- 只在现有 60 条 Development 上运行预先限定的消融和少量候选；上一轮指纹为 `5e8eee...849b9a` 的 sealed holdout 不得用于本轮调参。
- 候选必须同时通过 MRR、Recall@5、失败率、空结果率和 P95 Gate，并对 multi-document 与 hard-negative 分层指标设非回归门槛；没有候选通过则保留现有默认配置。
- 本轮不新增 LLM 调用、不更换 Embedding/Milvus、不扩大 FinalTopK，也不把离线指标表述为生产收益。

## Capabilities

### New Capabilities

- `rag-hybrid-ranking`: 定义知识文档字段化元数据、可配置 Hybrid 融合、可解释确定性精排、分层误差评测与新一轮数据隔离约束。

### Modified Capabilities

无。

## Impact

- 主要影响 `internal/ai/rag/` 的 BM25 文档表示、RRF 融合、确定性精排和 trace。
- 影响 `internal/ai/cmd/rag_online_eval_cmd/` 的知识文档预热、有效配置快照、分层误差汇总和报告格式。
- 在 `manifest/config/config.yaml` 的 RAG 段新增融合权重、字段加分和分层 Gate 配置；所有新增行为必须可关闭并保留当前默认回退路径。
- 复用 `evals/rag/retrieval_development.jsonl` 和上一轮 Development 基线作为调试与比较证据；不得重新读取上一轮 sealed holdout 的逐案例结果指导实现或参数选择。
- 不改变 Controller/Application 分层，不新增 `internal/ai/rag/` 到 Milvus 基础设施的依赖。
