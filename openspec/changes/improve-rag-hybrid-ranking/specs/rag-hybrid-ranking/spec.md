## Purpose

为 OpsCaptain 的知识检索提供不依赖额外大模型调用、可配置且可解释的 Hybrid 召回与排序契约，并通过分层 Development 误差分析提升多文档、困难干扰和跨语言查询的检索质量。

## ADDED Requirements

### Requirement: Structured knowledge document fields
知识文档进入词法检索时 SHALL 保留稳定文档 ID、标题、Provider、Tags 与正文的字段边界。系统 MUST NOT 通过无差别重复拼接全部元数据来改变正文词频，并 SHALL 在字段缺失时继续使用正文检索而不导致请求失败。

#### Scenario: Index document with structured metadata
- **WHEN** 知识文档包含标题、Provider、Tags 和稳定文档 ID
- **THEN** 词法索引分别记录这些字段，查询可命中字段信息且正文长度归一化不受无关元数据重复影响

#### Scenario: Index legacy document without metadata
- **WHEN** 文档只有 ID 和正文，没有可解析的结构化字段
- **THEN** 系统仍可索引并检索该文档，结构化字段加分视为零

### Requirement: Configurable weighted hybrid fusion
Hybrid 检索 SHALL 允许从 `manifest/config/config.yaml` 配置 Dense 与 Lexical 排名贡献权重。权重 MUST 为非负值且至少一个大于零；配置缺失或非法时系统 SHALL 返回可识别配置错误或使用明确记录的安全默认值，不得产生非确定排序。

#### Scenario: Fuse dense and lexical rankings with configured weights
- **WHEN** Dense 与 Lexical 均返回候选且配置了有效权重
- **THEN** Fusion 使用声明的权重计算候选顺序，并在 trace 中记录两个来源的排名与融合分数

#### Scenario: Candidate appears in one retrieval channel
- **WHEN** 候选只出现在 Dense 或 Lexical 结果中
- **THEN** 候选仍参与 Fusion，其分数只包含实际出现通道的贡献

### Requirement: Explainable deterministic field-aware refinement
系统 SHALL 在 CandidateTopK 截断前使用标题、Tags、Provider 和稳定文档 ID 的查询匹配信号进行确定性精排。字段加分 SHALL 可配置、可关闭，并 MUST 在逐文档 trace 中记录命中字段、加分值和精排前后位置。

#### Scenario: Exact title or tag match promotes a relevant document
- **WHEN** 查询明确包含候选文档的标题词项、稳定 ID 词项或 Tag，且候选已被 Dense/BM25 召回
- **THEN** 系统按照配置施加字段加分并记录排序变化，不调用 LLM

#### Scenario: Field-aware refinement is disabled
- **WHEN** 字段精排开关关闭
- **THEN** 候选顺序只由 Hybrid Fusion 和已有兼容逻辑决定，字段加分为零

### Requirement: Bounded multi-document coverage
系统 SHALL 继续分离 CandidateTopK 与 FinalTopK，并 SHALL 在不扩大 FinalTopK 的前提下支持可配置的确定性覆盖策略，避免多文档查询的前若干结果被同一文档族或同一主题重复占满。覆盖策略不得使用评测标签或相关文档 ID。

#### Scenario: Multi-document query has candidates from multiple topics
- **WHEN** 候选池包含与查询多个子主题匹配的文档且覆盖策略开启
- **THEN** 系统可在 FinalTopK 内选择覆盖更多查询词项或主题的文档，并记录选择理由

#### Scenario: Coverage strategy has insufficient signals
- **WHEN** 查询或候选没有足够的字段/词项信息判断覆盖差异
- **THEN** 系统保持确定性排序结果，不凭空构造主题或扩大最终文档数

### Requirement: Stratified development error report
评测系统 SHALL 按 category、difficulty、language 汇总 Recall@K、Hit Rate@K、MRR、失败率和样本数，并 SHALL 输出未完全召回案例及缺失相关文档 ID 的频次。每个分层指标的分母 MUST 明确且逐案例结果 SHALL 保留。

#### Scenario: Generate stratified report
- **WHEN** Development 评测完成
- **THEN** 报告包含整体指标、分层指标、Recall@5 未完全命中的案例清单和缺失文档频次

#### Scenario: A stratum has no samples
- **WHEN** 请求汇总的某个分层值没有样本
- **THEN** 报告明确记录样本数为零且不输出误导性的质量比例

### Requirement: Subgroup-aware non-regression gate
候选配置除满足整体 MRR、Recall@5、失败率、空结果率和 P95 门槛外，SHALL 同时满足可配置的 multi-document 与 hard-negative Recall@5 非回归门槛。任一硬门槛失败时候选 MUST 被拒绝，不能用其他分层收益抵消。

#### Scenario: Overall quality improves but hard-negative regresses
- **WHEN** 候选整体 MRR 提升但 hard-negative Recall@5 超过允许回退
- **THEN** Gate 标记为失败并指出对应分层指标

#### Scenario: Candidate satisfies all overall and subgroup gates
- **WHEN** 候选的整体、分层、可靠性和延迟指标全部满足配置门槛
- **THEN** Gate 标记为通过并列出每项基线、候选、差值和限制

### Requirement: Evaluation generation isolation
本轮参数选择 MUST 只使用声明为 Development 的数据和报告。上一轮 sealed holdout 的查询、标签和逐案例结果 MUST NOT 用于特征设计、权重选择或候选比较；若形成新候选，系统 SHALL 要求使用新版本、独立指纹且未参与开发的新 holdout 才能声称完成新一轮泛化验证。

#### Scenario: Attempt to use previous holdout as a tuning baseline
- **WHEN** 操作者把上一轮 holdout 报告作为本轮候选调参或 Gate 基线
- **THEN** 流程拒绝该比较或将其明确标记为不可用于候选选择

#### Scenario: Development candidate is frozen
- **WHEN** 单一候选通过全部 Development Gate 并冻结
- **THEN** 系统保存候选配置与 Development 指纹，但在获得新的独立 holdout 前不修改默认配置、不宣称泛化收益

### Requirement: Offline evidence boundary
所有本轮报告 SHALL 标注代码版本、数据身份、语料版本、有效配置和离线证据环境。报告与实施记录 MUST NOT 将 Development 改进表述为生产效果、线上收益或线上可用性证明。

#### Scenario: Present a successful development candidate
- **WHEN** Development 候选通过 Gate
- **THEN** 输出明确说明结果仅适用于该离线数据与依赖环境，并列出尚缺的新一代独立 holdout 或线上验证
