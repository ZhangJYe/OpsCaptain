# OpsCaption 项目架构分析

> **分析日期：** 2026-04-25  
> **项目定位：** 面向 AIOps 场景的智能运维助手  
> **Go Module：** SuperBizAgent  
> **技术栈：** Go 1.24+ / GoFrame v2 / Eino / DeepSeek V3 / Milvus / RabbitMQ / Redis  
> **前端：** 原生 HTML/JS (SuperBizAgentFrontend)

---

## 目录

1. [系统全景](#一系统全景)
2. [Chat Pipeline — ReAct Agent 执行引擎](#二chat-pipeline--react-agent-执行引擎)
3. [RAG 检索系统](#三rag-检索系统)
4. [Context Engine — 上下文工程](#四context-engine--上下文工程)
5. [Memory 记忆系统](#五memory-记忆系统)
6. [MQ 异步任务架构](#六mq-异步任务架构)
7. [Skills 与 Tools 系统](#七skills-与-tools-系统)
8. [知识入库流水线](#八知识入库流水线)
9. [Agent Contracts](#九agent-contracts)
10. [Plan-Execute-Replan 规划模式](#十plan-execute-replan-规划模式)
11. [安全与韧性](#十一安全与韧性)
12. [可观测性](#十二可观测性)
13. [数据流全景](#十三数据流全景)
14. [关键设计决策](#十四关键设计决策)
15. [目录结构速查](#十五目录结构速查)

---

## 一、系统全景

### 1.1 系统定位

OpsCaption 是一个面向 AIOps 场景的智能运维助手，核心能力：

- 接收用户运维问题（如"某服务 CPU 告警了怎么办"）
- 通过 ReAct Agent 自主推理 + 调用工具检索信息
- 从知识库（RAG）、长期记忆中获取上下文
- 生成结构化的诊断回答

### 1.2 整体架构

```
┌──────────────────────────────────────────────────────────────┐
│                       用户 (浏览器)                            │
│                  HTTP / SSE / 轮询 task_id                    │
└──────────────────────────┬───────────────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                   Controller 层 (GoFrame)                     │
│  chat_v1_chat        → 普通 Chat (ReAct Agent)               │
│  chat_v1_chat_stream → SSE 流式 Chat                         │
│  chat_v1_chat_task   → 异步提交 + 结果查询                     │
│  chat_v1_ai_ops      → AIOps 专用入口                        │
│  chat_v1_file_upload → 知识库文件上传                         │
│  chat_v1_admin       → 管理接口 (memory/degradation/approval) │
└──────────────────────────┬───────────────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                     Service 层                                │
│  MemoryService   → 记忆注入 / 持久化 / 上下文装配              │
│  ChatTaskQueue   → MQ 异步任务 (RabbitMQ + Redis)             │
│  MemoryQueue     → 记忆异步抽取 (MQ + 本地 fallback)           │
│  TokenAudit      → LLM Token 用量审计                         │
│  Degradation     → 手动降级开关                               │
│  ApprovalGate    → 高风险操作审批                              │
└──────────────────────────┬───────────────────────────────────┘
                           ▼
┌──────────────────────────────────────────────────────────────┐
│                     Agent 执行层 (Eino)                       │
│                                                              │
│  Chat Pipeline (ReAct Agent)                                 │
│  ┌──────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │ Input    │───▶│ ChatTemplate │───▶│ ReAct Agent  │       │
│  │ ToChat   │    │ (Prompt组装) │    │ (推理+工具)   │       │
│  └──────────┘    └──────────────┘    └──────────────┘       │
│                                            │                 │
│                                    调用 Tools (最多25步)      │
│                                            │                 │
│                              ┌─────────────┼─────────────┐   │
│                              ▼             ▼             ▼   │
│                         query_docs   query_log   query_alerts│
│                                                              │
│  Plan-Execute-Replan (Eino ADK)                              │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐              │
│  │ Planner  │───▶│ Executor │───▶│RePlanner │ (5轮)        │
│  └──────────┘    └──────────┘    └──────────┘              │
└──────────────────────────────────────────────────────────────┘
                           ▲
┌──────────────────────────┴──────────────────────────────────┐
│                      支撑层                                   │
│                                                              │
│  ┌──────────┐ ┌───────────┐ ┌──────────┐ ┌──────────────┐  │
│  │ RAG 链路 │ │ Context   │ │ Memory   │ │ Knowledge    │  │
│  │          │ │ Engine    │ │ 系统      │ │ Index Pipe   │  │
│  │ Rewrite  │ │ Assembler │ │ ShortTerm│ │ Loader→      │  │
│  │ Retrieve │ │ Budget    │ │ LongTerm │ │ Transformer→ │  │
│  │ Rerank   │ │ Profile   │ │ Working  │ │ Embedder→    │  │
│  │ Hybrid   │ │ Trace     │ │          │ │ Indexer      │  │
│  └──────────┘ └───────────┘ └──────────┘ └──────────────┘  │
│                                                              │
│  ┌──────────┐ ┌───────────┐ ┌──────────────────────────┐   │
│  │ Skills   │ │ Contracts │ │ 安全 / 韧性 / 可观测性     │   │
│  │ Registry │ │ 职责边界   │ │ JWT/RateLimit/PromptGuard│   │
│  │ Match    │ │ Must/MustNot│ │ /Health/Metrics/Tracing │   │
│  └──────────┘ └───────────┘ └──────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

### 1.3 两条执行路径

| 路径 | 触发条件 | 框架 | 特点 |
|------|---------|------|------|
| **ReAct Agent** (Chat Pipeline) | 所有 Chat 请求 | Eino compose.Graph | 单 Agent + 工具调用，最多 25 步推理循环 |
| **Plan-Execute-Replan** | AIOps 场景 | Eino ADK (planexecute) | Planner→Executor→RePlanner，最多 5 轮迭代 |

两条路径共享同一套支撑层：RAG、Context Engine、Memory、Tools。

---

## 二、Chat Pipeline — ReAct Agent 执行引擎

### 2.1 图编排

**代码位置：** `internal/ai/agent/chat_pipeline/`

Chat Pipeline 使用 Eino 的 `compose.Graph` 构建一个有向无环图：

```
┌─────────┐     ┌──────────────┐     ┌──────────────┐
│  START  │────▶│ InputToChat  │────▶│ ChatTemplate │
└─────────┘     │ (Lambda)     │     │ (Prompt组装) │
                └──────────────┘     └──────┬───────┘
                                            │
                                            ▼
                ┌──────────────┐     ┌──────────────┐
                │    END      │◀────│ ReAct Agent  │
                └──────────────┘     │ (推理+工具)   │
                                     └──────────────┘
```

节点说明：
- **InputToChat**：Lambda 节点，将 `UserMessage` 转换为 ChatTemplate 所需的变量（content / history / documents / date）
- **ChatTemplate**：基于 FString 格式的 Prompt 模板，拼装 System Prompt + History + 运行时上下文 + 用户问题
- **ReActAgent**：核心推理节点，通过 Eino 的 ReAct 实现 Reasoning + Acting 循环

### 2.2 Prompt 分层设计

**代码位置：** `internal/ai/agent/chat_pipeline/prompt.go`

Chat Template 的消息顺序：

```
1. SystemMessage    ← buildSystemPrompt(ctx)
2. MessagesPlaceholder("history", false)  ← 对话历史（动态注入）
3. UserMessage      ← runtimeContextTemplate ({date} + {documents})
4. UserMessage      ← {content}（当前用户问题）
```

Prompt 分为三层：

| 层级 | 作用域 | 内容 | 缓存策略 |
|------|--------|------|---------|
| **静态规则** | `scope=global` | 身份设定、语言规则、证据规则、工具使用指南 | 可缓存（预留 Anthropic cache_control） |
| **动态配置** | `scope=session` | 日志 topic 地域/ID（从 config.yaml 读取） | 边界标记 `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` |
| **运行时上下文** | User Message | 当前日期 + RAG 文档 | 不进 System Prompt |

**关键设计决策：**

> RAG 文档不进 System Prompt，而是放在普通 User Message 中——文档可被模型参考，但不会获得 System Prompt 优先级，降低 prompt injection 风险。

### 2.3 ReAct Agent 配置

- **最大推理步数**：25 步
- **工具白名单**：每个请求动态配置可用工具
- **输出要求**：纯文本，不含 Markdown 语法

### 2.4 Lambda 函数

**代码位置：** `internal/ai/agent/chat_pipeline/lambda_func.go`

`InputToChat` Lambda 负责渲染模板变量：

```go
{
    content:   input.Query,       // 用户问题
    history:   input.History,     // 对话历史
    documents: input.Documents,   // RAG 检索结果
    date:      time.Now(),        // 当前日期
}
```

---

## 三、RAG 检索系统

**目录：** `internal/ai/rag/`

### 3.1 检索流程

```
用户 query
    │
    ▼
Query Rewrite（LLM 改写口语为搜索词，3s 超时保护）
    │
    ▼
RetrieverPool.GetOrCreate()（按 Milvus 地址 + top_k 缓存复用）
    │
    ▼
Milvus ANN 搜索（Doubao Embedding → 向量相似度）
    │
    ▼
RetrieveRefine（精炼：去重 + 相关性过滤）
    │
    ▼
Rerank（LLM 打分 0-10 → 重排，5s 超时保护）
    │
    ▼
返回 top-K 文档 + QueryTrace
```

### 3.2 四种查询模式

| 模式 | 流程 | 适用场景 |
|------|------|---------|
| `retrieve` | 直接检索 | 关键词已精准 |
| `rewrite` | 改写 + 检索 | 需要扩展查询词 |
| `full` | 改写 + 检索 + 重排 | **默认模式**，保证精度 |
| `hybrid` | 向量检索 + BM25 关键词融合 | 混合检索，互补召回 |

### 3.3 核心组件

#### Query Rewrite (`query_rewrite.go`)
- 用 DeepSeek V3 将口语化 query 改写成搜索友好的形式
- 示例："服务挂了怎么办" → "服务故障排查方法 pod failure recovery"
- **3 秒超时**，失败自动降级为原始 query

#### RetrieverPool (`retriever_pool.go`)
- 按 `Milvus地址 + top_k` 做缓存 key 复用连接
- 失败也做短 TTL 缓存，**防止雪崩**
- 通过 `SharedPool` 提供进程级单例

#### Rerank (`rerank.go`)
- LLM 对每个文档打 0-10 相关性分，按分重排
- **5 秒超时**，失败用原始排序

#### Hybrid Retrieval (`hybrid.go` + `bm25.go`)
- 向量检索 (Milvus) + 关键词检索 (BM25) **并行执行**
- 融合策略：RRF (Reciprocal Rank Fusion)
- 互补召回：向量擅长语义相似，BM25 擅长精确关键词

#### RetrieveRefine (`retrieve_refine.go`)
- 检索结果二次过滤：去重、最小分数阈值、最大返回数

### 3.4 RAG 评测系统

**目录：** `internal/ai/rag/eval/`

| 组件 | 职责 |
|------|------|
| **Baseline 生成器** | 从 groundtruth 生成 build/holdout split |
| **Runner** | 执行 eval cases，计算 Recall@K / Hit@K |
| **InMemoryRetriever** | 评测用内存向量库（不依赖 Milvus） |
| **Online Eval** | 线上 RAG 质量持续监控 |
| **Adapter** | Milvus retriever → eval 接口适配 |

**关键原则：**
- build split 和 eval holdout split **严格分离**
- 不能拿全量数据自证效果

### 3.5 配置要点

RAG 参数全走 config.yaml，不硬编码：
- `multi_agent.knowledge_evidence_limit` — top_k
- `multi_agent.candidate_top_k` — 候选池大小
- Rerank 超时 / Rewrite 超时

---

## 四、Context Engine — 上下文工程

**目录：** `internal/ai/contextengine/`

### 4.1 设计理念

LLM 有 token 上限，不能把所有信息都塞进去。Context Engine 的职责是：**按需、按预算、组装最合适的上下文**。

### 4.2 四类上下文来源

```
ContextPackage
├── HistoryMessages  — 对话历史 (短期记忆)
├── MemoryItems      — 长期记忆 (跨会话)
├── DocumentItems    — RAG 检索结果
└── ToolItems        — 工具调用结果
```

### 4.3 Budget 预算机制

每类来源有 token 预算上限：

```
ContextBudget
├── HistoryTokens:  2000
├── MemoryTokens:    500
├── DocumentTokens: 3000
├── ToolTokens:     2000
└── ReservedTokens:  500
```

超出预算自动裁剪，通过 `ContextTrace` 记录装配细节（选中数、丢弃数、丢弃原因）。

### 4.4 Profile 策略机制

不同场景使用不同的上下文策略：

| Profile | AllowHistory | AllowMemory | AllowDocs | AllowToolResults | Staged |
|---------|-------------|-------------|-----------|-----------------|--------|
| **chat** | ✅ | ✅ | ✅ | ❌ | ✅（记忆前置为消息） |
| **aiops** | ❌ | ✅ | ✅ | ✅ | ❌ |
| **report** | ❌ | ❌ | ❌ | ✅ | ❌ |

### 4.5 Assembler 装配流程

```
Assembler.Assemble(ctx, req, history)
│
├─ PolicyResolver.Resolve() → 选择 Profile
│
├─ selectHistory()         → 按 Budget 从最新消息逆向选择
│   └─ 强制保留摘要前缀 "[对话历史摘要]"
│
├─ LongTermMemory.RetrieveScoped() → 按 Scope 检索记忆
│   └─ selectMemories() → 过滤：过期/Scope/Confidence/Safety/Budget
│
├─ selectDocuments()       → RAG 检索 + Budget 裁剪
│
├─ selectToolItems()       → 工具结果 Budget 裁剪
│
└─ 如果 Staged → 记忆作为消息前置注入 history
```

### 4.6 Memory 注入策略

记忆注入基于多维度过滤：

| 过滤维度 | 机制 |
|---------|------|
| **过期** | `ExpiresAt` 检查 |
| **作用域** | Session / User / Project / Global |
| **置信度** | 低于 `MinMemoryConfidence` 丢弃 |
| **安全标签** | 非 `internal/trusted_internal/safe` 丢弃 |
| **Token 预算** | 超出 `MemoryTokens` 截断 |
| **数量窗口** | 超出 `MaxMemoryItems` 丢弃 |
| **新鲜度** | 按 `FreshnessScore` 排序（最近使用的优先） |

---

## 五、Memory 记忆系统

**目录：** `utility/mem/` + `internal/ai/service/memory_service.go`

### 5.1 三层记忆架构

```
Memory
├── 短期记忆 — 当前对话的 HistoryMessages
├── 长期记忆 — 跨会话重要信息 (LongTermMemory)
│   └── 作用域：Session / User / Project / Global
└── 工作记忆 — 当前任务的 ToolItems
```

### 5.2 记忆写入流程

```
对话结束
    │
    ▼
MemoryService.PersistOutcome(sessionID, query, summary)
├─ 写入 SimpleMemory (短期，key=sessionID)
│
├─ 尝试 enqueueMemoryExtraction → MQ
│   ├─ MQ 可用 → Memory Consumer 异步抽取 (最佳路径)
│   └─ MQ 不可用 → 本地 semaphore + goroutine fallback
│
└─ 抽取流程：
    ├─ ExtractMemoryCandidates (LLM 提取)
    │   └─ Prompt: "从对话中提取值得记住的关键信息"
    ├─ ValidateMemoryCandidate (基础过滤)
    │   ├─ 太短 → 丢弃
    │   ├─ 太长(异常) → 丢弃
    │   ├─ 纯代码块 → 丢弃
    │   └─ assistant boilerplate → 丢弃
    └─ 写入 LongTermMemory
```

### 5.3 记忆读取流程

```
请求到来
    │
    ▼
MemoryService.BuildContextPlan(mode, sessionID, query)
├─ ContextAssembler.Assemble()
│   ├─ PolicyResolver → Profile 选择
│   ├─ LongTermMemory.RetrieveScoped(query, limit, policy)
│   │   └─ 读锁 (RLock)，只在更新 AccessCnt 时短暂写锁
│   └─ selectMemories → 过滤 + 裁剪
└─ 返回 memory_context + memory_refs
```

### 5.4 关键设计

| 设计点 | 实现 |
|--------|------|
| **异步抽取** | `context.WithoutCancel + context.WithTimeout` 隔离 |
| **超时保护** | `memory.extract_timeout_ms` (默认 1500ms) |
| **并发控制** | semaphore，可配置 `memory.extract_max_concurrency` (默认 8) |
| **Agent 模式** | `rule` (规则) / `llm` (LLM)，fallback 到 rule |
| **全局上限** | `memory.long_term_max_entries` 防止无限增长 |
| **过期清理** | 后台协程清理过期记忆 |

---

## 六、MQ 异步任务架构

### 6.1 双 MQ 链路

系统有两条独立的 RabbitMQ 异步链路：

#### 链路一：Chat Task（聊天异步）

```
用户提交 → chat_submit API
    │
    ├─ Redis: 写入任务状态 "queued"
    ├─ MQ:    发布 chat_task_event 到 exchange
    └─ 返回:  { task_id }（用户可关页面）
    
Consumer:
    ├─ 从 Queue 消费消息
    ├─ Redis: 更新状态 "running"
    ├─ 执行 Chat 业务逻辑
    │   ├─ 成功 → Redis: "succeeded" + answer/detail/trace
    │   ├─ 失败且可重试 → 发布到 Retry Queue
    │   └─ 超过重试次数 → 发布到 DLQ，Redis: "failed"
    
用户回来:
    └─ chat_task API → Redis 读取结果
```

**拓扑结构：**
```
Exchange: opscaption.events (direct)
├── Queue: opscaption.chat.task        ← RoutingKey: chat.task.request
├── Queue: opscaption.chat.task.retry  ← RoutingKey: chat.task.retry
│   └── x-dead-letter-exchange → 回流主队列
└── Queue: opscaption.chat.task.dlq    ← RoutingKey: chat.task.dlq
```

#### 链路二：Memory Extraction（记忆异步抽取）

```
对话结束 → PersistOutcome
    ├─ 先写 SimpleMemory (同步)
    ├─ 尝试 enqueue → MQ
    │   ├─ MQ 可用 → Memory Consumer 异步抽取 → LongTermMemory
    │   └─ MQ 不可用 → 本地 semaphore + goroutine fallback
    └─ 保证：核心记忆不丢，有降级兜底
```

### 6.2 可靠性机制

| 机制 | 实现 |
|------|------|
| **At-least-once** | 消息可能重复，消费端幂等 |
| **Retry + DLQ** | 失败先入 retry queue，超阈值入 DLQ |
| **状态机** | queued → running → succeeded/failed，Redis 持久化 |
| **超时保护** | `chat_async.execute_timeout_ms` / `memory.extract_timeout_ms` |
| **启动自愈** | 启动时自动连接 MQ，失败按间隔重试 |
| **优雅关闭** | 收到 SIGTERM → 标记 unready → 等待 in-flight → 关闭连接 |

### 6.3 核心配置

Chat Task:
- `chat_async.enabled` / `chat_async.task_ttl_seconds` / `chat_async.execute_timeout_ms`
- `rabbitmq.chat_task_queue` / `rabbitmq.chat_task_prefetch` / `rabbitmq.chat_task_max_retries`

Memory Extraction:
- `rabbitmq.memory_extract_queue` / `rabbitmq.prefetch` / `rabbitmq.max_retries`
- `memory.extract_timeout_ms` / `memory.extract_max_concurrency`

### 6.4 设计理念

> MQ 解耦"请求生命周期"和"任务生命周期"：用户关网页后任务继续执行；回来后按 task_id 查询结果。

---

## 七、Skills 与 Tools 系统

### 7.1 Skills 系统

**目录：** `internal/ai/skills/`

Skill 是"可插拔的能力单元"，每个 Skill 实现三个方法：

```go
type Skill interface {
    Name() string              // 标识
    Match(task) bool           // 判断能不能处理
    Run(ctx, task) error       // 执行
}
```

核心组件：

| 组件 | 职责 |
|------|------|
| **Registry** | Register + Resolve + Match，管理一组 Skill |
| **FocusCollector** | 从 task 中提取 focus 信息 |
| **ProgressiveDisclosure** | 控制 Skill 输出详细度，渐进展示 |

**匹配机制：** 按注册顺序匹配，第一个 Match 成功的执行；都不匹配返回错误。

### 7.2 Tools 系统

**目录：** `internal/ai/tools/`

| 工具 | 功能 | 核心 API |
|------|------|---------|
| `query_metrics_alerts` | 查 Prometheus 活跃告警 | Prometheus API |
| `query_log` | 查 MCP 日志 | 日志检索 |
| `query_internal_docs` | 查知识库（走 RAG 链路） | RetrieverPool → Milvus |
| `mysql_crud` | MySQL 数据库查询 | SQL 执行 |
| `get_current_time` | 获取当前时间 | 系统时间 |
| `tiered_tools` | 工具分层管理 | 能力分级 |

**工具白名单：** ReAct Agent 请求时动态配置可用工具列表，不同类型的请求可用不同工具。

---

## 八、知识入库流水线

**目录：** `internal/ai/agent/knowledge_index_pipeline/`

### 8.1 入库流程

```
用户上传文件 (chat_v1_file_upload.go)
    │
    ▼
Knowledge Index Pipeline
├── Loader:      读取文件内容
│   └── 支持格式：Markdown / PDF / TXT / CSV / DOC / DOCX
│
├── Transformer: 文档切分
│   └── 按标题切分 (Markdown) / 按 case 切分 (JSONL)
│
├── Embedder:    Doubao Embedding 向量化
│   └── 生成 1536 维向量
│
└── Indexer:     写入 Milvus
    └── 每条记录：id + content + metadata + vector
```

### 8.2 设计要点

- **Chunking 策略**：不只按 Markdown 标题切分，也支持 JSONL case 级切分
- **原始 parquet 不入向量库**：必须先预处理成 serving docs
- **文件上传支持**：点击上传 + 拖拽上传，大小限制 50MB

---

## 九、Agent Contracts

**文件：** `internal/ai/agent/contracts/contracts.go`

### 9.1 Contract 结构

每个核心 Agent 有稳定的 Contract 定义：

```go
Agent Contract {
    Agent, Version       // 标识和版本
    CacheScope           // 缓存级别 (global / session)
    Role                 // 角色
    Responsibilities     // 职责
    Inputs, Outputs      // 输入输出
    Must, MustNot        // 必须做 / 禁止做
    EvidencePolicy       // 证据策略
}
```

### 9.2 Contract 用途

- **运行时 trace**：结果附带 `agent_contract_id` / `agent_contract_version`
- **回归定位**：当 Agent 产生越权行为时，追踪到违反的 contract 版本
- **缓存预留**：`CacheScope=global` 的 contract 预留了 Anthropic Prompt Caching 映射

### 9.3 与 Claude Code 的对应

| Claude Code | OpsCaption |
|-------------|-----------|
| 静态 system prompt 可缓存 | `CacheScopeGlobal` |
| dynamic boundary | `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` |
| 工具权限规则 | `MustNot` + `EvidencePolicy` |

> 当前走 OpenAI-compatible provider，不塞 Anthropic 私有字段。架构已为未来切换 Claude provider 做准备。

---

## 十、Plan-Execute-Replan 规划模式

**目录：** `internal/ai/agent/plan_execute_replan/`

### 10.1 架构

基于 Eino ADK 的 `planexecute` prebuilt 模式：

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│ Planner  │────▶│ Executor │────▶│RePlanner │──┐
│ (制定计划) │     │ (执行步骤) │     │ (评估调整) │  │
└──────────┘     └──────────┘     └──────────┘  │
      ▲                                          │
      └──────────────────────────────────────────┘
                  最多 5 轮迭代
```

### 10.2 配置

- **MaxIterations**: 5 轮
- **超时**: 3 分钟
- **使用场景**: AIOps 复杂故障排查

### 10.3 输出

返回最终回答 + 每步执行详情 (detail)，支持逐步追踪推理过程。

---

## 十一、安全与韧性

### 11.1 安全层次

| 层次 | 组件 | 位置 |
|------|------|------|
| **认证** | JWT Token 签发/验证/吊销 | `utility/auth/jwt.go` |
| **限流** | 本地令牌桶 + Redis 分布式 | `utility/auth/rate_limiter.go` |
| **输入安全** | Prompt Guard（危险输入过滤） | `utility/safety/prompt_guard.go` |
| **输出安全** | Output Filter（敏感输出过滤） | `utility/safety/output_filter.go` |
| **CORS** | 智能 ResolveAllowedOrigin（不过宽） | `utility/middleware/` |
| **Secrets** | 不暴露/不记录 keys | 启动时 ValidateStartupSecrets |

### 11.2 韧性机制

| 机制 | 位置 | 说明 |
|------|------|------|
| **降级开关** | `degradation.go` | 手动关闭 AI 能力，返回友好提示 |
| **审批门** | `approval_gate.go` | 高风险操作拦截审批 |
| **重试/熔断** | `utility/resilience/` | 工具调用韧性 |
| **LLM 缓存** | `utility/cache/llm_cache.go` | 相同查询复用 LLM 结果 |
| **Token 审计** | `token_audit.go` | 用量追踪和限制 |

### 11.3 降级设计

降级是系统的核心设计哲学：

- **RAG Rewrite 超时** → 降级为原始 query
- **RAG Rerank 超时** → 降级为原始排序
- **MQ 不可用** → 降级为本地 goroutine fallback
- **LLM Memory Agent 不可用** → 降级为 Rule Agent
- **AI 服务不可用** → 手动降级，返回友好提示

---

## 十二、可观测性

| 组件 | 位置 | 说明 |
|------|------|------|
| **Prometheus Metrics** | `utility/metrics/` | 请求量 / 延迟 / LLM Token / 错误率 |
| **OpenTelemetry Tracing** | `utility/tracing/` | 分布式追踪，全链路 trace_id |
| **Health Check** | `/healthz` `/readyz` | 存活/就绪检查，依赖状态报告 |
| **结构化日志** | `utility/logging/` | 统一日志格式 + 级别控制 |
| **pprof** | `debug.pprof_address` | 非生产环境默认开启 |

---

## 十三、数据流全景

### 13.1 一次完整的 Chat 请求

```
用户输入："checkoutservice CPU 告警了怎么办"
    │
    ▼
[Controller] JWT验证 → RateLimit → PromptGuard
    │
    ▼
[MemoryService.BuildChatPackage]
    ├─ ContextAssembler.Assemble("chat")
    │   ├─ selectHistory (短期记忆)
    │   ├─ LongTermMemory.RetrieveScoped (长期记忆)
    │   └─ selectDocuments → RAG.Query (知识检索)
    └─ 返回 ContextPackage
    │
    ▼
[Chat Pipeline — ReAct Agent]
    ├─ InputToChat → {content, history, documents, date}
    ├─ ChatTemplate → System Prompt + History + Runtime Ctx + Query
    └─ ReAct Agent (最多25步)
        │
        ├─ 步骤1: 理解问题 → 调用 query_internal_docs
        │   └─ RAG: rewrite → retrieve → rerank → 返回3篇文档
        ├─ 步骤2: 分析文档 → 调用 query_prometheus_alerts
        │   └─ 返回2条活跃告警
        ├─ 步骤3: 综合 → 生成诊断回答
        └─ 输出: "根据分析，checkoutservice CPU 告警可能由..."
    │
    ▼
[PersistOutcome] → 异步记忆抽取
    ├─ SimpleMemory (同步)
    └─ MQ → LongTermMemory (异步)
    │
    ▼
HTTP 响应 (或 SSE 流式)
```

### 13.2 异步任务数据流

```
POST /api/v1/chat_task/submit { query, session_id }
    │
    ├─ Redis: task_id → { status: "queued", query, ... }
    ├─ MQ:    publish chat_task_event
    └─ 响应:  { task_id }  ← 用户可关页面
    
[数秒后，MQ Consumer 执行]
    ├─ Redis: status → "running"
    ├─ Chat Pipeline → ReAct Agent → answer + detail
    ├─ Redis: status → "succeeded", answer, detail, trace_id
    
[用户重新打开页面]
GET /api/v1/chat_task?task_id=xxx
    └─ Redis → { status: "succeeded", answer, ... }
```

### 13.3 RAG 知识入库流

```
用户拖拽/点击上传文件
    │
    ▼
POST /api/v1/upload (multipart/form-data)
    ├─ 保存到 file_dir
    │
    ▼
Knowledge Index Pipeline
    ├─ Loader:      读取文件
    ├─ Transformer: 切分文档
    ├─ Embedder:    Doubao → 向量
    ├─ Indexer:     写入 Milvus
    └─ 完成
```

---

## 十四、关键设计决策

| 决策 | 理由 | 来源 |
|------|------|------|
| **单体内执行** 而非分布式 Agent | 优先验证业务模型 | AGENTS.md §3.2.2 |
| **文件存储** Ledger/ArtifactStore | 不增加额外部署依赖 | AGENTS.md §17.1 |
| **RAG docs 不进 System Prompt** | 降低 prompt injection 风险，提升缓存友好性 | Prompt 架构指南 |
| **ContextProfile 按模式分层** | chat/aiops/report 不同上下文需求 | contextengine |
| **MQ + 本地 fallback 双链路** | 保证核心能力不因单点故障丢失 | MQ 架构 |
| **记忆抽取 LLM → Rule fallback** | API 不可用时不丢记忆能力 | memory_service.go |
| **build/holdout split 严格分离** | 不能拿全量数据自证效果 | RAG Eval |
| **RetrieverPool 按地址+top_k 缓存** | 避免重复建连，失败短 TTL 防雪崩 | retriever_pool.go |
| **所有配置走 config.yaml** | 不硬编码，可动态调整 | 代码规范 |
| **超时保护全覆盖** | Rewrite/Rerank/Extract/Execute 都有独立超时 | 全局 |
| **Degraded 降级而非 Fatal** | 局部失败不阻塞整体流程 | 全局设计哲学 |

---

## 十五、目录结构速查

```
OpsCaptain/
├── main.go                          # 入口：服务初始化 + 中间件 + 优雅关闭
├── go.mod / go.sum                  # 依赖管理
├── AGENTS.md                        # Agent 协作规则（所有 AI Agent 的行为约束）
│
├── internal/
│   ├── ai/
│   │   ├── agent/
│   │   │   ├── chat_pipeline/       # ★ ReAct Agent (图编排 + Prompt)
│   │   │   ├── plan_execute_replan/ # ★ Plan-Execute-Replan (Eino ADK)
│   │   │   ├── contracts/           # Agent Contract 定义
│   │   │   ├── knowledge_index_pipeline/ # 知识入库流水线
│   │   │   ├── skillspecialists/    # Skills-Driven Agent (知识/日志/指标)
│   │   │   ├── supervisor/          # 编排器 (实验性)
│   │   │   ├── triage/              # 路由器 (实验性)
│   │   │   ├── reporter/            # 报告器 (实验性)
│   │   │   └── specialists/         # Legacy Specialists
│   │   ├── protocol/                # 统一数据结构 (TaskEnvelope/TaskResult)
│   │   ├── runtime/                 # 任务分发引擎 (Registry/Ledger/Bus/Artifacts)
│   │   ├── rag/                     # ★ RAG 链路 (Query/Rewrite/Rerank/Pool/Hybrid)
│   │   │   └── eval/                # RAG 评测 (Baseline/Runner/Online)
│   │   ├── contextengine/           # ★ 上下文工程 (Assembler/Budget/Profile)
│   │   ├── service/                 # ★ 服务层
│   │   │   ├── memory_service.go    #   记忆管理核心
│   │   │   ├── chat_task_queue.go   #   MQ 异步 Chat
│   │   │   ├── memory_queue.go      #   MQ 异步记忆
│   │   │   ├── chat_multi_agent.go  #   路由判断
│   │   │   ├── degradation.go       #   降级开关
│   │   │   ├── approval_gate.go     #   审批门
│   │   │   └── token_audit.go       #   Token 审计
│   │   ├── skills/                  # Skill 抽象 (Registry/Focus/Disclosure)
│   │   ├── tools/                   # 工具定义 (query_docs/log/alerts/mysql)
│   │   ├── models/                  # LLM 模型接入 (DeepSeek V3)
│   │   ├── embedder/                # Doubao Embedding 管理
│   │   ├── retriever/               # Milvus Retriever
│   │   ├── indexer/ / loader/       # 索引/加载基础设施
│   │   └── cmd/                     # CLI 入口 (knowledge/rag_eval/chat/aiops)
│   ├── controller/chat/             # HTTP Controller (GoFrame)
│   ├── consts/                      # 常量
│   └── logic/                       # 业务逻辑 (chat/sse)
│
├── utility/                         # ★ 基础设施
│   ├── auth/        # JWT + RateLimit
│   ├── mem/         # 长期记忆存储
│   ├── metrics/     # Prometheus 指标
│   ├── tracing/     # OpenTelemetry
│   ├── health/      # 健康检查
│   ├── safety/      # Prompt/Output 安全
│   ├── resilience/  # 重试/熔断
│   ├── cache/       # LLM 缓存
│   ├── logging/     # 结构化日志
│   ├── middleware/   # HTTP 中间件
│   └── common/      # 公共配置/Env
│
├── SuperBizAgentFrontend/           # 原生 JS 前端 (SSE + 拖拽上传)
├── deploy/                          # 生产部署 (docker-compose/Caddy/Prometheus)
├── scripts/aiops/                   # Python 数据预处理
├── docs/                            # 运维知识库文档
├── Learn/                           # ★ 学习笔记与设计文档
│   └── architecture/                #   架构分析文档
├── res/                             # 复盘与设计
├── todo/                            # 执行计划
├── skills/                          # Skill 定义文件
└── manifest/config/config.yaml      # 配置文件
```

---

## 十六、总结

OpsCaption 在以下方面体现了成熟的工程实践：

1. **务实的 Agent 设计** — ReAct 单 Agent + Plan-Execute-Replan，不引入不必要的分布式复杂度
2. **完整的 RAG 链路** — Rewrite → Retrieve → Rerank → Hybrid，四种模式可切换，带超时降级
3. **精细的上下文工程** — 四类来源 + Budget 控制 + Profile 策略 + 多维过滤
4. **三层记忆系统** — 短期/长期/工作记忆，MQ 异步抽取 + 本地 fallback 双链路
5. **可靠的 MQ 架构** — 请求/执行解耦，重试+死信，状态持久化，用户断连不丢任务
6. **全面的韧性设计** — 超时保护全覆盖，失败降级而非崩溃，多级 fallback
7. **可追溯的 Agent Contract** — 每个 Agent 职责边界明确，metadata 用于 trace 和回归
8. **Prompt 安全架构** — System Prompt 与运行时上下文分离，降低 injection 风险
