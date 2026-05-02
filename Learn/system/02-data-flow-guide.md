# OpsCaption 当前数据流指南

> 更新时间：2026-05-02
> 当前有效链路只有两条：**Chat/ReAct** 与 **AIOps/Plan-Execute-Replan**

---

## 1. Chat 请求数据流

### 1.1 `/chat`

```text
Client
  ↓
chat_v1_chat.go
  ↓
校验 SessionID / Prompt Guard / Kill Switch
  ↓
Session Lock
  ↓
LLM Response Cache（问候类输入绕过）
  ↓
MemoryService.BuildChatPackage
  ↓
ContextEngine 装配 history / memory / docs
  ↓
BuildChatAgentWithQuery
  ↓
Eino ReAct Agent
  ↓
按需调用 tools / RAG
  ↓
输出过滤 + PersistOutcome + Cache Store
  ↓
JSON Response
```

### 1.2 `/chat_stream`

```text
Client
  ↓
chat_v1_chat_stream.go
  ↓
SSE 初始化
  ↓
校验 / 降级 / Session Lock
  ↓
MemoryService.BuildChatPackage
  ↓
BuildChatAgentWithQuery
  ↓
runner.Stream(...)
  ↓
SSE events: connected / meta / thought / message / done / error
  ↓
PersistOutcome
```

### 1.3 Chat 路径特点

- 统一走 `chat_pipeline`
- Agent 模式是 `ReAct`
- 工具暴露由 `ProgressiveDisclosure` 控制
- 记忆写回统一走 `MemoryService.PersistOutcome`
- 轻社交输入不会再被普通回答缓存硬短路

---

## 2. AIOps 请求数据流

### 2.1 `/ai_ops`

```text
Client
  ↓
RunAIOpsMultiAgent
  ↓
Approval Gate
  ↓
Degradation Check
  ↓
MemoryService.BuildContextPlan
  ↓
getOrCreateAIOpsRuntime
  ↓
Runtime.Dispatch(rootTask)
  ↓
aiops_plan_execute_replan
  ↓
Planner → Executor → RePlanner
  ↓
ExecutionResponseFromResult
  ↓
PersistOutcome
```

### 2.2 AIOps 输入在 runtime 里的关键字段

`rootTask.Input` 里当前会带这些核心信息：

- `raw_query`
- `executable_query`
- `context_detail`
- `response_mode = "ai_ops"`
- `entrypoint = "ai_ops"`

它们用于：

- 保留用户原始问题
- 注入记忆补充后的可执行查询
- 保持 trace / detail 输出一致

---

## 3. SSE 事件流

当前 ChatStream 会向前端发送：

| event | 含义 |
|---|---|
| `connected` | SSE 连接已建立 |
| `meta` | 模式、trace、detail、degraded 等元信息 |
| `thought` | 上下文 detail / 过程提示 |
| `message` | 文本分块 |
| `done` | 流结束 |
| `error` | 流执行失败 |

说明：

- 当前并不会把每一次 tool call 作为独立结构化事件推给前端
- 如果以后要增强“过程可观测性”，应该在现有 SSE 协议之上扩展，而不是重新引入 `chat_multi_agent` 路由

---

## 4. 已废止的数据流

以下这条曾经存在，但现在已经不属于当前聊天实现：

```text
ShouldUseMultiAgentForChat(query)
  ↓
RunChatMultiAgent(...)
  ↓
supervisor → triage → specialists → reporter
```

这条 `chat_multi_agent` 路由已经从 controller 和 service 活代码里移除。

---

## 5. 调试顺序建议

如果线上出现问题，建议按这条顺序 trace：

### Chat 问题

1. Controller 是否通过校验 / 降级
2. Cache 是否命中
3. MemoryService 是否成功装配上下文
4. ReAct Agent 是否正常调用工具
5. 输出过滤 / PersistOutcome 是否正常

### AIOps 问题

1. Approval / Degradation 是否拦截
2. MemoryContext 是否注入
3. Runtime 是否成功创建 / 复用
4. Plan-Execute-Replan 是否完成执行
5. Trace / Detail / PersistOutcome 是否完整
