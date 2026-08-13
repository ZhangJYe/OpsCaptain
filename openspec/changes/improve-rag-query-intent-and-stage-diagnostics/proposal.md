## Why

字段化 Hybrid 候选已把 Development Recall@5 提升到 `0.8111`，但仍有 14 个案例未完全召回，其中 hard-negative 6 个、multi-document 5 个。当前报告只展示最终排序，无法判断相关文档是在 Dense/BM25、Fusion、字段精排还是 FinalTopK 阶段丢失；同时查询中的“只要/不是/不执行”等对比语义仍被当作普通正向词项，导致相邻文档互相干扰。

## What Changes

- 为 Development 评测增加 Dense、Lexical、Fusion、Candidate、Final 各阶段的逐文档身份与 Recall@K 诊断，明确召回缺口发生位置。
- 新增确定性的正向/排除意图解析，识别“只看 A，不要 B”“A 而不是 B”等有限语法，不调用 LLM、不读取评测标签。
- 在候选精排中配置化奖励正向信号、有限降权仅命中排除信号的文档，并输出意图和加减分 trace。
- 增加 hard-negative 及整体质量非回归 Gate，限定一个兼容基线、一个意图候选，不进行新的权重网格搜索。
- 保持 CandidateTopK、FinalTopK、ContextEngine token budget 与生产默认开关不变；本轮只形成 Development 候选。

## Capabilities

### New Capabilities

- `rag-query-intent-diagnostics`: 定义阶段级召回诊断、确定性正向/排除意图解析、可解释候选降权及其离线评测边界。

### Modified Capabilities

无。

## Impact

- 主要影响 `internal/ai/rag/` 的 Hybrid trace、查询意图 profile 与确定性精排。
- 影响 `internal/ai/rag/eval/` 和 `internal/ai/cmd/rag_online_eval_cmd/` 的报告 schema、阶段指标、Gate 与 Development 报告。
- 新增配置进入 `manifest/config/config.yaml`，默认关闭以保持当前生产行为。
- 不新增网络模型调用，不修改知识正文，不复用上一代 sealed holdout。
