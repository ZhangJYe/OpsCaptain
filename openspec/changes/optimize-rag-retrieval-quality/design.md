## Context

参见 `proposal.md` 的动机。当前生产 Query 路径已经组合 Query Rewrite、并行 Dense/BM25、RRF Fusion、Metadata Boost、可选 LLM Rerank 和 FinalTopK 裁剪；开关和候选规模均来自 `manifest/config/config.yaml`。现有评测路径已补齐数据/语料/配置身份、P50/P95、候选门槛和 Hybrid trace，但数据层仍只有 12 条 development 与 6 条 holdout。当前 `docs/knowledge/` 有 30 份 Markdown，其中包含 README、索引和说明文档，必须先筛出真正具备答案内容的可评测文档，再扩大样本。

本设计遵守以下约束：`internal/ai/rag/` 不直接依赖 Milvus 实现；RAG 结果仍需经过 ContextEngine budget；评测失败不触发进程级业务故障；离线评测不作为生产验证。

## Goals / Non-Goals

**Goals:**

- 在同一生产 Hybrid 路径上得到可信的 Rewrite/Rerank 消融结论。
- 建立至少 60 条 development 与 100 条 sealed holdout，并让规模、类别分布、标签有效性、语料覆盖和跨 split 泄漏可自动校验。
- 将数据身份、有效配置、质量、可靠性和延迟放入同一份可比较报告。
- 以 development 选择候选，以 sealed holdout 验证候选，并用可配置 gate 决定是否调整默认配置。
- 保持候选召回规模与最终上下文规模解耦，保留超时和模型失败时的回退。

**Non-Goals:**

- 不更换 Embedding/LLM、Milvus 或 BM25 实现。
- 不重构知识文档切分、入库和增量索引链路。
- 不新增线上自动调参、A/B 流量或生产效果宣称。
- 不把 Planner/Agent 模式纳入本次基础检索配置选择；它们可继续单独评测。
- 不在本次引入无答案判定或检索置信阈值；`relevant_ids` 为空的 answerability/abstention 评测另行设计。

## Decisions

### 1. 先固定可比基线，再调整默认配置

保留现有 `rag_online_eval_cmd` 中尚未提交的方向：所有基础消融模式都调用生产 `rag.Query`，通过 request context override 控制 Rewrite/Rerank。`hybrid` 表示读取当前配置，四个显式消融模式表示固定组合；未知模式直接报错。

这样可以确保 Dense、Lexical、Fusion、Metadata Boost、缓存和裁剪路径一致。备选方案是继续用 `QueryForEval` 作为无增强基线，但该路径是 Dense-only，无法把结果差异归因于 Rewrite/Rerank，因此不采用。

### 2. 数据集角色由运行入口显式声明

评测入口新增 `dataset-role` 和 `corpus-version` 参数。外部 JSONL 的 SHA-256 作为 `dataset_fingerprint`；`corpus-version` 由语料发布或索引构建过程提供，避免依赖无法枚举的远端向量库内容。报告同时记录代码 revision 和有效 RAG 配置快照。

允许 development 运行模式矩阵和参数候选；holdout 一次只接受一个显式模式/配置，不提供自动 sweep。代码无法阻止人工重复查看 holdout，因此报告中的角色、时间和候选身份用于审计，流程上要求候选先由 development gate 选出。

备选方案是自动随机切分单一数据集，但小样本切分不稳定且容易随运行变化，因此使用版本化的独立文件和固定指纹。

### 3. 报告扩展而不引入新的评测框架

沿用 `internal/ai/rag/eval`：逐案例结果继续保存排名、相关 ID 和 Recall@K，并增加运行元数据、失败率、P50/P95 和报告比较结果。延迟百分位从已有逐案例 `QueryMetrics` 计算，避免增加外部依赖。

质量主指标默认由配置选择，建议以 MRR 或 Recall@5 之一作为 primary；其余 Recall@K、Hit Rate@K 作为非回归指标。失败率、空结果率和 P95 总延迟是独立硬门槛，不能用质量提升抵消。所有 gate 数值放在 `manifest/config/config.yaml` 的 `rag.eval_gate` 下。

备选方案是只输出一个综合分数，但权重会掩盖失败和延迟回退，因此不采用。

### 4. 修正 Hybrid trace 的时间语义

`HybridTrace` 增加 Hybrid 阶段墙钟总耗时，并保留 Dense、Lexical、Fusion 细分耗时和候选计数。由于 Dense 与 Lexical 并行，总耗时不能用三项简单相加；在进入两个并行检索前开始计时，在 Fusion 和候选裁剪完成后结束。`QueryTrace.RetrieveLatencyMs` 使用该墙钟值，评测结构额外透传细分字段。

Rerank、Rewrite 与端到端延迟维持独立计时。报告对成功案例计算 avg/P50/P95，同时用总案例数单独计算失败率。

### 5. 只用评测证据调整已有 RAG 参数

候选矩阵优先使用已有配置项：DenseTopK、LexicalTopK、FusionK、CandidateTopK、FinalTopK、Rewrite/Rerank enabled、超时和最大 Rerank 候选数。先运行当前配置和四个消融模式，再只对胜出的组合做小范围 development 调参；通过 development gate 后才进入 holdout。

实现不添加运行时自动调参器。最终是否修改默认开关和数值取决于实际报告；若真实 Milvus、Embedding 或模型服务不可用，则完成代码和确定性测试，但把真实依赖评测标记为未完成，不以 mock 结果替代。

### 6. 最终结果继续受双重预算约束

Hybrid 召回先截断到 CandidateTopK，Rerank 或 Fusion fallback 后再截断到 FinalTopK。进入生成模型之前继续调用既有 ContextEngine token budget；本次只补充测试和观测，不把 token 裁剪复制到 RAG 包。

### 7. 先建立可评测语料清单，再扩充 query

新增版本化数据清单，列出当前语料版本中可作为答案依据的稳定文档 ID，并排除 README、目录索引、导入说明等不适合作为答案标签的文件。每条正样本的 `relevant_ids` 必须引用该清单；多文档问题允许引用多个 ID。

采用清单而不是直接要求覆盖 `docs/knowledge/` 下全部文件，是因为目录中文档职责不同，机械覆盖会把索引页和说明页误当成标准答案。清单同时记录语料版本，使文档增删后必须重新校验数据集。

### 8. 使用单一主类别约束分布

每条样本声明一个主类别，允许在 notes 中记录次要特征。Development 与 holdout 均以以下比例为目标，并允许单类在目标值上下浮动 5 个百分点：

- 精确实体与术语：20%
- 口语化与同义改写：25%
- 多文档或多步骤查询：20%
- 中英文、缩写与混合表达：10%
- 相似文档干扰等困难样本：25%

单一主类别使分布统计互斥且可复现。备选方案是一个样本带多个类别并分别计数，但会导致总比例超过 100%，难以判断某类是否真正不足，因此不采用。

### 9. 扩展数据 schema 并自动检查标签质量

在现有 `id`、`query`、`relevant_ids`、`notes` 基础上增加必填的 `category`、`difficulty` 和 `language`，困难样本可选填 `distractor_ids`。校验器检查：

- ID 唯一、query 非空、相关文档非空且存在；
- `relevant_ids` 与 `distractor_ids` 不重叠；
- 类别、难度、语言属于声明枚举；
- 每份可评测文档的 development/holdout 覆盖情况；
- development 与 holdout 的标准化完全重复和近重复 query。

近重复阈值放入 `manifest/config/config.yaml` 的 RAG 评测配置，不在校验逻辑中硬编码。校验先采用可解释的 token 集合相似度，不调用 LLM，避免校验结果受模型版本和网络状态影响。

### 10. Holdout 先封存，后运行

Holdout 可以在候选确定前完成编写和静态标签校验，但不得用于检索模式比较。静态校验通过后保存版本和 SHA-256 指纹；开发期间只运行 development。待候选配置在 development 上冻结后，使用相同指纹的 holdout 运行一次最终 gate。

如果 holdout 暴露出系统性数据错误，可以修订数据并生成新版本，但旧结果作废，且新版本不得继续用于同一轮反复调参。

## Risks / Trade-offs

- [评测集过小导致调参过拟合] → development 与 holdout 独立版本化，限制候选数量，并在报告中保留逐案例变化。
- [批量生成的问题模板化，造成虚假的高召回] → 按主类别分层，要求困难干扰样本，并人工抽查 query 是否仅复述标题。
- [标签 ID 错误或语料更新导致标签失效] → 使用可评测文档清单和自动标签校验，语料版本变化后重新验证。
- [Holdout 被反复查看并用于调参] → 封存指纹、限制为单候选运行；修订时提升数据版本并作废旧比较。
- [远端语料与报告声明不一致] → 要求显式 `corpus-version`，报告不把缺失版本的运行标记为可比较。
- [Rewrite/Rerank 提升质量但增加模型成本和延迟] → 使用独立 P95、失败率和超时 gate，保留关闭开关和 fallback。
- [缓存使不同模式结果不可比] → 每份报告记录缓存命中；基线矩阵使用一致的缓存策略，并在比较前校验关键配置一致。
- [本地依赖不可用] → 确定性单元测试只验证路径、指标和 gate；真实 Hybrid 报告必须来自可用的 Milvus/模型环境并明确证据类型。

## Migration Plan

1. 保留已完成的评测路径、trace/报告字段和单元测试，不改变生产 RAG 默认开关。
2. 盘点当前语料并建立版本化可评测文档清单，扩展数据 schema 与静态校验器。
3. 将 development 扩充到至少 60 条并通过规模、分层、覆盖、标签和泄漏校验，再运行基线、消融与有限候选参数。
4. 将 holdout 扩充到至少 100 条，完成静态校验后封存版本和指纹，但不运行检索评测。
5. 冻结 development 胜出的单一候选后运行 sealed holdout，并保存基线、候选、gate 和逐案例报告。
6. 只有 holdout gate 通过时才更新 `config.yaml` 的默认参数；否则保持现状。回滚时恢复原配置即可，旧数据版本和旧报告不参与新 gate 比较。
