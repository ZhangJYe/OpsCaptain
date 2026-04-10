# OpsCaptain 模块设计分析

> 评审日期: 2026-04-08 (更新)
> 评审方法: SART (Situation-Action-Result-Test)

---

## 一、API 层 (`api/chat/v1/`)

### Situation
需要定义清晰的前后端交互协议，支持对话、文件上传、AIOps 分析、审批流程等多个业务接口。

### Action — 接口定义

| 接口 | 方法 | 路径 | 功能 |
|------|------|------|------|
| Chat | POST | `/api/chat` | 智能对话 (ReAct Agent) |
| ChatStream | POST | `/api/chat_stream` | 流式对话 (SSE, ReAct Agent) |
| FileUpload | POST | `/api/upload` | 文件上传 (multipart/form-data) |
| AIOps | POST | `/api/ai_ops` | AI 运维分析 (Plan-Execute-Replan) |
| AIOpsTrace | GET | `/api/ai_ops_trace` | 查询运维分析 Trace (当前返回日志提示) |
| TokenAudit | GET | `/api/token_audit` | 查询会话 Token 使用审计 |
| ApprovalRequests | GET | `/api/approval_requests` | 查看审批请求列表 |
| ApproveRequest | POST | `/api/approval_requests/approve` | 批准审批请求 |
| RejectRequest | POST | `/api/approval_requests/reject` | 拒绝审批请求 |

**请求/响应约定**:
- 使用 GoFrame `g.Meta` tag 声明路由和方法
- 参数验证通过 `v:"required|max-length:128"` tag
- 统一 JSON 响应包装: `{message, data}`
- SSE 端点使用 `text/event-stream` Content-Type

### Result
- 接口设计 RESTful，语义清晰
- 参数验证完善 (长度限制、格式校验)
- 响应结构包含降级状态、审批信息等运维专有字段

### Test

- [ ] 所有必填参数缺失时返回 400 + 验证错误信息
- [ ] 超长参数 (>8000 字符) 被拒绝
- [ ] 响应结构一致: `{message: "OK", data: {...}}`
- [ ] ChatRes 包含 mode 字段，固定为 `"chat"` / `"cache"` / `"degraded"`

---

## 二、Controller 层 (`internal/controller/chat/`)

### Situation
Controller 作为 HTTP 请求的入口，负责参数转换、安全检查、响应格式化。Chat 统一走 ReAct Agent，AIOps 走 Plan-Execute-Replan。

### Action — 职责划分

```
internal/controller/chat/
├── chat.go                # GoFrame 自动生成的空壳
├── chat_new.go            # ControllerV1 构造函数，注入 SSE Service
├── chat_v1_chat.go        # Chat() — ReAct Agent 处理
├── chat_v1_chat_stream.go # ChatStream() SSE 流式处理 (ReAct Agent)
├── chat_v1_ai_ops.go      # AIOps() + AIOpsTrace()
├── chat_v1_admin.go       # TokenAudit() + Approval 管理
├── chat_v1_file_upload.go # FileUpload()
├── prompt_guard.go        # rejectSuspiciousPrompt() 安全拦截
└── response_helpers.go    # filterAssistantPayload() 输出过滤
```

**关键设计决策**:

1. **会话锁 (Session Lock)**: 使用 `sync.Map` + `sync.Mutex` 保证同一会话串行处理
   ```
   sessionLocks.LoadOrStore(id, &sync.Mutex{})
   ```

2. **Chat 单一路径**: Chat 和 ChatStream 均直接调用 ReAct Agent，不做路由决策
   ```go
   var buildChatAgent = chat_pipeline.BuildChatAgent
   ```

3. **函数变量注入**: 核心依赖通过 `var` 包级变量注入，便于测试 Mock
   ```go
   var buildChatAgent         = chat_pipeline.BuildChatAgent
   var getDegradationDecision = aiservice.GetDegradationDecision
   var runAIOpsMultiAgent     = service.RunAIOpsMultiAgent  // ai_ops controller
   ```

### Result

| 特征 | 评价 |
|------|------|
| 关注点分离 | ✅ Controller 只做协调，不包含业务逻辑 |
| 安全检查 | ✅ Prompt Guard + Output Filter 双重防护 |
| 错误处理 | ✅ userFacingChatError / userFacingAIOpsError 优雅降级 |
| 可测试性 | ✅ 函数变量注入支持 Mock |
| 并发安全 | ✅ 会话锁保证串行 |
| 路径简洁 | ✅ Chat 无路由分支，直连 ReAct Agent |

### Test

- [ ] Controller 不直接访问数据库或外部 API
- [ ] 所有 public 方法都有对应的 `_test.go`
- [ ] 函数变量可被测试 Mock 替换
- [ ] Prompt Guard 对中英文注入模式均有效
- [ ] Chat 和 ChatStream 返回 `mode: "chat"`

---

## 三、Service 层 (`internal/ai/service/`)

### Situation
Service 层是核心业务编排层，协调 Plan-Execute-Replan、Memory、Approval 等子系统完成 AIOps 和 Chat 任务。

### Action — 模块职责

```
internal/ai/service/
├── ai_ops_service.go      # RunAIOpsMultiAgent + Plan-Execute-Replan 调用 + 审批管理
├── chat_multi_agent.go    # RunChatMultiAgent + ShouldUseMultiAgentForChat (保留，未连接)
├── memory_service.go      # MemoryService (Context 组装 + 记忆持久化)
├── degradation.go         # DegradationDecision + Kill Switch (Config/Redis)
├── approval_gate.go       # StaticApprovalGate (高危操作拦截)
├── approval_queue.go      # ApprovalQueue (审批队列管理)
├── token_audit.go         # Token 使用量审计 + 每日限额
```

**AIOps Service 核心流程** (ai_ops_service.go):
```
RunAIOpsMultiAgent(ctx, query)
  ├── ApprovalGate.Check()         → 高危拦截
  ├── GetDegradationDecision()     → 降级检查
  ├── MemoryService.BuildContextPlan() → 记忆上下文
  ├── enrichedQuery = query + 历史上下文
  ├── buildPlanAgent(ctx, enrichedQuery)
  │     → plan_execute_replan.BuildPlanAgent()
  │     → (content, planDetail, err)
  ├── PersistOutcome()             → 记忆持久化
  └── 返回 ExecutionResponse
```

**Chat Multi-Agent Service** (chat_multi_agent.go):
```
状态: 保留，未连接 Chat Controller
内容: ShouldUseMultiAgentForChat() — 关键词路由
      RunChatMultiAgent() — Runtime 分发
      getOrCreateChatRuntime() — 懒加载 Runtime
      registerChatAgents() — 注册 6 个 Agent
```

**ExecutionResponse** — 统一执行结果:
```
ExecutionResponse {
    Content           string
    Detail            []string
    TraceID           string
    Status            ResultStatus  (succeeded/failed/degraded)
    DegradationReason string
    ApprovalRequired  bool
    ApprovalRequestID string
    ApprovalStatus    string
    ExecutionPlan     []string
}
```

### Result

| 特征 | 评价 |
|------|------|
| 解耦设计 | ✅ AIOps Service 通过函数变量 `buildPlanAgent` 解耦具体实现 |
| 接口抽象 | ✅ aiOpsMemory 接口便于 Mock |
| 可配置性 | ✅ 超时、限额、降级均支持配置 |
| 成本控制 | ✅ Token Audit + Daily Limit 双保险 |
| 安全审批 | ✅ 高危操作强制人工审批 |
| 保留扩展 | ✅ Multi-Agent Chat Service 完整保留，随时可重新接入 |

### Test

- [ ] buildPlanAgent 被 RunAIOpsMultiAgent 正确调用
- [ ] Degradation Kill Switch 支持 Config 和 Redis 两种来源
- [ ] Token Audit 正确累计 prompt/completion/total tokens
- [ ] Daily Token Limit 超限后返回 DailyTokenLimitError
- [ ] Approval Queue 状态转换: pending → approved → executed
- [ ] 审批拒绝时返回拒绝原因

---

## 四、Plan-Execute-Replan Agent (`internal/ai/agent/plan_execute_replan/`)

### Situation
AIOps 需要 LLM 驱动的深度分析能力：先制定计划、逐步执行、评估结果并按需调整。

### Action — 组件结构

```
plan_execute_replan/
├── plan_execute_replan.go  # BuildPlanAgent() — 组装 Planner+Executor+Replanner
├── planner.go              # NewPlanner() — LLM 生成分步计划
├── executor.go             # NewExecutor() — LLM + 工具逐步执行
└── replan.go               # NewRePlanAgent() — LLM 评估并调整计划
```

**BuildPlanAgent 流程**:
```go
BuildPlanAgent(ctx, query) → (content string, detail []string, err error)
  ├── context.WithTimeout(ctx, 3 分钟)
  ├── NewPlanner(ctx)    → Eino Planner (GLM-4.5-AIR)
  ├── NewExecutor(ctx)   → Eino Executor (GLM-4.5-AIR + Tools)
  ├── NewRePlanAgent(ctx) → Eino Replanner (GLM-4.5-AIR)
  ├── planexecute.New(ctx, &Config{
  │     Planner, Executor, Replanner,
  │     MaxIterations: 5,
  │   })
  ├── adk.NewRunner(ctx, RunnerConfig{Agent})
  ├── runner.Query(ctx, query) → Iterator
  └── 遍历 Iterator → 收集 detail + lastMessage
```

**三个阶段**:

| 阶段 | LLM 角色 | 约束 |
|------|---------|------|
| Planner | 分析问题，生成分步计划 | 一次调用 |
| Executor | 逐步执行计划，自主选择工具 | 最多 10 轮工具调用 |
| Replanner | 评估执行结果，决定是否追加步骤 | 最多 5 轮迭代 |

**可用工具集**: query_prometheus_alerts, query_internal_docs, query_log (MCP), mysql_crud, get_current_time

### Result

| 特征 | 评价 |
|------|------|
| LLM 全程驱动 | ✅ 规划、执行、重规划三阶段均由 LLM 决策 |
| 迭代式执行 | ✅ Replanner 确保结果完整性 |
| 超时保护 | ✅ 全局 3 分钟超时 |
| 接口简洁 | ✅ 一个函数调用 `BuildPlanAgent(ctx, query)` 即可 |

### Test

- [ ] 正常查询 → 返回 content + detail
- [ ] LLM 不可达 → 返回错误
- [ ] 超过 3 分钟 → context deadline exceeded
- [ ] detail 包含每一步执行的摘要

---

## 五、Chat Pipeline (`internal/ai/agent/chat_pipeline/`)

### Situation
Chat 对话需要 LLM 自主决策工具调用的 ReAct Agent，支持最多 25 轮推理-行动循环。

### Action — 组件结构

```
chat_pipeline/
├── orchestration.go   # BuildChatAgent() — Eino Graph 编译
├── flow.go            # ReAct Agent Lambda (最多 25 轮)
├── model.go           # ChatModel 创建 (GLM-4.5-AIR)
├── prompt.go          # System Prompt Template
├── lambda_func.go     # InputToChat 类型转换
├── tools_node.go      # 工具节点集合
└── types.go           # UserMessage / AgentOutput
```

**Eino Compose Graph**:
```
START
  ▼
InputToChat (UserMessage → []*schema.Message)
  ▼
ChatTemplate (System Prompt 注入，含 Documents/History)
  ▼
ReAct Agent (GLM-4.5-AIR, 最多 25 轮)
  ├── LLM 推理: 需要调工具吗？
  ├── Tool Calls:
  │   ├── query_prometheus_alerts
  │   ├── query_internal_docs
  │   ├── query_log (MCP)
  │   ├── mysql_crud
  │   └── get_current_time
  └── LLM 看结果 → 继续 or 回复
  ▼
END → AgentOutput { Content, Detail }
```

### Result
- 统一入口 `BuildChatAgent(ctx)` 返回 `Runnable[*UserMessage, *AgentOutput]`
- 同时支持 `Invoke()` (同步) 和 `Stream()` (SSE 流式)
- LLM 自主决策工具选择，无需人工路由

### Test

- [ ] "hello" → 直接回复，不调工具
- [ ] "查一下 Prometheus 告警" → 调用 query_prometheus_alerts
- [ ] 工具调用失败 → LLM 接收错误信息，尝试其他方式
- [ ] 25 轮上限 → 强制结束

---

## 六、Multi-Agent Runtime (`internal/ai/runtime/`)

### Situation
完整的多智能体运行时框架，支持 Agent 注册、Task 分发、事件追踪、结果持久化。当前保留在代码库中，未连接 Chat 链路。

### Action — 核心抽象

```
runtime/
├── agent.go       # Agent 接口定义
├── runtime.go     # Runtime 调度核心 (Dispatch + Execute)
├── registry.go    # Agent 注册表
├── ledger.go      # Ledger 接口 + InMemoryLedger (Task/Event 追踪)
├── bus.go         # Bus 接口 + LedgerBus (事件发布)
├── artifacts.go   # ArtifactStore (结果工件存储)
├── context.go     # Context 工具函数 (withRuntime/withTask/FromContext)
├── file_store.go  # FileLedger + FileArtifactStore (持久化存储)
```

**Agent 接口**:
```go
type Agent interface {
    Name() string
    Capabilities() []string
    Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error)
}
```

**连接状态**:
- `chat_multi_agent.go` 保留了完整的 Runtime 创建和 Agent 注册逻辑
- Controller 层已切断调用 (不再引用 `RunChatMultiAgent`)
- AIOps 已改用 Plan-Execute-Replan，不再使用 Runtime
- Knowledge Index Agent 实现了 `runtime.Agent` 接口，可被 Runtime 调度

**Dispatch 执行流程**:
```
Runtime.Dispatch(task)
  ├── 生成 TaskID / TraceID / Timestamps
  ├── Ledger.CreateTask()
  ├── Registry.Get(task.Assignee) → Agent
  ├── UpdateTaskStatus → Running
  ├── Publish TaskEvent(task_started)
  ├── Start OTel Span (runtime.dispatch / agent.{name})
  ├── executeAgent(ctx, agent, task, timeout)
  │     ├── context.WithTimeout(ctx, timeout)
  │     ├── agent.Handle(runCtx, task)
  │     └── select { case result / case <-timeout }
  ├── normalizeTaskResult()
  ├── Ledger.AppendResult()
  ├── UpdateTaskStatus → Succeeded/Failed
  ├── Publish TaskEvent(task_completed/task_failed)
  └── Metrics.ObserveAgentDispatch()
```

### Result

| 特征 | 评价 |
|------|------|
| 接口驱动 | ✅ Agent/Ledger/Bus/ArtifactStore 均为接口 |
| 可观测性 | ✅ 每个 Dispatch 生成 OTel Span + TaskEvent |
| 超时控制 | ✅ 每个 Agent 独立超时 (默认 30s，可配置) |
| 容错 | ✅ 超时/取消/失败均生成结构化 TaskResult |
| 存储抽象 | ✅ InMemory (测试) + File (生产) 两种实现 |
| 当前状态 | ⚠️ 保留未连接，可随时重新接入 Chat 或其他场景 |

### Test

- [ ] Dispatch nil task → 返回错误
- [ ] Dispatch 未注册 Agent → 返回 "agent not registered" 错误
- [ ] Agent 执行超时 → 返回 timeout TaskResult + task_timeout Event
- [ ] Agent 执行成功 → Ledger 中有 Task + Result + Events
- [ ] EventsByTrace 按 CreatedAt 升序排列

---

## 七、Agent Protocol (`internal/ai/protocol/`)

### Situation
Runtime 框架下多个 Agent 之间需要统一的通信协议，定义 Task 信封、执行结果、事件等数据结构。

### Action — 类型体系

```
┌─────────────────────────────────────────────────────┐
│                  TaskEnvelope                        │
│  (Task 信封 — Agent 接收的输入)                       │
│                                                      │
│  ├── TaskID / ParentTaskID / SessionID / TraceID    │
│  ├── Goal (任务目标文本)                              │
│  ├── Assignee (目标 Agent 名称)                      │
│  ├── Creator (创建者 Agent 名称)                     │
│  ├── Intent / Priority                              │
│  ├── Status (pending/running/succeeded/failed)      │
│  ├── Input map[string]any (自由传参)                 │
│  ├── Constraints map[string]any (超时等约束)         │
│  ├── MemoryRefs / ArtifactRefs (上下文引用)          │
│  └── CreatedAt / UpdatedAt / DeadlineAt             │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│                  TaskResult                          │
│  (Agent 返回的执行结果)                               │
│                                                      │
│  ├── TaskID / Agent                                 │
│  ├── Status (succeeded/failed/degraded)             │
│  ├── Summary (摘要文本)                              │
│  ├── Confidence (置信度 0.0~1.0)                    │
│  ├── DegradationReason (降级原因)                    │
│  ├── Evidence []EvidenceItem (证据列表)              │
│  ├── ArtifactRefs (生成的工件引用)                   │
│  ├── NextActions []string (建议的后续操作)            │
│  ├── Metadata map[string]any (自由元数据)            │
│  ├── Error *TaskError (错误详情)                     │
│  └── StartedAt / FinishedAt                         │
└─────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────┐
│                  TaskEvent                           │
│  (Runtime 事件 — 追踪用)                              │
│                                                      │
│  ├── EventID / TaskID / TraceID                     │
│  ├── Type: task_started / task_info /               │
│  │         task_completed / task_failed /            │
│  │         task_timeout                             │
│  ├── Agent (产生事件的 Agent)                        │
│  ├── Message (人可读消息)                            │
│  ├── Payload map[string]any                         │
│  └── CreatedAt                                      │
└─────────────────────────────────────────────────────┘
```

**使用方**:
- Multi-Agent Runtime (Dispatch) — 保留未连接
- chat_multi_agent.go (RunChatMultiAgent) — 保留未连接
- ai_ops_service.go — 仅使用 ResultStatus 常量和 MemoryRef 类型
- knowledge_index_pipeline/agent.go — Agent.Handle 的输入输出类型

### Result
- 统一的 Task-Result-Event 三层协议
- 支持 Task 层级关系 (Parent-Child)
- TraceID 贯穿整个调用链

### Test

- [ ] NewRootTask 生成唯一 TaskID 和 TraceID
- [ ] NewChildTask 继承 Parent 的 SessionID 和 TraceID
- [ ] TaskResult 的 Status 枚举完整覆盖三种状态

---

## 八、Multi-Agent 体系 (保留未连接)

### Situation
系统内保留了完整的 Supervisor-Specialist 模式多智能体代码，包括编排器、意图分类、三个专家和报告聚合。当前未连接 Chat 链路。

### Action — Agent 家族

```
internal/ai/agent/
├── supervisor/           # 编排控制 Agent (保留)
│   └── supervisor.go     # Triage → Parallel Specialists → Reporter
│
├── triage/               # 意图分类 Agent (保留)
│   └── triage.go         # 规则匹配: Intent + Domains + Priority
│
├── skillspecialists/      # 基于 Skill 的专家 Agent (保留)
│   ├── metrics/           # Prometheus 告警查询
│   │   └── agent.go       # 4 Skills: release_guard / capacity / alert_triage / incident
│   ├── logs/              # 日志证据提取
│   │   └── agent.go       # 6 Skills: panic_trace / api_failure / payment / auth / evidence / raw
│   └── knowledge/         # 知识库检索
│       └── agent.go       # 5 Skills: rollback / release_sop / error_code / sop / incident
│
├── reporter/              # 报告聚合 Agent (保留)
│   └── reporter.go        # 结果聚合 + Markdown 报告 / Chat 响应
│
├── specialists/           # 旧版专家 Agent (保留兼容)
│   ├── metrics/
│   ├── logs/
│   └── knowledge/
```

**连接状态**:
- `chat_multi_agent.go` 中的 `registerChatAgents()` 注册了全部 6 个 Agent
- `ShouldUseMultiAgentForChat()` 关键词路由函数仍存在
- `RunChatMultiAgent()` 完整的 Runtime 分发逻辑仍存在
- **Controller 层不再调用任何 Multi-Agent 函数**

### Agent 交互矩阵 (保留逻辑)

| Agent | 输入 | 输出 | 调用关系 |
|-------|------|------|---------|
| **Supervisor** | RootTask (query + memory_context) | Final TaskResult | Triage → [Specialists] → Reporter |
| **Triage** | query | intent + domains[] + priority | 被 Supervisor 调用 |
| **Metrics** | query + intent + memory_context | TaskResult (alerts evidence) | 被 Supervisor 并行调用 |
| **Logs** | query + intent + memory_context | TaskResult (log evidence) | 被 Supervisor 并行调用 |
| **Knowledge** | query + intent + memory_context | TaskResult (doc evidence) | 被 Supervisor 并行调用 |
| **Reporter** | query + intent + results[] | TaskResult (aggregated report) | 被 Supervisor 调用 |

### Result
- 完整保留，可随时通过 Controller 重新接入
- 当前所有 Agent 内部为规则/模板驱动 (零 LLM 调用)
- 未来集成 LLM 后可作为 Chat 的增强模式

### Test

- [ ] registerChatAgents 注册 6 个 Agent 无错误
- [ ] Supervisor 编排逻辑完整 (Triage → Specialists → Reporter)
- [ ] 单个 Specialist 失败不影响其他 Specialist
- [ ] Reporter 正确聚合多个 Specialist 结果

---

## 九、Knowledge Index Agent (`internal/ai/agent/knowledge_index_pipeline/`)

### Situation
知识索引 Pipeline 需要支持被 Runtime 调度，同时保持作为独立 ETL 服务使用。

### Action — 双入口设计

```
knowledge_index_pipeline/
├── orchestration.go   # BuildKnowledgeIndexing() — Eino Graph 编排
├── loader.go          # 文档加载
├── transformer.go     # 文档分割/转换 (Markdown Splitter)
├── indexer.go         # 向量索引 (Milvus)
└── agent.go           # Agent 壳 — 实现 runtime.Agent 接口
```

**ETL Pipeline 入口** (传统方式):
```
rag.IndexingService.IndexSource(ctx, path)
  → BuildKnowledgeIndexing() → Graph.Invoke()
```

**Agent 入口** (Runtime 可调度):
```go
type Indexer interface {
    IndexForAgent(ctx context.Context, path string) (IndexResult, error)
}

type Agent struct { indexer Indexer }
func (a *Agent) Name() string           → "knowledge_indexer"
func (a *Agent) Capabilities() []string  → ["knowledge-indexing", "document-indexing", "vector-indexing"]
func (a *Agent) Handle(ctx, task) (*protocol.TaskResult, error)
  ├── 从 task.Input["path"] 或 task.Goal 提取路径
  ├── 空路径 → MISSING_PATH 错误
  ├── indexer.IndexForAgent(ctx, path)
  └── 返回 TaskResult + Metadata (chunk_count, chunk_ids, ...)
```

**依赖方向**:
```
rag/indexing_service.go → knowledge_index_pipeline/orchestration.go (BuildKnowledgeIndexing)
knowledge_index_pipeline/agent.go → Indexer 接口 (不依赖 rag 包)
```

### Result

| 特征 | 评价 |
|------|------|
| 双入口 | ✅ ETL 直接调用 + Agent 壳被 Runtime 调度 |
| 依赖解耦 | ✅ Agent 通过 Indexer 接口与 rag 解耦，无循环依赖 |
| 幂等性 | ✅ 先删旧数据再写新数据 |
| 可组合 | ✅ 可注册到 Runtime 与其他 Agent 协同 |

### Test

- [ ] Agent.Handle() 传入 path → succeeded + chunk_count > 0
- [ ] Agent.Handle() 不传 path → failed + MISSING_PATH
- [ ] Indexer 接口可被 Mock

---

## 十、Skill 框架 (`internal/ai/skills/`)

### Situation
Multi-Agent 体系中的 Specialist Agent 内部使用 Skill Registry 做精细化路由。框架与 Multi-Agent 一同保留。

### Action — 核心接口

```go
type Skill interface {
    Name() string
    Description() string
    Match(task *protocol.TaskEnvelope) bool
    Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error)
}

type Registry struct {
    domain string
    skills []Skill
}
```

**匹配策略**:
1. 按注册顺序遍历 Skills
2. 调用 `skill.Match(task)` — 关键词匹配或自定义 Matcher
3. 首个匹配即选中
4. 无匹配时 fallback 到第一个 Skill

**辅助函数**:
- `ContainsAny(goal, keywords...)` — 大小写无关的关键词包含检查
- `PrefixedCapabilities(base, skillNames)` — 生成 "skill:{name}" 能力列表
- `AttachMetadata(result, domain, skill)` — 在结果中标记使用的 Skill

### Result
- 简洁的匹配-执行模式，易于扩展新 Skill
- 支持自定义 Matcher 处理复杂匹配逻辑
- 当前随 Multi-Agent 体系保留

### Test

- [ ] Registry 不允许空 Skill 列表
- [ ] 匹配优先级: 按注册顺序，首个命中
- [ ] 无匹配时 fallback 到第一个 Skill

---

## 十一、Tools 层 (`internal/ai/tools/`)

### Situation
Chat (ReAct Agent) 和 AIOps (Plan-Execute-Replan Executor) 共享同一组工具。

### Action — 工具清单

| 工具 | 文件 | 功能 | 数据源 | 使用方 |
|------|------|------|-------|-------|
| query_prometheus_alerts | query_metrics_alerts.go | 查询 Prometheus 活跃告警 | Prometheus HTTP API | Chat + AIOps |
| query_internal_docs | query_internal_docs.go | 搜索内部文档知识库 | Milvus (RAG) | Chat + AIOps |
| query_log (MCP) | query_log.go | 查询日志 | MCP Server | Chat + AIOps |
| mysql_crud | mysql_crud.go | 执行 MySQL 查询 | MySQL (GORM) | Chat + AIOps |
| get_current_time | get_current_time.go | 获取当前时间 | 系统时钟 | Chat + AIOps |

**工具实现模式**:
```go
func NewXxxTool() tool.InvokableTool {
    t, err := utils.InferOptionableTool(
        "tool_name",
        "tool description for LLM",
        func(ctx context.Context, input *InputStruct, opts ...tool.Option) (string, error) {
            // 1. 创建超时上下文
            // 2. 调用外部服务
            // 3. JSON 序列化结果
            return jsonString, nil
        })
    return t
}
```

### Result
- 统一的 Eino InvokableTool 接口
- Chat 和 AIOps 共享工具集，无需维护两套
- 每个工具独立超时控制
- 错误安全: 外部服务不可达时返回错误信息而不是 panic

### Test

- [ ] Prometheus 工具在 address 未配置时返回错误
- [ ] 内部文档工具在 Milvus 不可达时返回错误
- [ ] MCP 工具发现失败时返回空工具列表
- [ ] MySQL 工具正确执行 CRUD 操作
- [ ] 所有工具在超时后正确返回

---

## 十二、Context Engine (`internal/ai/contextengine/`)

### Situation
AI 推理质量高度依赖上下文质量。需要智能组装来自多个来源的上下文，同时严格控制 Token 预算。

### Action — 组件结构

```
contextengine/
├── types.go           # ContextRequest / ContextProfile / ContextPackage / ContextItem
├── resolver.go        # PolicyResolver — 根据 Mode 选择 Profile
├── assembler.go       # Assembler — 四阶段组装 (History/Memory/Documents/Tools)
├── documents.go       # 文档检索与选择逻辑
├── tool_items.go      # ToolItemsFromResults() — 从 Agent 结果提取工具条目
```

**Profile 体系**:
```
Mode → Profile 映射:
├── "chat"              → 大 History Budget, 允许 Memory + Docs
├── "aiops"             → 无 History, 大 Memory + Docs Budget
├── "chat_multi_agent"  → (保留) 均衡分配
├── "reporter"          → 大 Tool Results Budget, 禁用 History
└── default             → 均衡分配
```

**四阶段组装**:
```
1. History: 从最新到最旧选择，Budget 内优先保留最近对话
2. Memory: 从 LongTermMemory 检索，按相关性 + 新鲜度排序
3. Documents: 从 RAG (Milvus) 检索，Budget 裁剪
4. Tool Results: 从 Specialist 结果提取，Budget 裁剪 (含 TrimToTokenBudget)
```

### Result

| 特征 | 评价 |
|------|------|
| 渐进式组装 | ✅ 四阶段独立，互不干扰 |
| 预算控制 | ✅ 每阶段独立 Token 预算 |
| 可追踪性 | ✅ ContextAssemblyTrace 记录完整决策过程 |
| Profile 扩展 | ✅ 新模式只需添加 Profile |
| 降级友好 | ✅ 单阶段失败不影响其他阶段 |

### Test

- [ ] 每阶段 Token 使用不超过 Budget
- [ ] DroppedItems 记录丢弃原因
- [ ] TraceDetails() 输出完整诊断信息
- [ ] DocumentTokens = 0 时跳过文档检索

---

## 十三、RAG 模块 (`internal/ai/rag/`)

### Situation
系统需要从大量内部运维文档中检索相关信息，支持向量化存储和语义检索，同时提供文档索引入口。

### Action — 组件结构

```
rag/
├── config.go           # RAG 配置 (Milvus/Embedding)
├── shared_pool.go      # SharedPool() — 全局 RetrieverPool 单例
├── retriever_pool.go   # RetrieverPool — 带缓存的 Retriever 管理
├── query.go            # Query() — 统一检索入口
├── indexing_service.go # IndexingService — 文档索引编排
├── eval/               # RAG 评估框架
│   ├── types.go        # 评估类型定义
│   ├── samples.go      # 评估样本
│   ├── runner.go       # 评估运行器
│   ├── adapter.go      # 适配器
│   └── inmemory.go     # 内存评估存储
```

**IndexingService 依赖关系**:
```
IndexingService {
    buildPipeline   → knowledge_index_pipeline.BuildKnowledgeIndexing
    newLoader       → loader.NewFileLoader
    newMilvusClient → client.NewMilvusClient
}

IndexSource(ctx, path) → IndexBuildSummary
  ├── buildPipeline(ctx) → Eino Graph
  ├── newLoader(ctx) → Loader
  ├── loader.Load(ctx, source) → docs
  ├── deleteExistingSource(ctx, sourceValue) → deleted
  └── graph.Invoke(ctx, source) → chunkIDs
```

**RetrieverPool 缓存策略**:
```
GetOrCreate(ctx)
  ├── CacheKey 匹配 + Retriever 存在 → 直接返回 (CacheHit)
  ├── CacheKey 匹配 + 上次失败 + TTL 内 → 返回缓存的错误 (InitFailureCached)
  └── 其他 → 创建新 Retriever (factory 调用)
```

### Result
- 惰性初始化 + 失败缓存避免反复重试
- 完整的诊断 Trace 支持性能分析
- 独立的评估框架支持 RAG 质量测试
- IndexingService 只向下依赖 knowledge_index_pipeline，不向上反引

### Test

- [ ] 首次查询触发 Retriever 创建
- [ ] 第二次查询命中缓存 (CacheHit = true)
- [ ] Retriever 创建失败后在 TTL 内不重试
- [ ] Reset() 清除缓存，下次重新创建
- [ ] IndexSource 先删后写，ChunkIDs 非空

---

## 十四、Utility 层

### 14.1 Memory (`utility/mem/`)

| 组件 | 功能 |
|------|------|
| SimpleMemory | 短期会话内存 (消息历史) |
| LongTermMemory | 长期记忆存储 (语义检索) |
| Extraction | 从对话中提取记忆条目 |
| TokenBudget | Token 估算和裁剪 |

### 14.2 Auth (`utility/auth/`)

| 组件 | 功能 |
|------|------|
| JWT | Token 签发与验证 |
| RateLimiter | 本地令牌桶限流 |
| RedisRateLimiter | 分布式 Redis 限流 |
| RBAC | 角色路径访问控制 |

### 14.3 Safety (`utility/safety/`)

| 组件 | 功能 |
|------|------|
| PromptGuard | 提示注入检测 (6 种模式: 英文/中文) |
| OutputFilter | 输出敏感信息过滤 |

**Prompt Guard 模式**:
```
1. ignore_previous_instructions  → "ignore (all)? previous instructions?"
2. you_are_now                   → "you are now"
3. system_prefix                 → "system:"
4. inst_block                    → "[inst]" / "<<sys>>"
5. chinese_ignore                → "忽略(之前|以上|前面)的?指令"
6. chinese_role_override         → "你现在是"
```

### 14.4 Observability

| 组件 | 功能 |
|------|------|
| `utility/metrics/` | Prometheus 指标 (HTTP 延迟、Agent 分发、Token 使用) |
| `utility/tracing/` | OpenTelemetry Span 创建与传播 |
| `utility/logging/` | 结构化日志 (GoFrame Logger) |

### 14.5 Resilience (`utility/resilience/`)

| 组件 | 功能 |
|------|------|
| Resilience | 断路器模式 |
| Semaphore | 并发限制信号量 |

### 14.6 Cache (`utility/cache/`)

| 组件 | 功能 |
|------|------|
| ChatResponseCache | LLM 响应缓存 (按 session+query 键) |

### Result — Utility 层总评

| 维度 | 评价 |
|------|------|
| 安全性 | ✅ 多层防护: JWT + RBAC + RateLimit + PromptGuard + OutputFilter |
| 可观测性 | ✅ Metrics + Tracing + Structured Logging 三位一体 |
| 韧性 | ✅ 断路器 + 信号量 + 优雅降级 |
| 可测试 | ✅ 每个 utility 包都有对应的 `_test.go` |

### Test

- [ ] JWT 过期 token 被拒绝
- [ ] Rate Limit 超限返回 429
- [ ] Prompt Guard 检测到注入模式返回 blocked
- [ ] Output Filter 过滤敏感信息
- [ ] Resilience 断路器在连续失败后打开

---

## 十五、模块间交互关系总览

```
                    ┌─────────────┐
                    │ Controller  │
                    │  (chat.*)   │
                    └──────┬──────┘
                           │
             ┌─────────────┼─────────────┐
             │             │             │
             ▼             ▼             ▼
      ┌──────────┐  ┌──────────┐  ┌──────────┐
      │ Service  │  │ Safety   │  │ Cache    │
      │ (ai/svc) │  │(utility) │  │(utility) │
      └────┬─────┘  └──────────┘  └──────────┘
           │
     ┌─────┼──────────┐
     │     │          │
     ▼     ▼          ▼
┌────────┐ ┌────────────────┐  ┌──────────────┐
│Memory  │ │  Plan-Execute  │  │  Degradation │
│Service │ │  -Replan       │  │  + Approval  │
└───┬────┘ │  (AIOps)       │  └──────────────┘
    │      └───────┬────────┘
    ▼              │
┌────────┐         ▼
│Context │   ┌──────────────────────────────────────┐
│Engine  │   │      LLM (GLM-4.5-AIR)               │
└───┬────┘   │  Planner → Executor → Replanner       │
    │        └──────────────┬───────────────────────┘
    ▼                       │
┌────────┐            ┌─────▼─────┐
│  RAG   │            │   Tools   │
│(ai/rag)│            │(ai/tools) │
└───┬────┘            └─────┬─────┘
    │                       │
    ▼                 ┌─────▼──────────────────────┐
┌────────┐            │     External Services       │
│Milvus  │            │ Prometheus / MCP / MySQL     │
│Embedder│            └────────────────────────────┘
└────────┘

Chat 路径:
  Controller → BuildChatAgent → ReAct Agent (LLM + Tools)

AIOps 路径:
  Controller → RunAIOpsMultiAgent → BuildPlanAgent
    → Planner (LLM) → Executor (LLM + Tools) → Replanner (LLM)

保留未连接:
  ┌──────────────────────────────────────┐
  │        Multi-Agent Runtime           │
  │  ┌────────┐ ┌────────┐ ┌──────────┐ │
  │  │Supervi.│→│Triage  │→│Specialist│ │
  │  └────────┘ └────────┘ └────┬─────┘ │
  │                             │       │
  │                       ┌─────▼─────┐ │
  │                       │  Skills   │ │
  │                       │ Registry  │ │
  │                       └───────────┘ │
  │  chat_multi_agent.go 保留完整逻辑   │
  └──────────────────────────────────────┘
```

**活跃依赖方向**: Controller → Service → Agent → Tools → External
**保留依赖方向**: chat_multi_agent.go → Runtime → Multi-Agent Agents → Skills → Tools
**横切关注点**: Safety / Observability / Resilience / Memory 横切所有层

---

## 十六、关键设计模式总结

| 模式 | 应用位置 | 描述 | 状态 |
|------|---------|------|------|
| **ReAct Agent** | chat_pipeline | LLM + Tool 调用循环 (Eino Graph，最多 25 轮) | 在用 (Chat) |
| **Plan-Execute-Replan** | plan_execute_replan | LLM 规划 → 执行 → 评估 → 迭代 (最多 5 轮) | 在用 (AIOps) |
| **Token Budget** | contextengine | 四阶段独立预算控制上下文大小 | 在用 |
| **Degradation** | service/degradation | Config/Redis Kill Switch + 超时降级 | 在用 |
| **Approval Gate** | service/approval_gate | 高危操作人工审批门控 | 在用 |
| **Retriever Pool** | rag/retriever_pool | 惰性初始化 + 失败缓存 | 在用 |
| **Function Injection** | controller/service | 包级变量函数便于测试 Mock | 在用 |
| **Middleware Chain** | utility/middleware | Tracing → Metrics → CORS → Auth → RateLimit → Response | 在用 |
| **Agent Interface** | knowledge_index_pipeline/agent | ETL Pipeline 的 Agent 壳，可被 Runtime 调度 | 在用 |
| **Supervisor Pattern** | supervisor agent | 顶层编排器协调子 Agent | 保留 |
| **Skill Registry** | skillspecialists/* | 按关键词匹配分发到细粒度 Skill | 保留 |
| **Event Sourcing** | runtime/ledger | 所有状态变更以 Event 形式记录 | 保留 |
