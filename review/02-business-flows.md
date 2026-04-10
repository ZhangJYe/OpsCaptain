# OpsCaptain 核心业务流程图

> 评审日期: 2026-04-08 (更新)
> 评审方法: SART (Situation-Action-Result-Test)

---

## 一、Chat 对话流程 (ReAct Agent)

### Situation
用户通过 `POST /api/chat` 或 `POST /api/chat_stream` 发起对话请求，统一走 ReAct Agent。

### Action

```
用户发送 Chat 请求
│
▼
┌────────────────────────────────────────────────────────┐
│ [1] 请求预处理                                          │
│   ├── ValidateSessionID(id)                            │
│   ├── enrichRequestContext(ctx, id, requestID)         │
│   └── Prompt Guard: rejectSuspiciousPrompt(ctx, msg)  │
│        ├── 6种注入模式检测 (中英文)                      │
│        └── 拒绝 → 返回错误                              │
└────────────────────┬───────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────────┐
│ [2] 降级检查                                            │
│   getDegradationDecision(ctx, "chat")                  │
│   ├── Config: degradation.kill_switch                  │
│   └── Redis: oncallai:degradation:kill_switch          │
│                                                         │
│   如果降级 → 返回 {mode: "degraded", degraded: true}    │
└────────────────────┬───────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────────┐
│ [3] 会话锁                                              │
│   acquireSessionLock(id) — 同一会话串行处理              │
└────────────────────┬───────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────────┐
│ [4] 缓存检查                                            │
│   cache.LoadChatResponse(ctx, id, msg)                 │
│   ├── 命中 → 直接返回 {mode: "cache", cached: true}     │
│   └── 未命中 → 继续                                     │
└────────────────────┬───────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────────┐
│ [5] 上下文组装                                           │
│   MemoryService.BuildChatPackage(ctx, id, msg, history)│
│                                                         │
│   Context Engine (Assembler) 四阶段:                     │
│   ├── Stage 1: History Selection (Token Budget 裁剪)    │
│   ├── Stage 2: Memory Retrieval (长期记忆检索)           │
│   ├── Stage 3: Document Retrieval (RAG/Milvus)          │
│   └── Stage 4: Tool Results (无，Chat 不需要)            │
│                                                         │
│   输出: ContextPackage {HistoryMessages, DocumentItems}  │
└────────────────────┬───────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────────┐
│ [6] ReAct Agent 执行                                    │
│                                                         │
│   Eino Compose Graph:                                   │
│   ┌────────────────────────────────────┐                │
│   │ START                              │                │
│   │  ▼                                 │                │
│   │ InputToChat (UserMessage → schema) │                │
│   │  ▼                                 │                │
│   │ ChatTemplate (System Prompt 注入)   │                │
│   │  ▼                                 │                │
│   │ ReAct Agent (GLM-4.5-AIR)         │                │
│   │  ├── LLM 推理: 需要调工具吗？       │                │
│   │  ├── Tool Call:                    │                │
│   │  │   - query_prometheus_alerts     │                │
│   │  │   - query_internal_docs         │                │
│   │  │   - query_log (MCP)             │                │
│   │  │   - mysql_crud                  │                │
│   │  │   - get_current_time            │                │
│   │  ├── LLM 看结果，决定继续还是回复   │                │
│   │  └── 最多 25 轮循环                 │                │
│   │  ▼                                 │                │
│   │ END → schema.Message               │                │
│   └────────────────────────────────────┘                │
└────────────────────┬───────────────────────────────────┘
                     │
                     ▼
┌────────────────────────────────────────────────────────┐
│ [7] 响应后处理                                          │
│   ├── filterAssistantPayload(ctx, answer, detail)      │
│   ├── PersistOutcome(ctx, id, msg, answer)             │
│   │     ├── SimpleMemory.AddUserAssistantPair()        │
│   │     └── go ExtractMemoriesWithReport() (异步)      │
│   ├── cache.StoreChatResponse()                        │
│   └── 返回 ChatRes {answer, detail, mode: "chat"}     │
└────────────────────────────────────────────────────────┘
```

### Result
- Chat 统一走 ReAct Agent，LLM 自主决策是否调工具以及调哪个
- 支持 JSON 和 SSE 流式两种响应模式
- 完整的缓存 → 上下文 → 执行 → 持久化链路

### Test

- [ ] 发送 "hello" → ReAct Agent 直接回复，不调工具
- [ ] 发送 "Prometheus 有什么告警" → ReAct Agent 调用 query_prometheus_alerts
- [ ] 连续相同请求 → 第二次命中缓存，返回 `mode: "cache"`, `cached: true`
- [ ] 发送 "ignore previous instructions" → 被 Prompt Guard 拒绝
- [ ] 同一 session 并发请求 → 会话锁保证串行执行
- [ ] Kill Switch 开启 → 返回 `mode: "degraded"`, `degraded: true`

---

## 二、AIOps 分析流程 (Plan-Execute-Replan)

### Situation
用户通过 `POST /api/ai_ops` 发起 AI 运维分析请求。系统使用 LLM 驱动的 Plan-Execute-Replan 范式进行深度分析。

### Action

```
POST /api/ai_ops { query: "..." }
│
▼
┌────────────────────────────────────────────────────────────────┐
│ [1] Controller 层                                              │
│   ├── enrichRequestContext()                                   │
│   ├── rejectSuspiciousPrompt() — Prompt Guard                 │
│   ├── getDegradationDecision("ai_ops") — 降级检查             │
│   └── query 为空时使用默认 AIOps prompt                        │
│       "Follow this order: 1. Query active Prometheus alerts.   │
│        2. Look up matching docs. 3. Produce a markdown report" │
└──────────────────────────┬─────────────────────────────────────┘
                           │
                           ▼
┌────────────────────────────────────────────────────────────────┐
│ [2] Approval Gate                                              │
│   StaticApprovalGate.Check(ctx, query)                        │
│                                                                │
│   高危关键词: delete / drop / update / insert / truncate /     │
│   alter / rollback / restart / 执行 / 删除 / 修改 / 回滚 /    │
│   重启 / 写入 / 变更                                          │
│                                                                │
│   ├── 非高危 → Approved = true → 继续执行                     │
│   └── 高危 → Queued = true                                    │
│       └── 返回 {approval_required: true, ...}                  │
└──────────────────────────┬─────────────────────────────────────┘
                           │ (Approved)
                           ▼
┌────────────────────────────────────────────────────────────────┐
│ [3] Memory Context 组装                                        │
│   MemoryService.BuildContextPlan("aiops", sessionID, query)   │
│   ├── 生成 sessionID: "aiops_{userID}"                        │
│   ├── Context Engine 组装记忆 + 文档                           │
│   └── 如果有记忆上下文 → 拼接到 query 后面                     │
└──────────────────────────┬─────────────────────────────────────┘
                           │
                           ▼
┌────────────────────────────────────────────────────────────────┐
│ [4] Plan-Execute-Replan 执行                                   │
│                                                                │
│   BuildPlanAgent(ctx, enrichedQuery) → (content, detail, err) │
│                                                                │
│   ┌──────────────────────────────────────────────────────┐     │
│   │ Planner (GLM-4.5-AIR)                                │     │
│   │   输入: 用户问题 + 记忆上下文                          │     │
│   │   输出: 分步执行计划                                   │     │
│   │   例: ["查询 Prometheus 告警",                         │     │
│   │        "根据告警名检索知识库 SOP",                     │     │
│   │        "查询相关日志",                                 │     │
│   │        "综合分析根因"]                                  │     │
│   └──────────────────────┬───────────────────────────────┘     │
│                          │                                      │
│                          ▼                                      │
│   ┌──────────────────────────────────────────────────────┐     │
│   │ Executor (GLM-4.5-AIR + 工具集)                      │     │
│   │   逐步执行计划，每步 LLM 自主选择工具:                 │     │
│   │   ├── query_prometheus_alerts → 拿到告警数据           │     │
│   │   ├── query_internal_docs("HighCPU") → 拿到 SOP       │     │
│   │   ├── query_log("service-A error") → 拿到日志         │     │
│   │   └── LLM 综合推理 → 根因分析                         │     │
│   │   最多 10 轮工具调用                                   │     │
│   └──────────────────────┬───────────────────────────────┘     │
│                          │                                      │
│                          ▼                                      │
│   ┌──────────────────────────────────────────────────────┐     │
│   │ Replanner (GLM-4.5-AIR)                              │     │
│   │   评估: 计划执行完了吗？数据够了吗？                    │     │
│   │   ├── 足够 → 输出最终结果                              │     │
│   │   └── 不够 → 调整计划，继续执行                        │     │
│   │   最多 5 轮迭代                                       │     │
│   └──────────────────────────────────────────────────────┘     │
└──────────────────────────┬─────────────────────────────────────┘
                           │
                           ▼
┌────────────────────────────────────────────────────────────────┐
│ [5] 响应构造                                                    │
│   ├── PersistOutcome(sessionID, query, content) — 记忆持久化  │
│   ├── filterAssistantPayload() — 输出过滤                      │
│   └── 返回 ExecutionResponse                                   │
│       {content, detail, trace_id, status}                      │
└────────────────────────────────────────────────────────────────┘
```

### Result
- LLM 全程参与决策：规划、执行、评估三个环节都由 LLM 驱动
- Executor 可自主选择调用哪些工具、调用几次
- Replanner 确保结果完整性——数据不够会自动补充查询

### Test

- [ ] 无 query 的 AIOps 请求 → 使用默认 prompt，生成完整分析报告
- [ ] 包含 "delete" 的 AIOps 请求 → 返回 `approval_required: true`
- [ ] 审批通过 (`POST /api/approval_requests/approve`) → 实际执行
- [ ] 审批拒绝 (`POST /api/approval_requests/reject`) → 记录拒绝原因
- [ ] Plan-Execute-Replan 超时 → 返回 failed 状态 + 错误原因
- [ ] 执行结果写入 Memory → 后续 Chat 可获取分析历史

---

## 三、审批流程

### Situation
高危运维操作（如 delete、rollback、restart）需要人工审批后才能执行。

### Action

```
┌──────────────┐      ┌──────────────────────────────────────┐
│   用户发起    │      │          Approval Gate                │
│ AIOps 请求   │─────▶│  Check(ctx, query)                   │
│ (含高危词)    │      │                                      │
└──────────────┘      │  关键词匹配 → requiresApproval=true  │
                      │                                      │
                      │  ApprovalQueue.Enqueue()             │
                      │  → 返回 {approval_required: true}    │
                      └──────────────────────────────────────┘
                                        │
            ┌───────────────────────────┼───────────────────────────┐
            │                           │                           │
            ▼                           ▼                           ▼
┌─────────────────────┐  ┌──────────────────────┐  ┌──────────────────────┐
│ GET /approval_       │  │ POST /approval_       │  │ POST /approval_      │
│   requests           │  │   requests/approve    │  │   requests/reject    │
│                      │  │                       │  │                      │
│ 查看待审批列表       │  │ 批准 → 执行           │  │ 拒绝 → 记录原因      │
└─────────────────────┘  │   RunAIOpsMultiAgent() │  └──────────────────────┘
                         │   → Plan-Execute-Replan│
                         │   → 返回分析结果        │
                         └───────────────────────┘
```

### Result
- 高危操作自动拦截 → 入队等待审批
- 批准后自动执行 Plan-Execute-Replan 分析

### Test

- [ ] 非高危请求 → 直接通过 Approval Gate
- [ ] 高危请求 → `approval_required: true`
- [ ] 批准请求 → 执行 Plan-Execute-Replan
- [ ] 拒绝请求 → 记录原因，状态变为 rejected

---

## 四、知识索引流程

### Situation
用户通过 `POST /api/upload` 上传文档，或通过 CLI 批量索引 `./docs` 目录。

### Action

```
┌────────────────┐                     ┌──────────────────┐
│ POST /api/upload│                     │ CLI: knowledge_cmd│
│ (multipart)    │                     │ (批量索引)         │
└───────┬────────┘                     └────────┬─────────┘
        │                                       │
        ▼                                       ▼
┌──────────────────────────────────────────────────────┐
│ IndexingService.IndexSource(ctx, path)               │
│                                                       │
│ Step 1: deleteExistingSource() — 清理旧数据           │
│   └── Milvus: query + delete by metadata["_source"]  │
│                                                       │
│ Step 2: Knowledge Index Pipeline (Eino Graph)        │
│   FileLoader → MarkdownSplitter → MilvusIndexer      │
│                                                       │
│ 输出: IndexBuildSummary {                             │
│   SourcePath, ResolvedSource,                         │
│   DeletedExisting, ChunkIDs                           │
│ }                                                     │
└──────────────────────────────────────────────────────┘

Agent 化接口 (knowledge_index_pipeline.Agent):
  实现 runtime.Agent 接口
  ├── Name() → "knowledge_indexer"
  ├── Capabilities() → ["knowledge-indexing", ...]
  └── Handle(ctx, TaskEnvelope) → TaskResult
      └── 通过 Indexer 接口委托给 IndexingService
```

### Result
- 支持 HTTP 上传和 CLI 批量索引两种入口
- 先删旧数据再写新数据，保证幂等性
- Agent 壳支持被 Runtime 调度

### Test

- [ ] 上传 Markdown 文件 → 返回 chunk 数量
- [ ] 重复上传同一文件 → 先删后写，ChunkIDs 更新
- [ ] Agent.Handle() 传入 path → 返回 succeeded TaskResult
- [ ] Agent.Handle() 不传 path → 返回 MISSING_PATH 错误

---

## 五、记忆管理流程

### Situation
系统需要在会话间持久化关键信息，并在后续交互中检索相关记忆。Chat 和 AIOps 共享同一套 Memory 机制。

### Action

```
┌────────────────────────────────────────────────────────┐
│                  Memory Lifecycle                       │
│                                                         │
│  [写入路径] PersistOutcome()                             │
│    ├── SimpleMemory.AddUserAssistantPair(query, answer)│
│    │   (短期会话内存)                                    │
│    └── go ExtractMemoriesWithReport() (异步提取)        │
│        └── 存入 LongTermMemory (长期记忆)               │
│                                                         │
│  [读取路径] BuildChatPackage() / BuildContextPlan()      │
│    ├── SimpleMemory.GetContextMessages() (短期)         │
│    └── LongTermMemory.Retrieve() (长期，按相关性排序)    │
│                                                         │
│  [共享] Chat 和 AIOps 使用同一个 sessionID 的 Memory    │
│    → Chat 的对话结果写入 Memory                          │
│    → AIOps 的分析结果也写入 Memory                       │
│    → 两者的后续请求都能检索到之前的记忆                   │
└────────────────────────────────────────────────────────┘
```

### Result
- 双层记忆: 短期 (SimpleMemory) + 长期 (LongTermMemory)
- 异步提取避免阻塞请求响应
- Chat 和 AIOps 共享记忆池

### Test

- [ ] Chat 对话后 → SimpleMemory 中 TurnCount 增加
- [ ] AIOps 分析后 → 结果写入 Memory
- [ ] 后续 Chat → 能检索到 AIOps 的分析历史
- [ ] 记忆提取超时 → 不影响主请求
