# OpsCaptain 面试回答手册

> 按模块组织，每个模块使用 STAR 法则，附带常见面试 Q&A。

---

## 目录

1. [记忆系统](#1-记忆系统)
2. [RAG 系统](#2-rag-系统)
3. [上下文管理](#3-上下文管理)
4. [上下文压缩](#4-上下文压缩)
5. [Agent 架构](#5-agent-架构)
6. [后端工程](#6-后端工程)
7. [Skill 系统](#7-skill-系统)
8. [MCP 工具系统](#8-mcp-工具系统)
9. [安全系统](#9-安全系统)
10. [信念图与 FSM](#10-信念图与-fsm-belief-engine-核心)
11. [防幻觉管线](#11-防幻觉管线-events-系统)
12. [多 Agent 运行时](#12-多-agent-运行时-runtime)
13. [AIOps 服务编排](#13-aiops-服务编排)
14. [LLM 模型抽象与弹性工程](#14-llm-模型抽象与弹性工程)
15. [部署架构](#15-部署架构)

---

## 1. 记忆系统

### STAR 讲法

**Situation：**

OpsCaptain 是 AIOps 助手，用户会反复咨询同一类问题（如"payment-service 的告警怎么处理"）。每次对话都是独立的，LLM 不记得用户之前问过什么、有什么偏好、遇到过什么问题。这导致用户每次都要重复描述上下文。

**Task：**

设计一个双层记忆系统：短期记忆管理当前对话窗口，长期记忆跨会话持久化用户的事实、偏好和排障经验。要求记忆提取不能阻塞主对话流程，检索要快，还要能自动淘汰过时信息。

**Action：**

1. **短期记忆（SimpleMemory）**：滑动窗口 20 条消息，超出时自动摘要压缩。摘要截断到 2000 字符，保留首尾 60%+30%。全局会话池管理 500 个会话，TTL 2 小时。

2. **长期记忆（LongTermMemory）**：四种类型（fact/preference/procedure/episode），四级作用域（session/user/project/global）。ID 用 `sha256(scope+content)` 前 8 字节，天然去重。支持 File 和 Redis 两种持久化后端。

3. **记忆提取**：双模式——规则提取器（零延迟，关键词匹配）和 LLM 提取器（更智能，结构化决策）。LLM 失败自动降级到规则模式。提取在 goroutine 中异步执行，信号量限流 8 并发，不阻塞主对话。

4. **记忆检索**：BM25 分词匹配 + 时间衰减（24h 半衰）+ 频率加成（每次 +0.3，上限 3.0）。Jaccard 去重（阈值 0.8）避免返回重复记忆。

5. **冲突淘汰**：同类同域的新记忆自动淘汰旧记忆（supersede），置信度减半，立即过期。

**Result：**

- 记忆提取延迟 < 50ms（规则模式），不阻塞对话
- 记忆检索支持 BM25 分词匹配，适合运维领域的结构化关键词
- 自动淘汰过时信息，无需手动清理
- 四级作用域支持记忆晋升（session → user → project → global）

### 面试 Q&A

**Q：为什么用 BM25 而不是向量检索做记忆检索？**

> 运维场景的记忆内容通常是结构化的关键词（IP、服务名、端口、错误码），BM25 的精确匹配比向量检索更适合。而且记忆条目数量有限（上限 1000），全内存 BM25 延迟接近零。

**Q：记忆提取为什么不阻塞主对话？**

> 记忆提取在 goroutine 中异步执行，通过信号量限流（默认 8 并发）。主对话流程不等待提取完成，用户立即得到回复。提取超时 1500ms，信号量等待超时 50ms。

**Q：如何处理冲突的记忆？**

> 使用 ConflictGroup 机制。当新记忆与已有记忆属于同一冲突组、同一类型、同一作用域时，旧记忆被标记为 "superseded"，置信度减半，立即过期。这样用户更新偏好时，旧偏好自动失效。

**Q：记忆的置信度是怎么设计的？**

> 按类型递减：preference(0.85) > procedure(0.75) > fact(0.70) > episode(0.50)。LLM 提取时可以自定义置信度，规则提取用默认值。检索时低于 0.50 的记忆被过滤。

---

## 2. RAG 系统

### STAR 讲法

**Situation：**

OpsCaptain 需要从知识库中检索相关文档来回答用户问题。知识库包含运维文档、排障手册、配置指南等。用户的问题可能是口语化的（"payment-service 挂了怎么办"），也可能是精确的（"查询 CPU 使用率的 PromQL"）。单一检索方式难以覆盖所有场景。

**Task：**

设计一个混合检索系统，结合向量语义检索和 BM25 词法检索的优势，通过 RRF 融合提升召回率。支持 LLM 驱动的查询改写和重排序。

**Action：**

1. **Query Rewrite**：使用 GLM Fast 模型（3s 超时）将用户口语化查询改写为更适合检索的形式。支持 `RewriteQueryMulti()` 生成多个多样化查询。失败时 graceful fallback 返回原始查询。

2. **Hybrid Retrieval**：两个 goroutine 并行执行 Milvus 向量检索（DenseTopK=50）和 BM25 词法检索（LexicalTopK=50）。BM25 实现自研，支持 CJK 单字+bigram 和英文 token 分词，k1=1.2, b=0.75。

3. **RRF 融合**：`score(d) = 1/(k + rank_dense) + 1/(k + rank_lex)`，k=60。同一文档双路命中时分数叠加。按总分降序排序取 CandidateTopK。

4. **Metadata Refine**：基于元数据的规则重排——服务名、错误码、指标名等关键词匹配 boost。非 LLM，零延迟。

5. **LLM Rerank**：使用 GLM Fast 模型对候选文档打分（0-10 分），按分数降序取 FinalTopK。超时 2s，降级到关键词顺序。

6. **文档索引**：两阶段分块——先按 Markdown Header 切分保留语义边界，再对过长 chunk 做语义切分。元数据通过 `.metadata.json` sidecar 文件注入。

**Result：**

- RRF 融合自动奖励双路命中，无需调权重
- BM25 全内存实现，零延迟
- LLM Rerank 复用已有基础设施，无需部署专用 rerank 模型
- QueryTrace 全链路打点，每阶段延迟、命中数完整记录

### 面试 Q&A

**Q：为什么用 RRF 而不是加权融合？**

> RRF 只依赖排名不依赖分数，对不同尺度的分数天然鲁棒。向量检索返回余弦相似度（0-1），BM25 返回 TF-IDF 分数（可能很大），直接加权需要归一化。RRF 的 `1/(k+rank)` 公式自动处理了这个问题。

**Q：BM25 的中文分词怎么做的？**

> 自研分词，CJK 字符按单字切分 + 相邻二字 bigram。比如"支付服务"分词为["支", "付", "服", "务", "支付", "付服", "服务"]。避免了外部依赖（如 jieba），满足中英文混合场景。

**Q：文档分块策略是什么？**

> 两阶段分块：先按 Markdown Header（# / ## / ### / ####）切分，保留标题作为 metadata。超过 800 字符的 chunk 再做语义切分，使用 Doubao Embedding 计算相似度，分隔符包括 `\n\n`、`\n`、句号等。

**Q：如何保证多租户数据隔离？**

> 双层防护：Milvus expression filter 在服务端过滤（基于 owner 字段），`safeRetriever` 在客户端过滤兜底。filter 失败时自动降级重试。

---

## 3. 上下文管理

### STAR 讲法

**Situation：**

OpsCaptain 的 LLM 需要从多个来源获取上下文：对话历史、长期记忆、RAG 文档、工具调用结果。这些内容的总 token 数可能远超 LLM 的上下文窗口。而且不同场景（聊天 vs AIOps 诊断）需要不同的上下文策略。

**Task：**

设计一个上下文引擎，根据场景动态组装上下文包，按 token 预算分配各来源的容量，并支持意图识别动态切换策略。

**Action：**

1. **Profile 机制**：四种模式（chat/aiops/aiops_diagnosis/specialist），每种模式有独立的 ContextProfile，包含功能开关和 token 预算。比如 aiops_diagnosis 启用历史、文档、工具结果，chat 模式可能禁用工具结果。

2. **五阶段组装流水线**：
   - Stage 0：策略解析 + 意图识别（LLM 500ms 超时，故障诊断自动升级 profile）
   - Stage 1：历史选择（Embedding 语义召回 + 滑动窗口，pair-aware 选择）
   - Stage 2：记忆选择（BM25 检索 + 综合排序：confidence 50% + freshness 30% + scope 20%）
   - Stage 3：文档选择（RAG 检索 + 压缩 + budget 裁剪）
   - Stage 4：工具结果选择（关键词召回 + LLM 重排 + 压缩）

3. **Token 预算分配**：总预算 8000 tokens，System 20% / History 40% / Memory 20% / Document 15% / Tool 10%。每个选择函数维护 `remaining` 计数器，超预算的 item 被 trim 或 drop。

4. **渐进式降级**：每个外部依赖（Embedding、LLM 意图识别、LLM 重排）都有超时和 fallback。历史召回降级到位置选择，意图识别默认 knowledge_query，工具重排降级到关键词顺序。

5. **Token 估算**：CJK 字符 1.5 token/字，ASCII 0.25 token/字符。Trim 时截取 90% 长度，对齐到最近换行符。

**Result：**

- 同一引擎服务聊天和诊断两种场景，通过 Profile 切换
- 意图识别动态升级 profile，用户在聊天模式问故障自动获得诊断上下文
- 全链路 trace 观测，每阶段 selected/dropped/latency 完整记录
- 渐进式降级保证任何环节失败都不中断主流程

### 面试 Q&A

**Q：为什么历史选择用 Embedding 而不是 BM25？**

> 历史消息是自然对话，语义相似度比关键词匹配更有效。比如用户问"之前那个服务的问题解决了没"，BM25 可能匹配不到，但 Embedding 能找到语义相关的历史消息。而且历史消息数量有限（默认 20 条），Embedding 计算成本可控。

**Q：pair-aware 选择是什么意思？**

> 当用户消息被语义召回选中时，对应的助手回复也自动包含，反之亦然。这样保证对话的完整性，LLM 能看到完整的问答对。

**Q：记忆和历史有什么区别？**

> 历史是原始对话消息，记忆是从对话中提取的结构化知识。历史是"用户说了什么"，记忆是"用户记住什么"。比如用户说"我们用的是 Redis 6.2"，历史记录这句话，记忆提取为 `fact: Redis 版本 6.2`。

**Q：token 预算怎么分配的？**

> 总预算 8000 tokens，按比例分配：System 20%（1600）、History 40%（3200）、Memory 20%（1600）、Document 15%（1200）、Tool 10%（800）。aiops_diagnosis 模式下 Tool 提升到 15%。每个来源独立消耗预算，互不影响。

---

## 4. 上下文压缩

### STAR 讲法

**Situation：**

OpsCaptain 的工具输出（Prometheus JSON、K8s events、日志）和 RAG 文档经常很长，大部分内容是正常的、冗余的，只有少量异常项有价值。旧逻辑靠长度截断控制 token，但可能切掉中间位置的关键错误。

**Task：**

设计一个可配置、可回滚、可评测的上下文压缩系统，目标是在不损害 evidence 的前提下降低 prompt token。支持 audit（只观察不压缩）和 optimize（实际压缩）两种模式。

**Action：**

1. **三种压缩策略**：
   - JSON 数组：保留前 N 项 + 后 M 项 + 错误关键词项 + query 命中项
   - 日志/文本：保留 ERROR/WARN 行 + 上下文窗口 + query 命中行 + 首尾行，去重连续重复行
   - JSON 对象：提取 error/message/status 等关键字段

2. **两种模式**：
   - Audit：执行压缩候选分析，记录 candidate/runtime 指标，不改变实际内容
   - Optimize：只在压缩确实省 token 时替换内容，否则保留原文

3. **三层接入**：
   - Tool wrapper 层：`CompressAfterToolCall` 包装器
   - ContextEngine tool items：`selectToolItemsWithCompression`
   - ContextEngine RAG docs：`selectDocuments` 中压缩

4. **评测体系**：
   - 32 个合成样本（Prometheus JSON、K8s events、应用日志、RAG 文档等）
   - 5 个真实文档样本（CCF AIOps 数据集）
   - 核心指标：evidence recall（必须 100%）、compression ratio、degraded count

**Result：**

- 合成样本压缩率 4.65%，真实文档压缩率 78.81%
- 证据保留率 100%，0 降级
- 纯 CPU 操作，P95 延迟 0ms
- 默认关闭，生产行为不变

### 面试 Q&A

**Q：为什么合成样本和真实文档的压缩率差这么多？**

> 合成样本平均 244 tokens，本身就很短，压缩空间有限。真实 RAG 文档平均 10K tokens，内容更长、冗余更多，压缩收益显著。这说明压缩在长文档场景下确实有价值。

**Q：RAG 文档压缩会影响召回率吗？**

> 不会。压缩发生在检索之后、发送给 LLM 之前。召回率由 Milvus 向量检索决定，压缩只影响发送给 LLM 的 token 数。

**Q：audit 模式有什么用？**

> Audit 模式记录"如果压缩会怎样"的指标，但不改变实际内容。用于无风险观察收益潜力，验证压缩策略是否安全。如果 audit 显示有 saving，说明可以安全启用 optimize。

**Q：为什么 evidence recall 必须 100%？**

> AIOps 场景中，压错比少省 token 更危险。如果压缩去掉了 CrashLoopBackOff 或 OOMKilled 这样的关键证据，LLM 可能给出错误的诊断结论。宁可不压缩，也不能丢证据。

---

## 5. Agent 架构

### STAR 讲法

**Situation：**

AIOps 故障诊断是复杂的多步推理任务。用户描述一个症状（"payment-service 响应慢"），Agent 需要收集证据、形成假设、验证假设、最终给出诊断结论。不同类型的故障需要不同的专家知识。

**Task：**

设计双引擎架构：GoS Belief Engine 用于结构化根因分析（信念图 + FSM 收敛），Plan-Execute-Replan 用于灵活的多步骤排查。两者共享同一套工具和专家体系。

**Action：**

1. **GoS Belief Engine**：
   - BeliefGraph：有向图，三类节点（Signal/Evidence/Hypothesis），四种边（support/refute/refines/causal）
   - BeliefFSM：三状态机（Drilling/Reporting/Done），基于 GapDelta/MinSupport/MaxSteps 阈值决策
   - 主循环：Ingest → ExtractFrontier → PickExpert → Act → UpdateGraph → FSM.Decide
   - Copy-on-Write 图更新，保证专家并行执行的并发安全

2. **Plan-Execute-Replan**：
   - 基于 Eino 框架的 `planexecute` 组件
   - Planner 生成计划，Executor 执行工具调用，Replanner 根据结果调整计划
   - 工具集：MCP 日志、Prometheus 告警/指标、内部文档查询
   - 降级报告机制：模型未返回结论时根据执行过程生成 fallback 报告

3. **Expert Agent**：
   - BaseExpert 实现多步检索-决策循环：tool_call → retrieve → analyze
   - 三个具体专家：LinuxSRE、NetworkSRE、DatabaseSRE
   - 关键词匹配选择专家（非 LLM），零延迟
   - 工具输出脱敏（API key、token、邮箱、IP）

4. **Chat Pipeline**：
   - DAG 编排：InputToChat → ChatTemplate → ReactAgent
   - 工具渐进披露：AlwaysOn + SkillGate 两层，注入攻击时只暴露 AlwaysOn
   - Prompt 动态组装：静态部分 + 安全警告 + 技能提示 + 运行时配置

5. **契约系统**：
   - 三个 specialist 的标准化契约：Role/Responsibilities/Must/MustNot/EvidencePolicy
   - `EnforceContract()` 验证结果，degraded 必须有原因，succeeded 必须有摘要

**Result：**

- 双引擎覆盖不同诊断场景：GoS 适合结构化收敛，Plan 适合灵活排查
- 渐进式降级：tool → RAG → LLM，任何环节失败不中断诊断
- 契约驱动约束 specialist 行为边界
- 完整 eval 框架：golden case 回归 + LLM-as-Judge 质量评分

### 面试 Q&A

**Q：GoS 和 Plan-Execute-Replan 有什么区别？分别适合什么场景？**

> GoS 是信念图驱动的迭代收敛模型，适合有明确假设空间的故障诊断。比如"服务响应慢"，GoS 会生成资源耗尽、网络问题、配置错误三个假设，然后通过专家收集证据逐步收敛。Plan-Execute-Replan 是线性执行模型，适合需要多步骤操作的场景，比如"查询最近 1 小时的告警并分析趋势"。

**Q：BeliefGraph 的 Copy-on-Write 是怎么实现的？**

> 每次更新图时，先复制一份当前图的快照，在快照上修改，然后原子替换。这样多个专家可以并行执行，互不干扰。读操作直接读当前图，不需要加锁。

**Q：FSM 的三个阈值怎么调？**

> GapDelta（默认 0.3）控制假设收敛的严格程度，越大越保守。MinSupport（默认 2）控制最少证据数，防止过早收敛。MaxSteps（默认 3）控制每层最大步数，防止无限循环。实际调优需要根据诊断场景的复杂度来平衡。

**Q：工具渐进披露是怎么工作的？**

> 工具分为两层：AlwaysOn 始终可用（如基础查询工具），SkillGate 按技能门控（如 Prometheus 工具只在指标域启用）。当检测到注入风险时，只暴露 AlwaysOn 工具，防止恶意调用。

**Q：为什么专家选择用关键词匹配而不是 LLM？**

> 关键词匹配零延迟零成本，而且运维场景的关键词非常明确（"网络/network/latency" → NetworkSRE）。用 LLM 选择专家会增加延迟和成本，而且可能选错。简单规则在明确场景下比 LLM 更可靠。

---

## 6. 后端工程

### STAR 讲法

**Situation：**

OpsCaptain 是一个生产级 AIOps 平台，需要处理高并发对话请求、异步任务、实时 SSE 推送、多租户隔离等工程挑战。技术栈是 Go 1.24 + GoFrame v2 + ByteDance Eino 框架。

**Task：**

设计一个高可用、可扩展的后端架构，支持同步/异步对话、实时进度推送、安全认证、限流熔断等企业级需求。

**Action：**

1. **API 设计**：
   - 同步对话：`POST /api/chat`，阻塞等待完整回复
   - SSE 流式：`POST /api/chat_stream`，实时推送 token
   - 异步提交：`POST /api/chat_submit`，返回 task_id
   - 轮询结果：`GET /api/chat_task`，查询异步任务状态
   - AIOps 异步：`POST /api/ai_ops_runs` → `GET /api/ai_ops_result` → `GET /api/ai_ops_trace`

2. **异步任务架构**：
   - Redis 存储任务状态和结果
   - RabbitMQ 消息队列分发任务
   - Worker 消费任务并更新状态
   - 前端轮询 `chat_task` 获取结果

3. **安全层**：
   - Prompt Guard：注入检测，suspicious/unsafe 风险分级
   - Output Filter：脱敏 API key、内部 IP、系统 prompt
   - Approval Gate：高风险操作需要人工审批
   - JWT 认证 + 限流（20/min，burst 30）

4. **可观测性**：
   - Jaeger 分布式追踪，全链路采样
   - Prometheus 指标暴露
   - 结构化日志（request_id 关联）
   - ContextAssemblyTrace 上下文组装观测

5. **容错设计**：
   - 信号量限流（对话并发 50）
   - 熔断器（连续失败 5 次触发）
   - LLM 响应缓存（TTL 300s）
   - 降级开关（kill_switch 一键关闭 AI 能力）

**Result：**

- 支持同步/异步/流式三种对话模式
- 全链路可观测，问题可追溯
- 多层安全防护，防止注入和信息泄露
- 渐进式降级保证高可用

### 面试 Q&A

**Q：为什么同时支持同步、SSE、异步三种对话模式？**

> 不同场景有不同需求。同步适合简单查询（< 5s），SSE 适合需要实时反馈的长对话（AIOps 诊断可能 30s+），异步适合批量任务或需要审批的场景。前端根据场景选择合适的模式。

**Q：异步任务的状态是怎么管理的？**

> Redis 存储任务状态（pending/running/completed/failed）和结果。Worker 消费 RabbitMQ 消息执行任务，完成后更新 Redis。前端轮询 `chat_task` 获取状态。trace_id 用于关联 AIOps 的执行轨迹。

**Q：Prompt Guard 怎么检测注入？**

> 多层检测：正则匹配已知注入模式、LLM 分类器判断风险等级（safe/suspicious/unsafe）、输出 filter 脱敏敏感信息。suspicious 级别限制工具暴露，unsafe 级别直接拒绝。

**Q：缓存策略是什么？**

> LLM 响应缓存，key 是 `session_id + query_hash`，TTL 300 秒。相同会话的相同查询直接返回缓存结果，减少 LLM 调用。缓存命中率取决于用户行为模式。

**Q：如何保证多租户隔离？**

> 三层隔离：JWT token 中的 tenant_id、Milvus expression filter 按 owner 过滤、Redis key 加 tenant 前缀。API 层通过 middleware 提取 tenant_id 并注入 context。

---

## 7. Skill 系统

### STAR 讲法

**Situation：**

OpsCaptain 支持多种运维场景：指标查询、日志分析、知识库检索。不同场景需要不同的工具和推理策略。如果一次性把所有工具都暴露给 LLM，会导致工具选择困难、token 浪费、甚至误用工具。

**Task：**

设计一个 Skill 系统，通过关键词匹配自动识别用户意图，按需暴露相关工具，并支持用户自定义扩展。

**Action：**

1. **三层架构**：
   - Skill 层（语义层）：通过关键词/matcher 识别用户意图，输出 Focus hint 引导推理
   - Tool 层（执行层）：实际数据获取（Prometheus、MCP 日志、RAG）
   - Disclosure 层（编排层）：按 tier 分层暴露工具

2. **三个 Domain**：
   - Metrics（4 个 skill）：release_guard / capacity_snapshot / alert_triage / incident_snapshot
   - Logs（6 个 skill）：service_offline_panic_trace / api_failure_rate_investigation / payment_timeout_trace 等
   - Knowledge（5 个 skill）：rollback_runbook / release_sop / service_error_code_lookup 等

3. **Progressive Disclosure（渐进披露）**：
   - TierAlwaysOn：始终可用（get_current_time、query_internal_docs、query_logs）
   - TierSkillGate：域匹配后开放（Prometheus 工具、用户 MCP 工具）
   - TierOnDemand：按需展开（mysql_crud）
   - 注入风险时只暴露 AlwaysOn 工具

4. **双通道注入**：
   - AIOps 路径：Focus hint 注入 prompt，引导 LLM 推理方向
   - Chat 路径：ProgressiveDisclosure 控制工具暴露范围

5. **用户扩展**：GenericSkill + MCPInvoker，JSON 配置即注册新 skill，无需改代码

**Result：**

- 关键词匹配零延迟，比 LLM 分类更快更可靠
- 渐进披露减少 LLM 工具选择负担
- 安全分层防止恶意调用
- 用户可自定义扩展，无需改代码

### 面试 Q&A

**Q：Skill 和 Tool 有什么区别？**

> Skill 是语义层，负责"用户想做什么"——通过关键词匹配识别意图，输出 Focus hint 引导推理方向。Tool 是执行层，负责"怎么获取数据"——实际调用 Prometheus、MCP、RAG 等外部服务。Skill 的 Run() 方法内部调用具体 Tool。

**Q：Progressive Disclosure 是怎么工作的？**

> 工具分三层：AlwaysOn 始终可用，SkillGate 按域匹配开放，OnDemand 按需展开。系统先通过 Skill 匹配识别用户意图涉及的域（metrics/logs/knowledge），然后只暴露该域对应的工具。比如用户问"CPU 使用率"，匹配到 metrics 域，就暴露 Prometheus 相关工具，但不暴露日志工具。

**Q：日志 Skill 的复合 matcher 是什么？**

> 某些场景需要多个信号同时出现才触发。比如 `logs_service_offline_panic_trace` 需要 panic 信号 AND offline 信号同时存在。`logs_payment_timeout_trace` 需要 (payment/checkout) + (timeout/error)，但排除 api failure 场景。这种复合条件用自定义 matcher 函数实现，比单关键词匹配更精准。

**Q：用户怎么自定义 Skill？**

> 通过 JSON 配置定义：OutputParser（解析模式）、JSONPath（嵌套 JSON 导航）、Keywords（匹配关键词）、Tier（工具层级）、Focus（推理引导）。UserSkillLoader 从 JSON 文件加载，只注册 StatusApproved 的 skill。Reload() 时先清除旧 GenericSkill，再重新加载。

---

## 8. MCP 工具系统

### STAR 讲法

**Situation：**

OpsCaptain 需要连接多种外部服务：日志系统、Prometheus、MySQL、用户自定义的 MCP 服务器。这些服务的可用性不确定，连接可能中断。而且用户可能想接入自己的工具。

**Task：**

设计一个高可用的工具集成框架，支持 MCP 协议、内置工具、连接池复用、断线重连、三级降级，以及用户自定义 MCP 工具的审批和注册。

**Action：**

1. **日志工具（MCP 集成）**：
   - 连接池复用：按 URL 复用 `pooledClient`，双重检查锁保证并发安全
   - 断线重连：指数退避（最多 3 次，基础延迟 1s），区分连接层错误和业务超时
   - 三级降级链：SSE MCP → HTTP Fallback → 降级占位工具

2. **Prometheus 工具（内置）**：
   - AlertsQuery：调用 `/api/v1/alerts`，按 alertname 去重，计算持续时间
   - MetricsDiscovery：调用 `/api/v1/label/__name__/values`，支持 keyword 过滤
   - InstantQuery：调用 `/api/v1/query`，输出 summary（min/max/avg）
   - RangeQuery：调用 `/api/v1/query_range`，趋势分类（flat/up/down）

3. **MySQL 工具（安全策略）**：
   - 只读查询，正则匹配 17 种危险关键字
   - 表白名单 + 子查询禁用 + 自动 LIMIT 包裹
   - 表名规范化：schema.table → table

4. **用户 MCP 工具**：
   - DynamicMCPRegistry 管理用户定义的 MCP 服务器
   - CIDR 白名单校验，支持 SSE 和 HTTP 两种传输
   - 审批流程：pending → approved/rejected
   - 降级响应：任何错误返回 `degradedJSON`

5. **ToolWrapper 装饰器**：
   - Before 拦截：参数校验（非空 + 合法 JSON）
   - After 处理：结果截断 / 语义压缩
   - 事件发射：tool_call_start / tool_call_end
   - 安全设计：after 失败时返回安全错误消息，不暴露原始数据

**Result：**

- MCP 连接池 + 三级降级，保证工具始终可用
- SQL 四层防护，防止 LLM 生成的 SQL 造成破坏
- ToolWrapper 统一拦截，不侵入工具实现
- 降级而非失败，LLM 能理解限制并调整策略

### 面试 Q&A

**Q：MCP 连接池的三级降级是怎么实现的？**

> 第一级：SSE MCP 连接，带超时和断线重连（指数退避，最多 3 次）。第二级：HTTP Fallback，POST 到 `/tools/query_logs`。第三级：降级占位工具，返回结构化 JSON `{"degraded": true, "message": "日志服务暂不可用"}`。每一级失败自动降级到下一级，LLM 始终有工具可调用。

**Q：MySQL 工具的安全策略是什么？**

> 四层防护：1）正则黑名单匹配 17 种危险关键字（DROP/DELETE/UPDATE/INSERT 等）；2）表白名单，只允许查询配置的表；3）子查询禁用，防止通过子查询绕过表白名单；4）自动包裹 `SELECT * FROM (%s) AS safe_query LIMIT %d`，防止全表扫描。

**Q：ToolWrapper 的 after 失败会怎样？**

> 返回 `[工具结果处理失败]` 消息，不暴露原始数据。同时发射不含 result summary 的事件，避免泄露未脱敏数据。这样即使脱敏/校验/审计失败，用户看到的也是安全的错误提示。

**Q：用户自定义 MCP 工具的安全控制？**

> CIDR 白名单校验：解析 endpoint URL 的 hostname，DNS 解析后与白名单比对。只有 StatusApproved 的工具才会被注册到分层工具列表。支持测试模式：MCPToolTest 临时注册测试连接，成功后立即 unregister。

---

## 9. 安全系统

### STAR 讲法

**Situation：**

OpsCaptain 是面向运维的 AI 助手，处理敏感的基础设施数据。需要防止：1）Prompt 注入攻击；2）敏感信息泄露；3）高风险操作误执行；4）API 滥用。

**Task：**

设计多层安全防护体系，从输入到输出全链路保护，同时不影响正常运维操作。

**Action：**

1. **Prompt Guard（注入检测）**：
   - Layer 1：正则匹配 6 种注入模式（中英文），命中即拦截
   - Layer 2：LLM 分类器（可选），区分运维操作和注入尝试
   - 三级风险：safe / suspicious / dangerous
   - suspicious 时限制工具暴露，dangerous 时直接拦截

2. **Output Filter（输出脱敏）**：
   - 5 种模式：系统 prompt 块、API key、内部 IP（RFC 1918）
   - 正则匹配 + 替换为 `[REDACTED_*]` 标记
   - 所有聊天响应经过 filter

3. **Approval Gate（审批机制）**：
   - 三个关键词列表：高风险词、执行词、分析词
   - 关键设计：区分"分析"和"执行"
     - "请给出回滚步骤"（分析）→ 允许
     - "请立即回滚"（执行）→ 需要审批
   - Redis 队列存储审批请求，TTL 24h

4. **认证与限流**：
   - 自研 HS256 JWT，无第三方依赖
   - 三种角色：admin / operator / viewer
   - 路径级 RBAC：AIOps 操作需要 operator，审批需要 admin
   - 双后端限流：Redis 滑动窗口（优先）+ 内存令牌桶（降级）

5. **输出质量门禁**：
   - SchemaGate：内容实质性检查、自相矛盾检测
   - OutputValidator：幻觉检测（验证 LLM 输出的指标值是否在工具结果中）
   - ContractEnforcer：契约违反自动降级

**Result：**

- 多层防护：输入注入检测 → 输出脱敏 → 操作审批 → 质量门禁
- 中英文双语安全，覆盖运维场景的特殊关键词
- 分析/执行区分，不影响正常运维咨询
- 渐进式降级，任何环节失败不阻断正常流程

### 面试 Q&A

**Q：Prompt Guard 怎么区分运维操作和注入？**

> 两层设计：Layer 1 正则匹配已知注入模式（如"忽略之前的指令"、"你现在是"），命中即拦截。Layer 2 LLM 分类器（可选）理解上下文，区分"忽略告警 ABC"（运维操作）和"忽略之前的所有指令"（注入）。LLM 失败时降级到 safe，不误拦正常请求。

**Q：Approval Gate 怎么区分分析和执行？**

> 三个关键词列表：高风险词（delete/drop/rollback）、执行词（execute/apply/立即/执行）、分析词（analyze/diagnose/分析/排查）。逻辑是：必须先命中高风险词才考虑审批；如果只命中分析词没有执行词，允许（如"请给出回滚步骤"）；如果命中执行词，需要审批（如"请立即回滚"）。

**Q：JWT 为什么自研不用第三方库？**

> 减少供应链风险，实现只有约 100 行代码，可审计。使用标准 HS256 算法（crypto/hmac + crypto/sha256），安全性不低于第三方库。同时支持 token 吊销（Redis 存储已吊销 token 的 SHA256 hash）。

**Q：限流的双后端是怎么设计的？**

> 优先使用 Redis 滑动窗口（Lua 脚本原子操作），Redis 不可用时自动降级到内存令牌桶。每个客户端独立限流（默认 20/min，burst 30）。客户端识别：优先用认证用户 ID，降级用 IP 地址。

**Q：幻觉检测是怎么做的？**

> OutputValidator 从 LLM 输出中提取指标值（如"CPU 使用率 95%"），然后验证这些值是否出现在工具返回的结果中。如果 LLM 编造了工具没有返回的数据，标记为幻觉。

---

## 10. 信念图与 FSM（Belief Engine 核心）

### STAR 讲法

**Situation：**

传统 Agent 用 chain-of-thought 线性推理，一旦中间步骤出错，后续推理全部偏离。AIOps 场景中，证据可能反驳之前的假设，需要"修正推理链"的能力。

**Task：**

设计一个基于图结构的推理引擎，支持假设的提出、证据的支撑/反驳、假设的撤回与替代，并通过 FSM 控制推理的收敛。

**Action：**

1. **BeliefGraph（信念图）**：
   - 三类节点：Signal（原始症状）、Evidence（工具/RAG 证据）、Hypothesis（候选根因）
   - 四种边：support（支撑）、refute（反驳）、refines（细化）、causal（因果）
   - Copy-on-Write 更新：每次修改先复制快照，原子替换，保证专家并行执行的并发安全
   - 节点撤回（retract）与替代（supersede）：新证据可以推翻旧假设

2. **BeliefFSM（有限状态机）**：
   - 三状态：Drilling（继续深挖）→ Reporting（可出报告）→ Done（结束）
   - 三阈值决策：
     - GapDelta（默认 0.3）：最高分假设与第二名的差距
     - MinSupport（默认 2）：最少支撑证据数
     - MaxSteps（默认 3）：每层最大步数
   - 当 gap >= GapDelta 且 support >= MinSupport 时 report，否则 drill down

3. **多层级假设**：
   - L1 假设：资源耗尽 / 网络问题 / 配置错误（自动生成）
   - L2 假设：CPU 高负载 / 内存泄漏 / DNS 故障（专家细化）
   - L3 假设：具体根因（最终结论）

**Result：**

- 图结构支持"修正推理链"，新证据可以推翻旧假设
- Copy-on-Write 保证并发安全，专家可并行执行
- FSM 三阈值防止无限循环，保证收敛
- 与 Plan-Execute-Replan 互补：GoS 适合结构化收敛，Plan 适合灵活排查

### 面试 Q&A

**Q：为什么用图结构而不是线性 chain？**

> 线性 chain 是"一条路走到黑"，中间步骤出错后续全错。图结构允许多个假设并行发展，新证据可以同时支撑某些假设、反驳另一些假设。比如"服务响应慢"可能同时有"CPU 高"和"网络延迟"两个假设，查到 CPU 正常后，"CPU 高"假设被反驳，但"网络延迟"假设继续发展。

**Q：Copy-on-Write 怎么实现的？**

> 每次更新图时，先调用 `UpdateCopyOnWrite()` 复制当前图的快照，在快照上修改（添加节点/边、更新置信度），然后原子替换指针。读操作直接读当前图，不需要加锁。这样多个专家可以并行执行，互不干扰。

**Q：FSM 的三阈值怎么调？**

> GapDelta 控制收敛严格程度，越大越保守（需要假设之间差距大才出报告）。MinSupport 控制最少证据数，防止"孤证"。MaxSteps 控制每层最大步数，防止无限循环。实际调优：简单故障用低阈值快速收敛，复杂故障用高阈值避免误判。

**Q：节点撤回和替代有什么区别？**

> Retract 是"这个假设不对"，直接移除。Supersede 是"这个假设被更好的假设替代"，旧假设保留但标记为 superseded，新假设继承旧假设的部分证据。比如"CPU 高负载"假设被"内存泄漏导致 CPU 高负载"替代，后者更精确。

---

## 11. 防幻觉管线（Events 系统）

### STAR 讲法

**Situation：**

LLM 在 AIOps 场景中容易幻觉：编造不存在的指标值、自相矛盾的诊断、给出没有证据支撑的结论。仅靠 prompt 约束不够，需要工程化的防线。

**Task：**

设计多层防幻觉管线，从不同维度检测和阻止 LLM 幻觉输出。

**Action：**

1. **五层防线**：
   - SchemaGate：输出实质性检查（>= 10 字符）、自相矛盾检测（同时包含"正常"和"异常"）
   - OutputValidator：指标溯源校验，验证 LLM 输出的数值是否来自工具结果
   - ContractCollector：契约校验，检查是否有工具调用、是否引用了工具数据
   - NoToolCallDetection：检测运维问题但没有工具调用的情况
   - LLMValidation：LLM 交叉校验（可选）

2. **统一事件协议**：
   - AgentEvent：model_start/end、tool_call_start/end、text_delta、error
   - CallbackEmitter：将 Eino 框架回调转换为 AgentEvent
   - 多目标扇出：SSE 推送、结构化日志、Trace 记录

3. **ToolWrapper 拦截链**：
   - Before：参数校验（非空 + 合法 JSON）
   - After：结果截断 / 语义压缩
   - 安全设计：after 失败时返回安全错误消息，不暴露原始数据

4. **HealthCollector**：
   - 工具健康度收集：P50/P95/P99 延迟、成功率、常见错误
   - 为工具可靠性提供数据支撑

**Result：**

- 五层防线从不同维度检测幻觉
- 统一事件协议连接三条通道（SSE/日志/Trace）
- ToolWrapper 零侵入增强，不修改工具实现
- HealthCollector 为工具可靠性提供数据

### 面试 Q&A

**Q：为什么不能只靠 prompt 约束防幻觉？**

> Prompt 约束是"软约束"，LLM 可能忽略。工程化防线是"硬校验"，在输出层强制检查。比如 prompt 说"不要编造数据"，但 LLM 可能还是编造了 CPU 95%。OutputValidator 会验证这个 95% 是否在工具返回的结果中，不在就标记为幻觉。

**Q：指标溯源校验怎么做的？**

> OutputValidator 从 LLM 输出中提取数值（如"CPU 使用率 95%"、"延迟 500ms"），然后在工具返回的结果中查找这些值。如果 LLM 编造了工具没有返回的数据，标记为幻觉并降级输出。

**Q：ToolWrapper 的 after 失败会怎样？**

> 返回 `[工具结果处理失败]` 消息，不暴露原始数据。同时发射不含 result summary 的事件，避免泄露未脱敏数据。这样即使脱敏/校验/审计失败，用户看到的也是安全的错误提示。

**Q：HealthCollector 有什么用？**

> 收集每个工具的延迟（P50/P95/P99）、成功率、常见错误。可以发现：某个工具延迟突然变高（可能服务有问题）、某个工具错误率上升（可能配置有误）、某个工具经常超时（需要增加超时时间）。为工具可靠性提供数据支撑。

---

## 12. 多 Agent 运行时（Runtime）

### STAR 讲法

**Situation：**

OpsCaptain 有多个 Agent（PlanAgent、GOSAgent、ExpertAgent），需要一个调度中心管理它们的执行：任务创建、状态流转、超时控制、结果持久化、事件发布。

**Task：**

设计一个轻量级的多 Agent 运行时，支持可插拔持久化、Agent 超时、Contract 强制执行。

**Action：**

1. **Runtime.Dispatch**：
   - 创建任务记录（TaskEnvelope）
   - 查找注册的 Agent（AgentRegistry）
   - 设置超时（context.WithTimeout）
   - 执行 Agent（agent.Handle()）
   - 持久化结果（Ledger）
   - 发布事件（Bus）

2. **Ledger + Bus 模式**：
   - InMemoryLedger：任务/结果/事件的内存账本，LRU 淘汰
   - LedgerBus：基于账本的事件总线
   - 支持可插拔持久化：内存（开发）/ 文件（生产）

3. **Agent 协议**：
   - TaskEnvelope：goal/assignee/constraints/memoryRefs/artifactRefs
   - TaskResult：summary/confidence/evidence/nextActions/degradationReason
   - 支持父子任务关系（parentTaskID）

4. **Contract 强制执行**：
   - 每个 Agent 执行后，ContractCollector 检查结果是否符合契约
   - 违反契约时自动降级（status=degraded, confidence 减半）

**Result：**

- 轻量级运行时，类似 LangGraph 的 Go 实现
- 每个 Dispatch 都有完整的 trace，可观测性强
- 可插拔持久化，开发用内存，生产用文件
- Contract 强制执行，保证 Agent 行为边界

### 面试 Q&A

**Q：为什么用 Ledger+Bus 模式而不是直接函数调用？**

> Ledger 提供了任务的完整生命周期记录（创建→执行→完成），Bus 提供了事件的发布-订阅机制。这样每个 Agent 的执行都有完整的 trace，可以回溯"为什么这个 Agent 给出这个结论"。直接函数调用没有这些能力。

**Q：TaskEnvelope 和直接传 string 有什么区别？**

> TaskEnvelope 是结构化的任务描述，包含 goal（目标）、assignee（指定 Agent）、constraints（约束条件）、memoryRefs（记忆引用）、artifactRefs（工件引用）。这样 Agent 收到的是结构化的任务，而不是模糊的字符串。constraints 可以限制 Agent 的行为（如"只分析不执行"）。

**Q：degraded 状态有什么用？**

> 区分"完全失败"和"降级可用"。完全失败（failed）是 Agent 崩溃了，没有结果。降级可用（degraded）是 Agent 部分成功，有结果但不够完整。比如日志查询超时，Agent 仍然可以基于已有的告警数据给出初步诊断，但置信度降低。

---

## 13. AIOps 服务编排

### STAR 讲法

**Situation：**

AIOps 诊断不是简单的"调用 Agent"，而是一个完整的执行管线：安全检查 → 审批门控 → 降级检查 → 上下文注入 → Skill 聚焦 → Agent 执行。还需要支持事故排障会话（多轮对话）。

**Task：**

设计 AIOps 服务编排层，串联完整的执行管线，管理事故排障会话。

**Action：**

1. **RunAIOpsMultiAgent 六步管线**：
   - Prompt Guard：注入检测
   - Approval Gate：高风险操作审批
   - Degradation Check：全局降级开关检查
   - Memory Injection：记忆上下文注入
   - Skill Focus：Skill 匹配 + Focus hint 注入
   - Runtime Dispatch：分发到 PlanAgent 或 GOSAgent

2. **事故排障会话**：
   - IncidentSession：多轮对话状态机
   - Turn 状态：user_message → agent_thinking → agent_response
   - 事件溯源：每个 Turn 记录完整的事件流
   - 审批集成：高风险操作在会话中发起审批

3. **Token 审计**：
   - Redis 按日按 session 计数
   - 日限额强制（config 配置）
   - 超限时拒绝新请求

4. **异步任务队列**：
   - Chat 任务：RabbitMQ，含重试/DLQ/去重
   - 记忆提取：RabbitMQ，异步执行不阻塞对话

**Result：**

- 六步管线串联完整的安全和执行流程
- 事故排障会话支持多轮对话和事件溯源
- Token 审计实现成本控制
- RabbitMQ 异步队列保证高可用

### 面试 Q&A

**Q：RunAIOpsMultiAgent 的六步管线是怎么串联的？**

> 1）Prompt Guard 检查注入风险；2）Approval Gate 检查是否需要人工审批；3）Degradation Check 检查全局降级开关；4）Memory Injection 注入相关记忆上下文；5）Skill Focus 匹配 Skill 并注入 Focus hint；6）Runtime Dispatch 分发到对应的 Agent。任何一步失败都有降级处理，不会中断整个流程。

**Q：事故排障会话和普通对话有什么区别？**

> 事故排障会话是多轮的，每轮都有完整的事件流（user_message → agent_thinking → agent_response）。支持事件溯源，可以回溯"为什么 Agent 在第 3 轮改变了结论"。集成了审批机制，高风险操作在会话中发起审批，审批通过后继续执行。

**Q：Token 审计怎么实现成本控制？**

> Redis 按日按 session 计数每次 LLM 调用的 token 用量。配置日限额（如 100 万 tokens/天），超限时拒绝新请求并返回"今日配额已用完"。这样可以防止单个用户或单天的成本失控。

---

## 14. LLM 模型抽象与弹性工程

### STAR 讲法

**Situation：**

LLM 调用是系统中最不可靠的环节：API 可能超时、限流、返回错误。而且不同模型（Doubao、DeepSeek、GLM）的 API 兼容性不同。

**Task：**

设计 LLM 调用的基础设施层，统一模型接口，添加可观测性、并发控制、熔断保护。

**Action：**

1. **instrumentedChatModel 包装器**：
   - OTel span 追踪：每次调用自动创建 span
   - Token 审计：记录 prompt_tokens / completion_tokens
   - 并发槽位限制：`AcquireLLMSlot()` 防止 API 过载
   - 熔断器：连续失败 5 次触发熔断，30s 后半开
   - 重试：指数退避，最多 3 次

2. **模型兼容性**：
   - OpenAI 兼容接口：支持 Doubao、DeepSeek、GLM 等
   - DeepSeek 兼容降级：ToolChoiceForced 不支持时自动降级

3. **弹性工程工具库**：
   - 并发槽位控制：限制同时进行的 LLM 调用数
   - 熔断器：GetBreaker() 按模型名隔离
   - 超时重试：Execute() 包装，区分可重试/不可重试错误

**Result：**

- 统一的 LLM 调用接口，业务层无感知
- 完整的可观测性：每次调用都有 trace、token 计数、延迟统计
- 并发控制防止 API 过载
- 熔断器快速失败，避免雪崩

### 面试 Q&A

**Q：为什么在模型层做 instrumented 包装而不是在业务层？**

> 业务层有几十个调用点，每个点都加 tracing/token 审计/熔断太容易遗漏。在模型层统一包装，所有调用自动获得这些能力，零遗漏。而且模型层可以统一处理 DeepSeek 兼容性问题。

**Q：并发槽位限制怎么工作？**

> `AcquireLLMSlot()` 使用信号量限制同时进行的 LLM 调用数（默认 10）。超过限制时等待，超时后返回 ConcurrencyLimitError。这样可以防止同时发起太多 API 调用导致限流或超时。

**Q：熔断器的三状态是什么？**

> Closed（正常）→ Open（熔断，快速失败）→ HalfOpen（半开，试探恢复）。连续失败 5 次触发熔断，30s 后进入半开状态，允许一个请求通过。如果成功，回到 Closed；如果失败，继续 Open。

**Q：Stream 模式为什么不能用超时包装？**

> Stream 模式下，LLM 返回的是一个 reader，数据逐步到达。如果用 `resilience.Execute` 包装超时，超时后会杀死 reader，导致已接收的数据丢失。所以 Stream 模式直接调用，不走超时包装。

---

## 15. 部署架构

### STAR 讲法

**Situation：**

OpsCaptain 是一个完整的 AI 系统，包含后端、前端、向量数据库、消息队列、缓存、监控等多个组件。需要一个可复现的生产部署方案。

**Task：**

设计 Docker Compose 生产部署方案，包含所有依赖组件。

**Action：**

1. **部署拓扑**：
   - Backend（Go + GoFrame）
   - Frontend（React + Vite，Nginx 静态托管）
   - Caddy（反向代理，自动 HTTPS）
   - Milvus（向量数据库，含 etcd + MinIO）
   - Redis（缓存、限流、任务状态）
   - RabbitMQ（异步任务队列）
   - Prometheus（指标采集）
   - Jaeger（分布式追踪）

2. **配置管理**：
   - 环境变量注入（API Key、数据库地址等敏感信息）
   - config.yaml 挂载（业务配置）
   - 支持热加载（部分配置）

3. **远程部署**：
   - remote-deploy.sh 脚本：rsync 代码 → docker compose up
   - 支持增量更新（只重建变化的服务）

**Result：**

- 一键部署：`docker compose -f deploy/docker-compose.prod.yml up -d`
- 完整的可观测性：Prometheus + Jaeger
- 自动 HTTPS：Caddy 自动申请证书
- 可复现：所有依赖容器化

### 面试 Q&A

**Q：为什么选 Milvus 而不是 Pinecone/Weaviate？**

> Milvus 是开源的，可以自部署，不依赖外部服务。支持 HNSW/FLAT 等多种索引类型，性能好。Go SDK 官方支持。Pinecone 是 SaaS，有数据主权和成本考虑。Weaviate 的 Go SDK 不够成熟。

**Q：RabbitMQ 在系统中的用途？**

> 两个用途：1）Chat 异步任务队列——用户提交对话后，任务进入 RabbitMQ，Worker 消费执行；2）记忆提取队列——对话完成后，记忆提取任务进入队列，异步执行不阻塞主对话。两个队列独立，互不影响。

**Q：Caddy 相比 Nginx 有什么优势？**

> Caddy 自动申请和续期 Let's Encrypt HTTPS 证书，零配置。配置语法更简洁。内置 HTTP/2 和 QUIC 支持。对于这种中小规模部署，Caddy 比 Nginx 更省心。

---

## 附录：关键技术栈

| 组件 | 技术 | 用途 |
|------|------|------|
| 后端框架 | Go 1.24 + GoFrame v2 | HTTP 服务、配置管理、依赖注入 |
| AI 框架 | ByteDance Eino | Agent 编排、DAG 流水线、工具管理 |
| 向量数据库 | Milvus | 文档向量存储和检索 |
| Embedding | Doubao Embedding | 文本向量化 |
| LLM | Doubao LLM / GLM Fast | 对话、改写、重排、意图识别 |
| 缓存 | Redis | 任务状态、会话缓存、限流 |
| 消息队列 | RabbitMQ | 异步任务分发 |
| 监控 | Prometheus + Jaeger | 指标采集、分布式追踪 |
| 前端 | React 18 + TypeScript + Vite | 对话界面、实时推送 |
