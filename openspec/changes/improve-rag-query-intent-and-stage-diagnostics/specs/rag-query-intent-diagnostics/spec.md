## Purpose

为 OpsCaptain 的 Hybrid RAG 提供逐阶段召回可观测性与不依赖大模型的正向/排除意图处理，使离线优化能够定位相关文档丢失阶段，并可解释地降低否定概念造成的相邻文档干扰。

## ADDED Requirements

### Requirement: Stage-level retrieval diagnostics
评测系统 SHALL 为每个案例记录 Dense、Lexical、Fusion、Candidate 与 Final 阶段的规范化文档 ID，并 SHALL 汇总每个阶段的 Recall@K。阶段诊断 MUST 只观察运行时检索结果，不得改变排序或读取相关标签指导检索。

#### Scenario: Relevant document disappears before final selection
- **WHEN** 相关文档出现在某个上游阶段但未出现在 FinalTopK
- **THEN** 报告标明它最后出现的阶段及对应排名，并在阶段汇总中反映召回下降

#### Scenario: Retriever never recalls the document
- **WHEN** 相关文档未出现在 Dense 或 Lexical 候选中
- **THEN** 报告将缺口归类为召回阶段缺失，而不是精排或 FinalTopK 丢失

### Requirement: Deterministic positive and exclusion intent
系统 SHALL 支持配置化的确定性查询意图解析，识别有限的正向目标与排除片段，例如“只看 A，不要 B”“A 而不是 B”。解析 MUST 基于原始 query，不调用 LLM，不读取 Memory、category、relevant_ids 或 distractor_ids。

#### Scenario: Query contains a contrast clause
- **WHEN** 查询同时包含正向目标和明确的“不是/不要/不执行/而不是”等排除表达
- **THEN** 系统输出正向词项、排除词项、命中的规则和原始片段，并保持可复现

#### Scenario: Query has no supported exclusion expression
- **WHEN** 查询没有可识别的排除表达或表达存在歧义
- **THEN** 系统不构造排除词项，保持当前查询与排序兼容

### Requirement: Bounded intent-aware refinement
系统 SHALL 在候选已召回后使用正向与排除词项执行确定性精排。正向字段匹配 SHALL 获得配置化加分；文档只命中排除词项且缺少正向匹配时 MAY 获得有上限的降权。系统 MUST 保持 CandidateTopK、FinalTopK 和 ContextEngine token budget 不变。

#### Scenario: Excluded concept is the only strong match
- **WHEN** 候选文档显著命中排除词项但没有命中正向目标
- **THEN** 系统按照配置有限降权该候选，并记录正向命中、排除命中、净分和排序位置

#### Scenario: Document matches both positive and excluded concepts
- **WHEN** 文档同时命中正向和排除词项
- **THEN** 系统不得仅凭排除词项删除文档，而是计算有界净分并保留可解释 trace

#### Scenario: Intent refinement is disabled
- **WHEN** 意图精排 feature flag 关闭
- **THEN** 检索排序与冻结的字段增强候选保持兼容，意图加减分为零

### Requirement: Intent-aware development evaluation
Development 候选 SHALL 使用同数据指纹、语料版本、报告 schema 与检索预算同兼容基线比较。Gate SHALL 检查整体 MRR、Recall@5、失败率、空结果率、P95、multi-document Recall@5 与 hard-negative Recall@5；任一硬门槛失败时候选 MUST 被拒绝。

#### Scenario: Hard-negative improves without overall regression
- **WHEN** 意图候选满足全部整体和分层门槛，且 hard-negative Recall@5 不回退
- **THEN** 报告可将其冻结为 Development 候选并保存有效配置与逐项 Gate

#### Scenario: Hard-negative improves but overall MRR regresses
- **WHEN** hard-negative 指标提高但整体 MRR 超过允许回退
- **THEN** Gate 拒绝该候选，不得用分层收益抵消整体失败

### Requirement: Evaluation isolation and evidence boundary
本轮设计与调参 MUST 只使用声明为 Development 的数据与报告。当前及历史 sealed holdout 的查询、标签和逐案例结果 MUST NOT 进入规则设计、词表、候选选择或 Gate；形成候选后 SHALL 要求新一代独立 holdout 才能声称泛化验证。

#### Scenario: Development candidate is frozen
- **WHEN** 单一意图候选通过 Development Gate
- **THEN** 系统保存候选但不更新生产默认开关，并明确仍缺新一代独立 holdout 和线上验证
