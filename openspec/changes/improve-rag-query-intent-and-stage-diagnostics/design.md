## Context

参见 `proposal.md` 和 `specs/rag-query-intent-diagnostics/spec.md`。当前冻结候选使用 Dense/BM25、等权 RRF、字段精排，Development MRR `0.7097`、Recall@5 `0.8111`，但报告只保留 FinalTopK 文档 trace。14 个不完整案例中，hard-negative 主要包含“只想/不是/不执行”等对比语义；目前这些词项全部进入普通 BM25 和字段匹配，无法区分目标与排除概念。

## Goals / Non-Goals

**Goals:**

- 在不改变请求主链路的前提下定位相关文档在哪个检索阶段丢失。
- 用有限规则解析明确的正向/排除意图，并对相邻文档进行可解释、有上限的精排。
- 保持字段增强候选作为同代基线，只运行一个意图候选并形成记录。

**Non-Goals:**

- 不做 LLM Rewrite/Rerank、多查询分解、通用语义否定理解或同义词知识库。
- 不用 Development 标签生成规则或词表，不修改知识正文。
- 不扩大 CandidateTopK、FinalTopK 或 ContextEngine 文档预算。
- 不复用任何 sealed holdout，也不更新生产默认开关。

## Decisions

### 1. 在 HybridTrace 捕获阶段文档身份，而不是从 Final trace 反推

Hybrid 检索已有 Dense、Lexical、Fusion、Candidate 数量，但数量无法回答“哪个文档在哪一层丢失”。扩展 trace 保存各阶段按顺序的规范化文档 ID，评测适配层再用案例标签计算阶段 Recall。运行时 trace 不携带相关标签，因此不会改变检索行为。

为控制在线开销，阶段 ID 使用现有 canonical key，仅保存到有界的 DenseTopK、LexicalTopK、CandidateTopK；报告中保留逐案例阶段排名和缺口类型。备选方案是只在 CLI 里重新执行检索，但会重复请求且无法保证与最终查询同一次运行，因此不采用。

### 2. 解析对比片段，不维护答案导向的领域黑名单

新增 `QueryIntent`：`PositiveTerms`、`ExcludedTerms`、`Rule`、`ContrastClause`。解析顺序采用少量配置化连接词：

1. 找到最明确的对比连接词，如“而不是”“不是”“不要”“不需要”“不执行”；
2. 连接词前为正向片段、后为排除片段；若出现“只想/只查/只讨论”，去掉引导词但保留后续目标；
3. 使用现有 token 规则生成集合并设置最大词项数；任一侧为空时放弃排除解析。

该方案处理显式 hard-negative，不试图理解所有中文否定。备选方案是按具体案例维护 `rollback`、`metrics server` 等黑名单，但会从 Development 标签泄漏且不可泛化，因此禁止。

### 3. 排除信号只做候选精排，不从 Dense/BM25 查询中删除

Dense 与 BM25 继续使用原始 query，避免错误解析导致召回丢失。意图信号进入 CandidateTopK 前的确定性 refinement：

- 查询正向词项与文档正文/标题/Tags/ID 重叠时获得小幅加分；
- 只有文档命中排除词项且正向重叠不足时才扣分；
- 同时命中两侧时只计算有上限的净分，不过滤文档；
- trace 保存 positive hits、excluded hits、bonus、penalty 与规则名。

配置默认关闭，候选通过 Development 也不改默认。备选方案是将排除词从检索 query 删除，但会改变 Dense 语义输入并难以定位召回缺口，因此本轮不采用。

### 4. 阶段指标先于候选判断

报告 schema 再升级一代，增加：

- 各阶段 Recall@1/3/5/20（阶段不足 K 时按实际返回）；
- 每案例相关文档的首次/最后出现阶段、阶段排名、缺口分类；
- 查询意图与逐文档意图 trace；
- 意图解析覆盖率、应用率和降权次数。

先关闭意图能力生成 v5 兼容基线，再只开启意图精排运行一个候选。Gate 沿用整体与分层非回归，额外要求 hard-negative Recall@5 不回退，不根据逐案例结果追加规则。

### 5. 保持评测代际隔离

当前 60 条 Development 已多次用于设计，只能用于本轮受限消融。若候选通过，记录为 Development 冻结候选，不修改默认配置；下一步必须建立新的独立 holdout。现有 sealed holdout 仅作为禁止复用的历史身份，不读取逐案例内容。

## Risks / Trade-offs

- [规则误把普通否定句切成排除意图] → 只支持明确对比连接词，任一侧为空即降级为无意图，默认关闭。
- [排除降权伤害同时讲解两个概念的综合文档] → 不过滤文档，只有缺少正向信号时才扣分，penalty 有上限并由整体 MRR Gate 约束。
- [阶段 ID 增加 trace 体积] → 仅保存配置 TopK 内的规范化 ID，不保存正文或向量分数副本。
- [继续在同一 Development 上优化导致过拟合] → 只运行一个候选，不新增案例专属词表，胜出后要求新一代独立 holdout。

## Migration Plan

1. 增加兼容默认配置与阶段 trace，关闭意图能力生成 schema v5 Development 基线。
2. 实现并测试确定性意图解析和有界精排，运行唯一候选。
3. 若候选通过全部 Gate，仅在实施记录冻结配置；生产默认保持关闭。
4. 回滚只需关闭意图 feature flag；阶段诊断属于只读观测，可独立保留。
