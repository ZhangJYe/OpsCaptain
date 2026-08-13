## Why

当前 RAG 已具备 Hybrid 检索、Query Rewrite 和 Rerank，但默认仅启用基础 Hybrid 路径，且现有消融评测曾混用生产 Hybrid 链路与独立 Dense 评测链路，阶段耗时口径也不足以支撑可靠比较。现有 development 仅 12 条、holdout 仅 6 条，无法稳定覆盖当前知识语料和困难查询，也不足以支撑参数选择。需要先建立可复现、可隔离且规模与覆盖率达标的 development/holdout 基线，再依据证据优化召回、排序和进入上下文的结果预算，避免凭少量样本或不可比指标调参。

## What Changes

- 统一 `hybrid-retrieve`、`hybrid-rewrite`、`hybrid-rerank`、`hybrid-full` 消融模式到同一生产 Hybrid 检索入口，仅通过显式开关改变 Rewrite/Rerank 行为。
- 建立 development 与 sealed holdout 两类评测集和运行约束，记录数据集身份、语料版本、有效配置、代码版本与运行环境，防止标签泄漏和结果混淆。
- 将 development 扩充到至少 60 条、sealed holdout 扩充到至少 100 条，并按精确实体、口语改写、多文档、中英文/缩写和困难干扰样本分层覆盖。
- 为样本增加类别、难度和语言标签，建立可评测知识文档清单、标签有效性校验、跨数据集近重复检测和覆盖率报告。
- 补齐 Recall@K、Hit Rate@K、MRR、失败率、空结果率及端到端和分阶段延迟统计，并纠正 Hybrid 检索耗时只代表 Dense 阶段的问题。
- 使用可配置的非回归门槛比较候选配置；只在 development 选定候选后运行 holdout，并保存逐案例结果与汇总报告。
- 依据评测结果调整 Hybrid 候选规模、最终 TopK、Rewrite/Rerank 开关与超时预算；失败时保留现有降级路径，不把离线结果表述为线上效果。
- 约束最终检索结果数量和下游 ContextEngine 预算衔接，避免扩大候选集后无界增加模型上下文。

## Capabilities

### New Capabilities

- `rag-retrieval-quality`: 定义生产路径可比评测、development/holdout 隔离、RAG 检索质量门槛、可观测性和安全降级行为。

### Modified Capabilities

无。

## Impact

- 主要影响 `internal/ai/rag/` 的 Hybrid trace、Query trace 和检索配置读取。
- 影响 `internal/ai/rag/eval/` 与 `internal/ai/cmd/rag_online_eval_cmd/` 的评测模式、指标、报告元数据和比较门槛。
- 可能调整 `manifest/config/config.yaml` 中现有 RAG 参数，并新增评测门槛配置；不新增 `internal/ai/rag/` 到 Milvus 基础设施的直接依赖。
- 新增或整理 `evals/rag/` 下的 development/holdout 数据与报告；评测仍属于离线或开发环境证据，不等同于生产验证。
- 影响 RAG 评测数据 schema、数据校验器和相关测试；新增数据规模、类别分布、文档覆盖和跨 split 泄漏检查。
- 不包含 Embedding 模型替换、向量数据库迁移、文档切分策略重构和线上自动调参。
