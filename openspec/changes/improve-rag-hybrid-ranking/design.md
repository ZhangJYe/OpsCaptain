## Context

参见 `proposal.md` 的动机和 `specs/rag-hybrid-ranking/spec.md` 的行为契约。当前生产链路并行执行 Dense 与 BM25，使用固定等权 RRF 合并，再通过 `refineRetrievedDocs` 做确定性排序，最终裁剪 CandidateTopK 与 FinalTopK。现有实现存在三个与 Development 误差直接相关的限制：

- BM25 预热只把 `_source` 文件名作为普通 metadata 与正文 token 混在一起；Markdown 标题、Provider、Tags 和稳定文档 ID 没有字段边界。
- RRF 对 Dense/Lexical 固定各贡献 `1/(k+rank)`，无法验证偏向精确词法命中是否能改善细粒度上游文档召回。
- 确定性精排主要面向 AIOps telemetry 字段；对知识文档标题、Tags、Provider 和稳定 ID 没有显式信号，且报告没有分层误差和逐文档排序解释。

上一轮 sealed holdout 已经用于上一代配置的独立验证，不能再参与本轮设计或调参。本轮基线是 `development-v3-hybrid-retrieve.json`：MRR `0.7061`、Recall@5 `0.7944`、P95 `172ms`。multi-document Recall@5 为 `0.7083`，hard-negative 为 `0.6000`。

## Goals / Non-Goals

**Goals:**

- 在不新增网络模型调用的情况下，提高细粒度文档和多文档问题的召回/排序质量。
- 让每个排序变化可由 Dense、Lexical、标题/Tags/ID 匹配或覆盖选择解释。
- 保持 CandidateTopK/FinalTopK 和 ContextEngine token budget 边界不变。
- 用严格限定的 Development 消融与候选 Gate 决定是否形成下一代冻结候选。

**Non-Goals:**

- 不重新运行或查看上一轮 sealed holdout 的逐案例结果指导调参。
- 不更换 Embedding、Milvus、BM25 算法族或知识文档正文内容。
- 不启用 LLM Rewrite/Rerank，不引入同义词 LLM 扩写。
- 不在本轮更新生产默认参数；通过 Development 只代表形成冻结候选，必须另建新一代独立 holdout 才能晋级。
- 不通过修改相关标签、合并相关文档 ID 或放宽指标来制造提升。

## Decisions

### 1. 解析 Markdown 头部字段，而不是复制全文制造词频

新增一个小型确定性解析器，从知识 Markdown 中提取：

- 首个一级标题作为 `title`；
- `Provider:` 列表项作为 `provider`；
- `Tags:` 逗号分隔项作为 `tags`；
- 文件名去扩展名作为稳定 `doc_id`；
- 相对路径/文件名继续作为 `source`。

BM25 文档内部保留正文 token 与各字段 token。正文使用现有 BM25 长度归一化；字段匹配作为独立、可配置的附加分数，不把 title/tags 重复拼接多次。这比简单把全部 metadata 加入正文更可解释，也避免长 URL 和 license 文本污染词频。

备选方案是修改知识 Markdown 正文、人工增加中文别名，但会把调参信息写入语料并难以区分索引优化与内容优化，因此不采用。

### 2. 在 RRF 保留排名鲁棒性的基础上增加通道权重

RRF 公式调整为：

`dense_weight / (fusion_k + dense_rank) + lexical_weight / (fusion_k + lexical_rank)`

默认权重均为 `1.0`，从而保持现有行为。权重进入 `HybridConfig`、context override、有效配置报告和可比性校验。候选仅允许预先声明的少量组合，例如 `1.0/1.25` 与 `1.0/1.5`；不做网格搜索。

备选方案是直接归一化并线性混合 Dense/BM25 原始分数，但两个检索器分数尺度不同、版本变化敏感，会降低可复现性，因此继续使用 rank-based fusion。

### 3. 知识字段加分并入现有确定性精排

扩展文档 profile，加入 `docID`、`titleTokens`、`tagTokens` 和 `providerTokens`。在保留原始融合位置基础分的前提下，分别计算：

- 文档 ID 或标题短语精确包含；
- 查询 token 与标题 token 重叠；
- 查询 token 与 Tags/Provider 重叠。

每类权重和上限由 `config.yaml` 配置，并在文档 metadata trace 中记录命中字段、加分值和精排前后位置。当前 telemetry 字段逻辑继续保留；知识字段为空时行为不变。

备选方案是加入新的交叉编码器或 LLM rerank。上一轮实际观察到 Rerank P95 超过 5 秒且高比例降级，不满足本轮低延迟目标，因此不采用。

### 4. 多文档覆盖只在 FinalTopK 选择阶段进行轻量贪心调整

对已经排序的 CandidateTopK，计算每个文档能够新增覆盖的查询 token/字段 token。选择 FinalTopK 时，以原排序为主，只在候选分数接近且新增覆盖明确时提升能够覆盖尚未覆盖词项的文档。该功能使用 feature flag 和最大位置提升限制，避免大幅破坏 MRR。

实现不得读取 category、relevant_ids、distractor_ids 等评测字段。若没有新增覆盖信号，严格保持原顺序。

备选方案是自动做 query decomposition，但会引入模型调用或复杂规则，并与 Planner 范围重叠，因此本轮不采用。

### 5. 先补分层报告，再开始参数实验

评测报告新增：

- category/difficulty/language 的 cases、Recall@1/3/5、Hit@1/3/5、MRR、failure rate；
- Recall@5 未完全命中案例清单；
- 缺失相关文档 ID 频次；
- 文档级 Dense rank、Lexical rank、fusion score、字段加分、覆盖加分和最终位置。

Gate 保持整体 MRR 为主指标、Recall@5 为次指标，并增加 multi-document 与 hard-negative Recall@5 非回归。基线报告缺少新字段时，先用同代码关闭新能力重跑 Development v4 基线，不能直接拿报告 schema v2 强行比较。

### 6. 限定实验顺序，避免 Development 过拟合

只允许以下顺序：

1. v4 基线：所有新能力关闭或使用兼容默认值；
2. 字段解析 + 字段精排单独开启；
3. 在步骤 2 通过 Gate 的前提下，最多两个预先声明的 RRF 权重候选；
4. 只有仍存在多文档覆盖缺口时，运行一个覆盖策略候选；
5. 选择单一 Development 胜出候选并冻结。

最多四个实际候选，不根据中间逐案例结果无限追加参数。上一轮 sealed holdout 报告只保留历史记录，不进入比较命令或参数决策。

## Risks / Trade-offs

- [字段匹配过强导致标题关键词文档压过语义正确文档] → 权重可配置且有上限，以整体 MRR 和 hard-negative 非回归 Gate 约束。
- [Tags 来自上游文档，英文 query 提升但中文同义表达仍不足] → 本轮只验证可解释字段信号；同义词表或内容增强另立 change，避免混杂变量。
- [覆盖策略改善 multi-document 但损害首位准确率] → 仅在分数接近时有限提升，整体 MRR 是硬门槛。
- [Development 反复调参造成过拟合] → 候选数上限为四，参数预先声明，旧 holdout 禁止复用，候选冻结后需新独立 holdout。
- [报告 schema 变化使旧基线不可比] → 先在兼容默认配置下生成 v4 Development 基线，再比较同 schema 报告。
- [Markdown 格式不统一导致字段缺失] → 解析器容错，缺失字段退回正文与现有排序，不使请求失败。

## Migration Plan

1. 新增配置项时使用兼容默认值：RRF 权重 `1.0/1.0`，字段精排和覆盖策略默认关闭。
2. 实现字段解析、trace 和分层报告并用单元测试确认关闭开关时排序兼容。
3. 生成 v4 Development 基线，按限定顺序运行消融和候选。
4. 若无候选通过全部 Gate，保持默认配置不变并只保存误差报告；若有候选通过，仅记录为冻结 Development 候选。
5. 另建新一代独立 holdout 后才允许决定是否更新默认配置。回滚只需关闭 feature flag 或恢复权重 `1.0/1.0`。
