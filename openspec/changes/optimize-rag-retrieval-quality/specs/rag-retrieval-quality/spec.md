## Purpose

为 RAG 检索优化提供可复现、可比较且不泄漏 holdout 标签的质量契约，使召回、排序、延迟和降级表现能够共同决定配置是否可以启用。

## ADDED Requirements

### Requirement: Comparable production-path ablations
评测系统 SHALL 让 `hybrid-retrieve`、`hybrid-rewrite`、`hybrid-rerank` 和 `hybrid-full` 使用同一个生产 Hybrid 检索入口，并且各模式之间只改变声明的 Rewrite/Rerank 开关。

#### Scenario: Compare retrieval and rerank modes
- **WHEN** 操作者使用同一数据集、语料版本和基础配置运行 `hybrid-retrieve` 与 `hybrid-rerank`
- **THEN** 两次运行均经过 Dense、Lexical、Fusion 生产链路，且只有 Rerank 开关不同

#### Scenario: Reject unknown evaluation mode
- **WHEN** 操作者传入未声明的评测模式
- **THEN** 评测系统 MUST 返回明确错误并停止，而不是静默回退到其他模式

### Requirement: Development and holdout isolation
评测系统 SHALL 要求每次评测声明 `development` 或 `holdout` 数据角色，并 SHALL 在报告中记录数据集身份和内容指纹。系统 MUST NOT 自动使用 holdout 运行多参数搜索或模式矩阵。

#### Scenario: Tune on development data
- **WHEN** 操作者运行候选参数或多模式消融
- **THEN** 评测系统接受 `development` 数据并为每个候选生成独立报告

#### Scenario: Evaluate a selected holdout candidate
- **WHEN** 操作者使用 `holdout` 数据运行评测
- **THEN** 系统只评估显式指定的单个候选配置，并在报告中标记该运行属于 holdout

### Requirement: Adequate and stratified evaluation datasets
用于正式候选选择的 development 数据集 SHALL 至少包含 60 条有效样本，用于最终验证的 sealed holdout 数据集 SHALL 至少包含 100 条有效样本。每条样本 MUST 声明唯一 ID、自然语言 query、相关文档 ID、主类别、难度和语言；数据集 SHALL 按精确实体、口语改写、多文档、中英文或缩写、困难干扰五类查询进行分层，并输出类别分布与可评测知识文档覆盖率。

#### Scenario: Reject an undersized dataset for candidate selection
- **WHEN** development 少于 60 条，或 holdout 少于 100 条
- **THEN** 系统仍可输出用于调试的报告，但 MUST 将数据集标记为不满足正式候选选择或最终验证条件

#### Scenario: Validate dataset coverage and distribution
- **WHEN** 操作者校验 development 或 holdout 数据集
- **THEN** 校验结果列出样本数量、主类别分布、难度与语言分布、可评测文档覆盖率以及未覆盖文档

### Requirement: Evaluation label integrity and split leakage prevention
评测数据校验器 SHALL 确认每个非空相关文档 ID 均存在于版本化的可评测知识文档清单中，并 MUST 拒绝重复样本 ID、空 query、空相关文档列表、相关文档与干扰文档冲突以及 development/holdout 之间的相同或超过配置阈值的近重复 query。Holdout 标签、相关文档 ID 和评测结果 MUST NOT 进入检索索引、运行时提示或路由规则。

#### Scenario: Detect invalid labels
- **WHEN** 样本引用不存在的相关文档 ID，或相关文档列表为空
- **THEN** 校验失败并明确指出样本 ID 与无效字段

#### Scenario: Detect cross-split near duplicates
- **WHEN** development 与 holdout 中存在标准化后相同或相似度超过配置阈值的 query
- **THEN** 校验失败并列出冲突样本，holdout 不得用于最终评测

#### Scenario: Seal a valid holdout
- **WHEN** holdout 满足规模、schema、标签、覆盖和跨 split 防泄漏要求
- **THEN** 系统记录数据集版本与内容指纹，后续最终报告 MUST 引用同一指纹

### Requirement: Reproducible evaluation report
每份评测报告 SHALL 包含评测模式、数据角色、数据集指纹、语料版本、有效 RAG 配置、代码版本、生成时间、汇总指标、逐案例结果和失败信息。缺少必需的语料版本时，外部数据集评测 MUST 拒绝生成可比较报告。

#### Scenario: Generate a comparable report
- **WHEN** 一次评测成功完成
- **THEN** 输出报告包含复现该次运行所需的身份、配置和结果字段

#### Scenario: External dataset without corpus version
- **WHEN** 操作者对外部评测集运行评测但未提供语料版本
- **THEN** 评测系统返回校验错误且不生成标记为可比较的报告

### Requirement: Retrieval quality and reliability metrics
评测系统 SHALL 至少计算 Recall@K、Hit Rate@K、MRR、失败率和空结果率，并 SHALL 同时保留逐案例排名与命中信息。汇总指标 MUST 以成功案例数为分母的字段和以全部案例数为分母的可靠性字段分别表达。

#### Scenario: Partial query failures
- **WHEN** 部分查询失败且评测配置允许继续
- **THEN** 报告分别给出成功案例质量指标、总案例失败率和失败案例列表，不得把失败案例静默排除

### Requirement: Accurate latency and retrieval trace
评测系统 SHALL 记录端到端延迟以及 Rewrite、完整 Hybrid Retrieval 和 Rerank 阶段延迟，并 SHALL 输出平均值、P50 和 P95。完整 Hybrid Retrieval 延迟 MUST 覆盖并行 Dense/Lexical 检索和 Fusion，而不能仅以 Dense 延迟代替。

#### Scenario: Hybrid retrieval latency is reported
- **WHEN** 一次 Hybrid 查询完成
- **THEN** 查询 trace 中的 Retrieval 延迟表示 Hybrid 阶段墙钟时间，并保留 Dense、Lexical、Fusion 的细分延迟和候选数量

### Requirement: Configurable non-regression gate
系统 SHALL 使用 `manifest/config/config.yaml` 中的可配置门槛比较基线与候选报告。候选只有在主质量指标达到最小改进、其他质量指标未超过允许回退，并且失败率、空结果率和 P95 延迟均在预算内时才能通过。

#### Scenario: Candidate passes all gates
- **WHEN** 候选报告满足全部已配置质量、可靠性和延迟门槛
- **THEN** 比较结果标记为通过，并列出每项指标的基线值、候选值和差值

#### Scenario: Candidate improves recall but exceeds latency budget
- **WHEN** 候选提高了召回率但 P95 端到端延迟超过配置预算
- **THEN** 比较结果标记为不通过并指出延迟门槛失败

### Requirement: Bounded context handoff
RAG SHALL 将召回候选规模与最终交付规模分离。扩大候选集 MUST NOT 使交给下游的文档数超过配置的 FinalTopK，且文档进入模型上下文前 MUST 继续经过 ContextEngine token budget 裁剪。

#### Scenario: Large candidate pool with bounded final results
- **WHEN** CandidateTopK 大于 FinalTopK
- **THEN** RAG 可对较大候选集进行排序，但只向下游交付不超过 FinalTopK 的结果

### Requirement: Safe fallback and evidence boundary
Rewrite 或 Rerank 失败时，RAG SHALL 在当前请求预算内回退到原始查询或 Fusion 结果；若基础检索失败则 SHALL 返回可识别错误供上层降级。离线 development 或 holdout 报告 MUST 标明其证据环境，不得表示为生产效果。

#### Scenario: Rerank timeout
- **WHEN** Rerank 超时但 Fusion 已产生候选文档
- **THEN** RAG 返回经过 FinalTopK 限制的 Fusion 结果并记录 Rerank 降级信息

#### Scenario: Offline report is presented
- **WHEN** 系统输出 development 或 holdout 报告
- **THEN** 报告明确标记为离线或开发环境证据，不包含未经验证的生产结论
