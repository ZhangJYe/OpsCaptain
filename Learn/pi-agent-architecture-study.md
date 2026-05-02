# Pi Agent 架构研究与 OpsCaptain 对标分析

> 当前统一口径（2026-05）
> - 当前实现：Chat = `ContextEngine / MemoryService -> Eino ReAct Agent -> Tools / RAG`；AIOps = `Approval / Degradation / Memory -> Runtime -> Plan-Execute-Replan`
> - 本文里如果提到 `supervisor / triage / reporter / skillspecialists / chat_multi_agent`，应理解为历史实验或演进背景。
> - Pi Agent 参考重点已经从"是否自研 loop"收敛为"事件驱动层已落地，P2.5 稳定化已完成，P3 自研 Agent Loop 暂缓"。
>
> 日期：2026-05-02
> 状态：P0-P2.5 已落地，P3 暂缓

---

## 一、Pi Agent (pi-mono) 架构概述

### 1.1 项目背景

Pi 是一个极简终端编程 harness，作者 Mario Zechner (badlogic)。核心理念：**核心极简，扩展为王**。

- 项目地址：github.com/badlogic/pi-mono
- npm 包：@mariozechner/pi-coding-agent
- 关系：OpenClaw（github.com/openclaw/openclaw，367k stars）使用 Pi 作为编程能力的底层组件

### 1.2 分层架构

```
packages/
  ai/           → @mariozechner/pi-ai          (LLM 适配层)
  agent/        → @mariozechner/pi-agent-core  (Agent Runtime)
  tui/          → @mariozechner/pi-tui         (终端 UI)
  web-ui/       → @mariozechner/pi-web-ui      (Web UI)
  coding-agent/ → @mariozechner/pi-coding-agent (完整编程助手)
```

三层职责分明：

- **pi-ai**：屏蔽不同 LLM API，统一为 `streamSimple(model, context, options)` 调用
- **pi-agent-core**：agent loop + 工具调度 + 事件发射，不关心具体 LLM 和具体工具
- **coding-agent**：组装具体工具（read/write/edit/bash）、会话持久化、UI 交互

### 1.3 核心机制

#### 统一 LLM 适配

- `types.ts` 定义 `Api` 类型联合：`"anthropic-messages" | "openai-completions" | "google-generative-ai" | ...`
- `Model<TApi>` 接口统一所有模型元数据（id、name、provider、contextWindow、cost）
- `streamSimple()` 是统一入口，根据 `model.api` 路由到对应 provider 实现
- 每个 provider 各自实现流式解析，对外暴露相同的 `AssistantMessageEventStream`
- `register-builtins.ts` 用 lazy loading 注册所有 provider

#### Agent Loop（agent-loop.ts）

核心是 `runLoop()` 函数，双层 while 循环：

```
while (true) {                                    // 外层：follow-up 消息循环
  while (hasMoreToolCalls || pendingMessages) {   // 内层：tool calls + steering
    1. 注入 steering 消息
    2. 调用 LLM 获取响应
    3. 执行工具调用（串行或并行）
    4. 检查是否有新的 steering 消息
  }
  5. 检查是否有 follow-up 消息
}
```

#### 流式事件接口

`AgentEvent` 联合类型覆盖完整生命周期：

```typescript
type AgentEvent =
  | { type: "agent_start" }
  | { type: "agent_end"; messages: AgentMessage[] }
  | { type: "turn_start" }
  | { type: "turn_end"; message: AgentMessage; toolResults: ToolResultMessage[] }
  | { type: "message_start"; message: AgentMessage }
  | { type: "message_update"; message: AgentMessage; assistantMessageEvent: AssistantMessageEvent }
  | { type: "message_end"; message: AgentMessage }
  | { type: "tool_execution_start"; toolCallId: string; toolName: string; args: any }
  | { type: "tool_execution_update"; toolCallId: string; toolName: string; args: any; partialResult: any }
  | { type: "tool_execution_end"; toolCallId: string; toolName: string; result: any; isError: boolean }
```

#### 工具 Schema 校验

- `AgentTool<TParameters>` 用 TypeBox schema 定义参数类型
- `validateToolArguments()` 在 `prepareToolCall()` 中校验
- 支持串行和并行两种执行模式
- `beforeToolCall` / `afterToolCall` 钩子支持拦截和覆写

#### Steering 干预

- `config.getSteeringMessages()`：每轮工具执行后调用，注入到下一轮 LLM 调用前
- `config.getFollowUpMessages()`：agent 停止后调用，有新消息则继续
- 用户可以在 agent 运行中发送消息，loop 自动拾取

#### 会话边界

- `AgentMessage` = LLM Message + 自定义消息（通过 declaration merging 扩展）
- `convertToLlm(messages)` 在 LLM 调用边界将 AgentMessage 转换为 LLM 兼容的 Message
- `transformContext(messages)` 在转换前做上下文裁剪（pruning、注入外部上下文）

### 1.4 设计哲学

- **核心极简**：只提供 agent 运行所需的基础机制，不内置子代理、计划模式、MCP
- **扩展为王**：Extensions、Skills、Prompt Templates、Themes 都是外部扩展
- **SDK 化**：核心引擎可嵌入到任何宿主（CLI、Web、聊天 app、OpenClaw）
- **机制而非功能**：核心不是堆工具，而是提供 agent-loop、事件流、schema 校验等机制

---

## 二、OpsCaptain 现有架构分析

### 2.1 前提：聊天 Multi-Agent 路径已删除，历史 Orchestrator 仅保留为研究背景

**之前的 supervisor → triage → specialists → reporter 管道设计非常失败，当前项目中并未使用。**

当前实际在用的是两条路径：

- Chat：**eino ReAct Agent 路径**（`chat_pipeline`）
- AIOps：**Plan-Execute-Replan 路径**（`plan_execute_replan` + runtime 包装）

历史路径相关目录仍保留在代码库中（`internal/ai/agent/supervisor/`、`internal/ai/agent/triage/`、`internal/ai/agent/reporter/`、`internal/ai/agent/skillspecialists/`），但 `chat_multi_agent` 入口和聊天条件路由已经删除。

### 2.2 当前实际架构

```
ChatStream 控制器
  │
  ├─ buildChatAgent(ctx, query)
  │    └─ eino compose.Graph:
  │         START → InputToChat → ChatTemplate → ReactAgent → END
  │
  ├─ runner.Stream(ctx, userMessage)
  │    └─ eino react.Agent (黑盒，MaxStep: 25)
  │         └─ 内部自动调用工具（LLM 决定调用哪些、什么顺序）
  │
  └─ for { chunk := sr.Recv(); client.SendToClient("message", chunk) }
       └─ SSE 逐块推送给前端
```

#### 关键代码路径

**入口**：`internal/controller/chat/chat_v1_chat_stream.go` → `ChatStream()`

**Agent 构建**：`internal/ai/agent/chat_pipeline/orchestration.go` → `BuildChatAgentWithQuery()`

**React Agent 配置**：`internal/ai/agent/chat_pipeline/flow.go` → `newReactAgentLambdaWithQuery()`

```go
config := &react.AgentConfig{
    MaxStep:            25,
    ToolReturnDirectly: map[string]struct{}{},
}
config.ToolCallingModel = chatModelIns11
config.ToolsConfig.Tools = disclosed.Tools  // ProgressiveDisclosure 选出的工具
ins, _ := react.NewAgent(ctx, config)
```

**模型**：`internal/ai/agent/chat_pipeline/model.go` → `models.OpenAIForGLMFast()` (DeepSeek V3)

**工具选择**：ProgressiveDisclosure（domain 匹配 → AlwaysOn + SkillGate 分层暴露）

**流式输出**：`runner.Stream()` 返回 `schema.StreamReader[*schema.Message]`，控制器循环 `sr.Recv()` 逐块推送到 SSE

#### 当前流式事件

SSE 推送的事件类型：

| event type | 含义 |
|---|---|
| `connected` | SSE 连接建立 |
| `meta` | 元信息（mode、trace_id、detail、degraded） |
| `thought` | 思考过程（context detail） |
| `message` | 文本内容（逐块） |
| `done` | 流结束 |
| `error` | 错误 |

**问题：只推送最终文本，不推送工具调用过程。**

---

## 三、核心问题：eino react.Agent 是黑盒

### 3.1 黑盒带来的限制

当前架构的**所有核心逻辑都委托给了 eino 的 react.Agent**，OpsCaptain 对 agent loop 没有控制权：

| 能力 | Pi Agent | OpsCaptain (现状) |
|---|---|---|
| Agent Loop 控制 | 自己实现的双层 while 循环，完全可控 | eino react.Agent 黑盒，MaxStep: 25 |
| 工具调用过程可见 | 每个 tool_execution_start/end 都有事件 | 看不到，只有最终文本输出 |
| 中间推理可见 | thinking_start/delta/end 事件 | 只有 chunk.Content（最终文本） |
| Steering 干预 | getSteeringMessages() 钩子 | 无，react.Agent 运行中无法注入 |
| 工具执行拦截 | beforeToolCall / afterToolCall 钩子 | 无 |
| 工具 Schema 校验 | TypeBox 校验 + prepareArguments | eino 内部处理，外部不可见 |
| 多工具并行/串行控制 | toolExecution 配置 | eino 内部决定 |
| 流式事件类型 | 10 种事件覆盖完整生命周期 | 只有 message（文本块） |

### 3.2 前端能看到什么

```
当前：[thinking...] [一段完整文本] [done]
                ↑ 只有这个

理想：[agent_start]
      [turn_start]
      [message_start] → [thinking_delta...] → [thinking_end]
      [message_start] → [text_delta...] → [text_end]
      [tool_execution_start: query_metrics]
      [tool_execution_end: query_metrics → result]
      [tool_execution_start: query_logs]
      [tool_execution_end: query_logs → result]
      [message_start] → [text_delta...] → [text_end]
      [turn_end]
      [agent_end]
```

---

## 四、改进方案

### 4.1 设计结论

**可以推进，但不建议第一步就自研 Agent Loop。**

更合理的路线是：先把 Pi Agent 最有价值的部分抽出来，也就是**事件驱动和过程可观测**；等事件层跑稳后，再判断是否真的需要拿回完整 loop 控制权。

```
当前：
  ChatStream -> Eino ReAct Agent + AgentEvent + ToolWrapper -> SSE

已落地：
  P0: AgentEvent 协议 + Eino Callback Adapter
  P1: SSE agent_event + 前端事件消费
  P2: ToolWrapper before/after + TraceEmitter + HealthCollector

P2.5 当前目标：
  用测试、只读观测接口、真实 replay case 证明事件层稳定可靠

P3 暂缓目标：
  ChatStream -> OpsCaption AgentLoop -> eino model/tool interfaces -> AgentEvent -> SSE
```

这个分法的关键是把两个目标拆开：

- **可观测性**：先用 Eino callback 和现有 SSE 就能做。
- **可干预性 / 工具拦截 / 自定义终止**：callback 不一定够，需要后续 spike，再决定是否自研 loop。

### 4.2 P0：AgentEvent 协议 + Eino Callback Adapter

P0 不重写 loop，只做事件抽象和回调适配。

```go
// internal/ai/events/types.go
type AgentEventType string

const (
    EventAgentStart    AgentEventType = "agent_start"
    EventAgentEnd      AgentEventType = "agent_end"
    EventModelStart    AgentEventType = "model_start"
    EventModelEnd      AgentEventType = "model_end"
    EventToolCallStart AgentEventType = "tool_call_start"
    EventToolCallEnd   AgentEventType = "tool_call_end"
    EventTextDelta     AgentEventType = "text_delta"
    EventError         AgentEventType = "error"
)

type AgentEvent struct {
    Type      AgentEventType    `json:"type"`
    TraceID   string            `json:"trace_id,omitempty"`
    Name      string            `json:"name,omitempty"`
    Payload   map[string]any    `json:"payload,omitempty"`
    Timestamp time.Time         `json:"timestamp"`
}
```

```go
// internal/ai/events/eino_callback.go
type CallbackEmitter struct {
    Emit func(ctx context.Context, event AgentEvent)
}

func (c *CallbackEmitter) Handler() callbacks.Handler {
    // 适配 Eino model/tool callback。
    // 第一阶段只要求拿到 tool name、耗时、错误、结果摘要；
    // 不要求拿到完整 CoT，也不改变工具执行顺序。
}
```

接入点可以先放在现有流式链路：

```go
sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(
    events.NewModelCallbackEmitter(emitter, traceID).Handler(),
))
```

P0 的成功标准：

- 前端能看到“正在调用哪个工具”
- 后端日志能定位“哪个工具失败 / 超时”
- SSE 事件有统一 schema
- 不改变当前 ReAct Agent 的执行结果
- `go test ./...` 和流式回归通过

### 4.3 P1：事件流标准化 + 前端消费

P1 解决“事件能不能稳定消费”的问题。

SSE 事件建议分两层：

| SSE event | 用途 | 说明 |
|---|---|---|
| `meta` | 请求元信息 | trace_id、mode、degraded |
| `thought` | 当前已有过程提示 | 保留兼容 |
| `agent_event` | 新增结构化事件 | model/tool lifecycle |
| `message` | 文本增量 | 继续用于最终回答 |
| `done` | 流结束 | 客户端明确收尾 |
| `error` | 错误 | 传输或执行错误 |

前端不要直接依赖工具原始返回，只展示摘要和状态：

- `tool_call_start`：显示“正在查询 Prometheus / 日志 / 知识库”
- `tool_call_end`：显示耗时、成功/失败、摘要
- `error`：显示降级原因和 trace_id

### 4.4 P2：控制权可行性 Spike

只有当 P0/P1 证明 callback 不够时，再进入 P2。

重点验证三件事：

1. **Steering**：运行中用户追加消息，是否需要在当前 loop 内被即时消费。
2. **BeforeToolCall**：工具执行前是否必须做动态审批、参数改写或阻断。
3. **AfterToolCall**：工具执行后是否必须做结果过滤、证据归一化或覆写。

如果这三件事只是“未来可能需要”，就不要急着自研 loop；继续用 Eino ReAct + callback 更稳。

### 4.5 P2.5：稳定化阶段

P2.5 不是新架构，而是把已经做出来的事件驱动层变成可靠工程资产。

P0-P2 解决的是“能力有没有”：

```text
有 AgentEvent
有 SSE agent_event
有 ToolWrapper before/after
有 TraceEmitter
有 HealthCollector
前端能看到工具和模型事件
```

P2.5 解决的是“这套能力能不能稳定解释问题”：

```text
事件顺序是否稳定
工具健康数据是否看得到
真实 AIOps 问题里事件链是否能解释 Agent 查了什么、哪里失败、为什么降级
文档口径是否从未来计划更新为当前事实
```

#### 4.5.1 补 SSE 事件序列回归测试

SSE 是前端实时接收后端过程事件的通道。事件驱动的用户体验依赖稳定顺序：

```text
tool_call_start  // 开始调用工具
tool_call_end    // 工具调用结束
message          // 模型输出最终回答
done             // 本轮流式响应结束
```

如果顺序不稳定，前端会出现“答案已经完成，但又显示正在查日志”的错觉，用户会觉得 Agent 不可信。

这个测试不关注模型回答质量，而是验证流式协议本身：

- 后端是否按稳定顺序发送事件。
- 工具失败时是否仍然有 `tool_call_end`。
- `done` 是否只在最后出现。
- 前端是否可以根据事件顺序稳定收尾。

可以理解为给 Agent 的“过程直播”做回归测试。

#### 4.5.2 暴露 HealthCollector 只读观测能力

`HealthCollector` 已经能统计工具调用健康度，但如果数据只停留在内存结构里，工程价值有限。

下一步可以提供只读 admin 接口，例如：

```text
GET /api/admin/tool_health
```

返回示例：

```json
[
  {
    "tool_name": "query_metrics",
    "total_calls": 20,
    "success_rate": 0.95,
    "p95_duration_ms": 850,
    "common_errors": []
  }
]
```

这件事的价值是：OpsCaption 不只是能调用工具，还能观测工具链本身。如果日志工具经常超时、Prometheus 查询变慢，后端可以通过成功率和 P95 直接定位，而不是靠用户反馈和服务端日志猜。

如果暂时不做接口，也可以先做定时日志聚合，但只读接口更适合调试、演示和面试。

#### 4.5.3 做 3-5 个真实 AIOps Replay Case

Replay case 是固定输入、固定观测口径的回归样例。它不只看最终答案，还看中间事件链是否符合预期。

建议先准备 3-5 个典型问题：

```text
case 1: paymentservice 延迟升高
case 2: checkout timeout 日志增多
case 3: Redis 连接失败
case 4: 知识库 SOP 检索
case 5: 工具失败后的 degraded 回答
```

每个 case 观察事件链：

```text
用户输入 paymentservice 延迟升高
  ↓
tool_call_start: query_metrics
  ↓
tool_call_end: query_metrics success=true duration=300ms
  ↓
tool_call_start: query_logs
  ↓
tool_call_end: query_logs success=false error=timeout
  ↓
message: 基于已有 metrics 和知识库证据给出 degraded 结论
  ↓
done
```

验证重点：

- Agent 有没有调用该调用的工具。
- 工具失败时事件里有没有清楚展示失败原因。
- 工具失败后是否走 degraded，而不是假装完整成功。
- 最终回答是否能解释基于哪些证据。

面试表达可以简化为：

> 我做 replay case 时不是只看最终答案，而是验证中间事件链。这样能防止 Agent 看起来答了，实际没查证据。

#### 4.5.4 更新文档口径

这份文档原来是“未来计划”口吻：

```text
P0 做事件协议
P1 前端消费
P2 spike
P3 自研 loop
```

现在应更新成当前真实状态：

```text
已落地：AgentEvent / Eino Callback Adapter / SSE agent_event
已落地：ToolWrapper before/after / TraceEmitter / HealthCollector
已落地：前端事件消费
暂缓：P3 自研 Agent Loop
下一步：P2.5 稳定化
```

“暂缓自研 loop”不是退缩，而是架构判断：OpsCaption 当前最缺的是可观测性和稳定性，不是立刻替换 Eino ReAct。只有当现有方案不能满足 steering、强控制、动态中断时，才进入 P3。

### 4.6 P3：可选自研 Agent Loop

自研 loop 只能作为 P3，并且必须放在 feature flag 后面灰度。

```go
type AgentLoop struct {
    model  model.ToolCallingChatModel
    tools  []tool.BaseTool
    hooks  LoopHooks
    config LoopConfig
}

type LoopHooks struct {
    BeforeToolCall func(ctx context.Context, call ToolCallContext) (*BeforeResult, error)
    AfterToolCall  func(ctx context.Context, call ToolCallContext) (*AfterResult, error)
    GetSteering    func(ctx context.Context) []schema.Message
    Emit           func(ctx context.Context, event AgentEvent)
}
```

P3 的迁移要求：

- 先 shadow run，不直接替换线上 Chat
- 对比 ReAct 输出、工具调用次数、错误率和耗时
- 保留 fallback：自研 loop 失败时可回退 Eino ReAct
- 不影响 AIOps Plan-Execute-Replan 路径

---

## 五、Trade-off 分析

### 5.1 先做事件层，而不是先自研 loop

**一句话结论：OpsCaption 当前最缺的是可观测事件层，不是马上替换 Eino ReAct。**

| 目标 | Callback 事件层能否解决 | 是否需要自研 loop |
|---|---|---|
| 看见工具调用开始/结束 | 可以 | 暂不需要 |
| 记录工具耗时/错误 | 可以 | 暂不需要 |
| 前端展示过程反馈 | 可以 | 暂不需要 |
| 运行中插入用户纠偏 | 不一定 | 需要 spike |
| 工具执行前动态拦截 | 不一定 | 需要 spike |
| 自定义终止策略 | 不一定 | 需要 spike |
| 完全替换 Eino loop | 不能 | 需要 P3 |

因此，推进顺序应该是：

```text
可观测事件层 -> 事件消费稳定 -> 控制权缺口验证 -> 自研 loop 灰度
```

### 5.2 为什么还值得参考 Pi Agent

参考 Pi 的重点不是照搬它的完整 runtime，而是学习三个机制：

1. **事件驱动**：agent 的每一步都能转成结构化事件。
2. **核心机制下沉**：loop 只管模型、工具、事件，不把业务逻辑写死。
3. **UI 与执行解耦**：前端消费事件，而不是猜后端状态。

这些思想可以先落到 P0/P1，不必等自研 loop。

### 5.3 成本重新估计

| 阶段 | 成本 | 风险 | 说明 |
|---|---|---|---|
| P0 事件协议 + callback adapter | 2-4 天 | 低 | 不改执行语义，先拿可观测性 |
| P1 SSE / 前端事件消费 | 3-5 天 | 中 | 要处理兼容、断流、旧客户端 |
| P2 steering / interception spike | 1-2 周 | 中 | 只验证必要性，不承诺替换 |
| P2.5 稳定化 | 3-7 天 | 低 | 补事件序列测试、工具健康观测、真实 replay case、更新文档口径 |
| P3 自研 loop shadow path | 3-5 周 | 高 | 需要重接工具错误、记忆、缓存、降级、测试 |

原先“自研 loop 核心不超过 300 行”的说法过于乐观。真正成本不在 loop 本体，而在把现有生产能力重新挂回去：

- ContextEngine
- ProgressiveDisclosure
- SSE
- MemoryService.PersistOutcome
- output filter
- degradation / approval
- session lock
- tool error degrade
- trace / token audit

### 5.4 推进边界

本方案第一阶段只作用于 Chat ReAct 链路：

- 不恢复 `chat_multi_agent`
- 不改 AIOps Plan-Execute-Replan 执行方式
- 不把历史 `supervisor / triage / reporter` 重新接回主链路
- 不把工具原始结果直接暴露给前端

这样边界清楚，风险可控。

---

## 六、OpsCaptain 现有优势（应保留）

1. **ContextEngine**：预算控制 + 装配 trace，比 Pi 更精细。Pi 没有显式的 context budget 管理。
2. **ProgressiveDisclosure**：分层工具暴露（AlwaysOn / SkillGate / OnDemand），Pi 没有这个机制。
3. **Skill-driven 工具组织**：domain-scoped + keyword matching，结构化程度比 Pi 的 skill 更高。
4. **SSE 基础设施**：已有的 SSE Service 可以直接复用，只需对接新的事件流。
5. **错误降级模式**：ResultStatusDegraded 全链路使用，Pi 没有这个。
6. **Memory 持久化**：MemoryService 封装了记忆的构建和持久化。
7. **GoFrame 生态**：配置管理、中间件、日志等基础设施已就绪。

---

## 七、执行建议

### 当前状态（已完成）

- 已定义 `AgentEvent` 协议，覆盖 model/tool start/end、error、text delta。
- 已实现 Eino callback adapter，把 callback 转成 `AgentEvent`。
- 已在 `ChatStream` 中新增 `agent_event` SSE 事件，并保留现有 `message/thought/done`。
- 已接入 `SSEEmitter`、`TraceEmitter`、`MultiEmitter` 和 `HealthCollector`。
- 已用 `ToolWrapper` 支持 before/after hook，并修复 after hook 失败时的安全语义。
- 已让前端消费 `agent_event`，展示模型/工具调用过程。
- 当前仍保留 Eino ReAct 执行语义，没有自研 loop。

### 短期（P2.5，3-7 天）

- 补 SSE 事件序列回归测试，确认 `tool_call_start -> tool_call_end -> message -> done` 顺序稳定。
- 给 `HealthCollector` 提供只读 admin 接口或日志聚合，能看到工具成功率、P50/P95/P99 和常见错误。
- 准备 3-5 个真实 AIOps replay case，验证事件链能解释“Agent 查了什么、哪里失败、为什么降级”。
- 将本文和 `AGENTS.md` 的口径统一为“事件驱动层已落地，P3 自研 loop 暂缓”。

P2.5 的核心目标不是新增概念，而是证明当前方案稳定、可观测、可排查、可面试讲清楚。

```text
P0-P2：把能力做出来
P2.5：把能力变成可靠工程资产
P3：只有真的不够用，才自研 Agent Loop
```

### 已完成阶段的原始最小改动

```go
// 最小改动：利用 eino callback 捕获事件，再转成 AgentEvent
sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(
    events.NewModelCallbackEmitter(emitter, traceID).Handler(),
))
```

### 中期（2-3 周）

- 基于 P2.5 replay 结果，判断是否需要进一步加强 steering、beforeToolCall、afterToolCall。
- 如果只读健康接口证明有价值，可以进一步接入前端或 Prometheus 指标。
- 如果事件链已经能解释大多数问题，继续保留 Eino ReAct，不进入 P3。
- 如果 callback / wrapper 仍无法满足动态干预，再重新评估自研 loop。

### 长期（1 月+）

- 如果 callback 满足需求：继续保留 Eino ReAct，完善事件和前端体验
- 如果 callback 不满足：实现自研 loop 的 shadow path，先和 Eino ReAct 并行对比
- 将自研 loop 放在 feature flag 后面灰度，不直接替换主链路
- AIOps Plan-Execute-Replan 单独评估，不纳入第一阶段改造

### 不建议做的事

- 不要把“可观测性问题”直接升级成“必须自研 loop”
- 不要一次性重写所有东西，先用 callback 包装验证事件流设计
- 不要抛弃 eino 的模型适配和工具接口，复用它们
- `chat_multi_agent` 入口和聊天条件路由已删除；保留 `supervisor/triage/reporter` 目录的唯一目的，是作为历史实验和复盘材料

---

## 八、面试回答

### Q1：你的 Agent 架构是怎么设计的？

> 我的项目 OpsCaptain 是一个 AIOps 智能运维助手。最初我尝试了 supervisor → triage → specialists → reporter 的多 agent 管道架构，但发现这个设计过于僵化——每个 specialist 是纯确定性的工具调用器，没有 LLM 推理能力，triage 只是关键词匹配，整体灵活性不够。
>
> 后来我把 Chat 链路收敛到基于 LLM 的 ReAct 模式，用 cloudwego/eino 的 react.Agent 作为执行引擎；复杂 AIOps 分析则保留 Runtime 包装，执行核心是 Plan-Execute-Replan。
>
> 我参考 Pi Agent 的事件驱动设计后，没有选择直接自研 loop，而是先把事件驱动层落到当前 Eino ReAct 链路上：通过 AgentEvent、Eino callback、ToolWrapper、SSE agent_event 和 TraceEmitter，把模型调用、工具调用、耗时、错误等过程事件透给前端和后端 trace。
>
> 目前 P0-P2 已经落地，下一步是 P2.5 稳定化：补事件序列回归测试、工具健康度只读观测、真实 AIOps replay case。只有当这些验证证明 Eino callback / wrapper 仍无法满足运行中 steering、工具调用强控制或自定义终止策略时，才考虑 P3 自研 Agent Loop。

### Q2：为什么不直接用 eino 的 react.Agent？自己写 agent loop 有什么好处？

> 我不会一上来就否定 Eino 的 react.Agent。它的优势是稳定、集成成本低，模型适配和工具调用能力都是现成的。
>
> 真正的问题是：AIOps 场景需要更强的过程可观测性。用户不只想看到最终答案，也想知道系统正在查指标、查日志还是查知识库；工程上也需要知道哪个工具超时、哪个工具返回 degraded。
>
> 所以我把演进拆成两步：第一步先用 Eino callback 做事件层，解决可观测性；第二步再评估是否需要自研 loop 来解决可干预性，比如 steering、beforeToolCall、afterToolCall。
>
> 如果只需要知道工具调用过程，就不应该自研 loop；如果要在工具调用前动态审批、运行中注入用户纠偏，才值得进入自研 loop 的阶段。

### Q3：你的事件流是怎么设计的？

> 我定义了一套统一的 AgentEvent，第一阶段覆盖 model_start/model_end、tool_call_start/tool_call_end、text_delta、error 等关键事件。每个事件都带 trace_id、事件类型、工具名、耗时、错误和安全摘要 payload。
>
> SSE 侧不替换现有 `message/done` 协议，而是新增 `agent_event`。这样老客户端仍然能看到最终回答，新客户端可以额外展示过程状态。工具事件由 ToolWrapper 统一发出，模型事件由 callback 发出，避免重复上报。
>
> 这个设计参考了 Pi Agent 的事件驱动思路，但不会照搬它的完整 runtime。OpsCaption 的重点是先把 AIOps 的过程变得可观察、可审计；P3 自研 loop 暂缓。

### Q4：ProgressiveDisclosure 是什么？解决了什么问题？

> ProgressiveDisclosure 是我设计的分层工具暴露机制。问题是：当工具数量很多时，把所有工具都塞给 LLM 会导致两个问题——token 浪费（每个工具的 schema 都占 context window）和选择困难（工具太多 LLM 容易选错）。
>
> 我把工具分成三层：
> - **AlwaysOn**：通用工具（如搜索、计算器），所有 query 都可见
> - **SkillGate**：领域工具（如查 Prometheus、查日志），只有当 query 匹配到对应 domain 时才暴露
> - **OnDemand**：高级工具，用户显式选择时才加载
>
> 匹配逻辑是 domain-based 的：我预先定义了每个 skill registry 的 domain 关键词，query 经过分词后和 domain 做交集匹配。这样 LLM 只看到当前 query 相关的工具，减少了 token 消耗，也提高了工具选择的准确率。
>
> Pi Agent 没有这个机制，它把所有工具都暴露给 LLM。这是我项目的一个差异化设计。

### Q5：ContextEngine 解决了什么问题？

> ContextEngine 解决的是 context window 预算管理问题。一个 agent 的 context 由多部分组成：历史对话、记忆、检索到的文档、工具输出。如果不做预算控制，很容易超出模型的 context window 限制。
>
> 我设计了 ContextProfile 和 ContextBudget 机制：
> - ContextProfile 按场景（chat / aiops）定义不同的预算分配策略
> - ContextBudget 给每部分（history / memory / docs / tools）分配 token 上限
> - Assembler 按预算裁剪，优先保留高相关性的内容
> - ContextTrace 记录装配细节，方便调试
>
> Pi Agent 没有显式的 context budget 管理，它依赖 LLM provider 的 context window 限制做被动截断。我的方案是主动管理，可以在预算内做更智能的取舍。

### Q6：你从 Pi Agent 学到了什么？

> 三个核心启发：
>
> 第一，**分层架构的威力**。Pi 把 LLM 适配（pi-ai）、agent runtime（pi-agent-core）、应用层（coding-agent）分得很干净。这让我理解到：agent loop 的核心不是堆工具，而是提供机制。工具是应用层的事，loop 只负责调度。
>
> 第二，**事件驱动的设计**。Pi 用 AgentEvent 联合类型覆盖了 agent 的完整生命周期，所有 UI 组件都通过订阅事件来更新。这个模式让同一个 agent loop 可以服务 TUI、Web、聊天 app 等多种前端。
>
> 第三，**黑盒 vs 白盒的 trade-off**。Pi 自己实现 agent loop 是因为它要支持 SDK 嵌入（OpenClaw 就是用它的 SDK）。这让我意识到：选择黑盒还是白盒，取决于你是否需要控制 loop 的每个环节。对于 AIOps 这种需要高可观测性和可干预性的场景，白盒值得评估，但要分阶段验证。

> 但我不会把“白盒”理解成第一步就重写全部 loop。更稳的路径是先把黑盒外面包出统一事件层；当事件层不能满足 steering 和工具拦截时，再把 loop 自研化。

---

## 九、量化设计提升与意义

### 9.1 量化框架：四个维度

```
                    改进前（黑盒）              改进后（事件驱动）
                    ─────────────              ─────────────────
可观测性            最终文本                   全链路事件
用户感知            等 30s → 一段文字           看到实时进度
排查效率            看日志猜                    事件链定位
工具健康            无数据                      耗时/错误率/成功率
```

### 9.2 核心指标及测量方法

#### 维度一：可观测性 → MTTR（平均故障定位时间）

| 指标 | 改进前 | 改进后 | 测量方法 |
|---|---|---|---|
| 工具失败定位时间 | 需查服务端日志，平均 5-15 分钟 | 事件流直接展示哪个工具失败、错误原因，< 30 秒 | 从工具失败到运维人员确认原因的时间 |
| 工具调用链路可见性 | 0%（黑盒） | 100%（每个 tool_call_start/end 都有事件） | 是否能在不看日志的情况下知道 agent 调了哪些工具 |
| 耗时瓶颈定位 | 无法定位，只知道「整体慢」 | 每个工具调用有独立耗时，可精确定位慢在哪 | 从「用户反馈慢」到「定位到慢工具」的时间 |

**具体测量方式：**

```go
// 每个工具调用事件自动携带耗时
type ToolCallEndPayload struct {
    ToolName    string `json:"tool_name"`
    DurationMs  int64  `json:"duration_ms"`
    Success     bool   `json:"success"`
    Error       string `json:"error,omitempty"`
    Summary     string `json:"summary,omitempty"`      // 安全摘要，不暴露原始数据
    AfterError  bool   `json:"after_error,omitempty"`  // after hook 失败时不带 summary
}
```

前后对比实验：
- 准备 10 个真实 AIOps 问题（告警排查、日志分析、知识检索）
- 让 3 个运维人员分别在黑盒模式和事件模式下排查
- 记录：定位时间、操作步骤数、是否需要看服务端日志

#### 维度二：用户体验 → 感知等待时间 + 信任度

| 指标 | 改进前 | 改进后 | 测量方法 |
|---|---|---|---|
| 感知等待时间 | 30s 黑盒等待（用户焦虑） | 30s 实时反馈（用户知道在干嘛） | 用户主观问卷：「你觉得等了多久？」 |
| 中途放弃率 | 高（用户不知道在干嘛，刷新页面） | 低（看到进度，愿意等） | SSE 连接中断率（非网络原因） |
| 重复提问率 | 高（不知道第一次查了什么，换个问法再问） | 低（知道查了什么，直接补充信息） | 同一 session 内相似 query 的重复次数 |
| 信任度 | 低（「AI 到底查没查？」） | 高（「它确实查了 Prometheus，但没数据」） | 用户问卷 + 用户是否采纳 AI 建议的比例 |

**心理学依据：** 进度条效应。研究表明，有进度反馈的等待，用户感知时间比实际时间短 30-40%。即使总耗时不变，用户体验显著提升。

#### 维度三：工程效率 → 问题排查成本

| 指标 | 改进前 | 改进后 | 测量方法 |
|---|---|---|---|
| 线上问题排查步骤 | 5-8 步（看日志 → 猜 → 重试 → 再看日志） | 1-2 步（看事件流 → 定位） | 排查 checklist 步骤数 |
| 工具健康度可见性 | 无 | 有（成功率、P50/P95/P99 耗时） | 事件聚合后的工具健康 dashboard |
| 回归测试覆盖 | 只测最终输出 | 可测中间事件（工具调用序列是否正确） | 测试用例中验证事件序列的比例 |
| Token 浪费可追踪 | 无法追踪 | 每个工具调用的 token 消耗可统计 | 事件中携带 token usage |

**具体测量方式：**

```go
// 事件聚合后可生成工具健康报告
type ToolHealthReport struct {
    ToolName       string
    TotalCalls     int
    SuccessRate    float64       // 成功率
    P50DurationMs  int64         // P50 耗时，毫秒
    P95DurationMs  int64         // P95 耗时，毫秒
    P99DurationMs  int64         // P99 耗时，毫秒
    CommonErrors   []string      // 常见错误
    LastFailure    time.Time     // 最近一次失败
}
```

#### 维度四：成本优化 → Token 节省 + 重试减少

| 指标 | 改进前 | 改进后 | 测量方法 |
|---|---|---|---|
| 工具 schema token 消耗 | 所有工具 schema 都塞进 context | ProgressiveDisclosure 按需暴露 | 对比两种模式下的 input token 数 |
| 无效重试次数 | 高（不知道哪个工具失败，整体重试） | 低（精确知道失败工具，针对性重试） | session 内的重试次数 |
| 上下文溢出率 | 被动截断，可能丢关键信息 | ContextEngine 主动预算管理 | context window 超限触发次数 |

**量化计算：**

```
假设：
- 平均每次查询调用 3 个工具
- 每个工具 schema 占 500 tokens
- ProgressiveDisclosure 平均减少 40% 的工具暴露
- DeepSeek V3 输入价格：¥1 / 1M tokens

单次查询节省：
  3 tools × 500 tokens × 40% = 600 tokens
  ¥0.0006 / 次

日均 1000 次查询：
  600,000 tokens / 天 = ¥0.6 / 天
  年化 ¥219（token 节省本身不大）

但真正的节省在重试减少：
  假设黑盒模式下 20% 的查询需要重试（不知道结果对不对）
  事件模式下重试率降到 5%
  每次重试成本 ≈ ¥0.5（3 工具调用 + LLM 推理）
  日节省：1000 × 15% × ¥0.5 = ¥75 / 天
  年化 ¥27,375
```

### 9.3 意义：不止是技术优化

#### 对产品层面

| 意义 | 说明 |
|---|---|
| **从工具到伙伴** | 黑盒模式下用户把 AI 当「魔法盒子」，事件模式下用户把 AI 当「可信赖的助手」 |
| **降低使用门槛** | 运维人员不需要理解 AI 的工作原理，只需要看事件流就知道它在干嘛 |
| **支撑高级功能** | steering（中途干预）、工具拦截（权限校验）等功能依赖事件层作为基础 |

#### 对工程层面

| 意义 | 说明 |
|---|---|
| **可观测性是基础设施** | 事件层不是「锦上添花」，是 agent 系统的「日志系统」。没有它，线上问题无法排查 |
| **解耦执行与展示** | 事件层让前端和后端独立演进。后端换执行引擎（eino → 自研），前端不需要改 |
| **支撑评测体系** | RAG 评测、工具评测、端到端评测都需要事件数据。没有事件层，评测只能看最终结果 |

#### 对业务层面

| 意义 | 说明 |
|---|---|
| **MTTR 降低** | 运维故障定位时间从分钟级降到秒级，直接影响 SLA |
| **运维效率提升** | 同样的人力可以处理更多告警，降低 oncall 压力 |
| **数据驱动优化** | 工具健康度数据可以指导优化方向：哪个工具最慢？哪个最不稳定？ |

### 9.4 面试中的量化表达

**一句话版本：**

> 这个设计的核心价值是把 agent 从黑盒变成白盒。量化来说：工具失败定位时间从 5-15 分钟降到 30 秒内；用户感知等待时间通过进度反馈降低 30-40%；工具健康度从不可见变成可监控（成功率、P95 耗时、常见错误）。同时 ProgressiveDisclosure 和 ContextEngine 的 token 优化，预计年化节省 ¥27,000+ 的重试成本。

**如果面试官问「怎么证明这些数字？」：**

> P2.5 阶段我会做一组 replay 验证：准备 3-5 个真实 AIOps 问题，不只看最终答案，也看事件链里是否出现了正确的工具调用、失败原因、降级路径和 `done` 收尾。工具健康度数据通过事件聚合自动产出，不需要人工统计。

---

## 十、人话版本

### 这个项目在干嘛？

OpsCaptain 是一个「AI 运维助手」。运维人员用自然语言问它问题，比如「支付服务最近有没有告警」「帮我查一下昨天的慢查询日志」，它会自动调用 Prometheus、日志系统、知识库等工具，把结果汇总成回答。

### 现在的问题是什么？

现在 AI 调用工具的过程是个黑盒。用户问了一个问题，等 30 秒，看到一段回答。但这 30 秒里 AI 干了什么？查了哪些系统？哪个查询超时了？哪个返回了错误？——全都看不到。

就像你请了一个助手去帮你查资料，他出去了半小时回来给你一份报告，但你不知道他去了哪些地方、遇到了什么困难、是不是走错了路。

### 参考 Pi Agent 学到了什么？

Pi 是一个开源的编程助手，它的设计很聪明：AI 的每一步操作（思考、调用工具、工具返回结果）都会发出一个结构化事件，就像一个实时播报员。前端（网页、终端）订阅这些事件，就能实时显示「AI 正在查 Prometheus」「AI 正在搜索日志」「查完了，耗时 2 秒」。

### 打算怎么做？

**不急着大改。** 分阶段走：

1. **第一步（已完成）**：在现有代码上加一层「事件播报」。AI 调用工具时，自动发出事件，前端能看到过程。这一步不改 AI 的执行逻辑，风险很低。

2. **第二步（已完成）**：让前端真正消费这些事件，展示「正在查 XX」「耗时 XX 秒」「成功/失败」。同时通过 ToolWrapper 拿到 before/after hook 和工具健康度统计。

3. **P2.5（当前阶段）**：不急着造新架构，而是补事件顺序测试、工具健康度观测和真实 replay case，证明这套事件层稳定、可解释、可排查。

4. **P3（视情况）**：如果发现事件播报和 ToolWrapper 仍不够，比如必须让用户中途纠正方向、必须自己控制工具执行顺序，再考虑自己实现 AI 的执行循环。这一步工程量大，必须灰度验证。

### 核心判断标准

- **只想看到过程** → 第一步就够了
- **想看到工具健康度和失败原因** → P2.5 就够了
- **想在过程中强干预** → 才考虑 P3
- **想完全控制工具执行顺序** → 才考虑 P3

---

## 十一、给面试官解释的版本

### 一句话概括

> 我参考了 Pi Agent 的事件驱动设计，用分阶段的方式解决 AIOps agent 的可观测性问题，而不是一上来就重写执行引擎。

### 完整版本（3 分钟）

> OpsCaptain 是一个 AIOps 智能运维助手，有两条执行链路：Chat 用 eino 的 ReAct Agent，AIOps 用 Plan-Execute-Replan。
>
> 我在实践中发现一个核心问题：**agent 的执行过程不可观测**。用户问一个问题，系统可能调用了 3-4 个工具（查指标、查日志、查知识库），但用户只能看到最终答案，看不到中间过程。这对 AIOps 场景是不够的——运维人员需要知道系统在查什么、查到了什么、哪个环节出了问题。
>
> 我研究了 Pi Agent 的架构，它用事件驱动的方式解决了这个问题：agent 的每一步（模型调用、工具调用、工具返回）都会发射结构化事件，前端订阅事件流就能实时展示过程。
>
> 但我没有照搬 Pi 的做法。Pi 自己实现了完整的 agent loop，这在它的场景（编程助手、需要 SDK 嵌入）是合理的。但 OpsCaptain 的第一步需求只是「看到过程」，不需要控制 loop 的每个环节。
>
> 所以我把演进拆成了几个阶段：
>
> - **P0（已完成）**：定义 AgentEvent 协议，用 Eino callback 把模型调用过程转成事件。
> - **P1（已完成）**：新增 SSE `agent_event`，前端消费事件并展示工具/模型状态。
> - **P2（已完成）**：用 ToolWrapper 拿到 before/after hook、结果截断、after hook 失败保护和工具健康度统计。
> - **P2.5（当前阶段）**：补事件序列回归测试、只读工具健康观测和真实 AIOps replay case，证明事件层稳定可靠。
> - **P3（暂缓）**：只有 P2.5 证明 Eino callback / wrapper 仍不够，才做自研 loop shadow path。
>
> 这个方案的核心判断是：**可观测性、工具治理和完整 loop 控制是三个不同层级的问题，应该分阶段解决。** 当前用 callback + wrapper 已经能解决大部分可观测和工具治理问题，不需要马上自研 loop。
>
> 我还保留了 OpsCaptain 的差异化设计：ProgressiveDisclosure（分层工具暴露，减少 token 浪费）和 ContextEngine（context window 预算管理），这些是 Pi Agent 没有的能力。

### 如果面试官追问

**Q: 为什么不用现成的 observability 方案（如 LangSmith、LangFuse）？**

> 两个原因。第一，这些方案主要面向通用 LLM 应用，对 AIOps 场景的工具调用（Prometheus 查询、日志搜索）没有针对性的展示。第二，我希望事件层和业务逻辑解耦——AgentEvent 是业务语义的（「正在查告警」），不是技术语义的（「tool call #3 returned」）。这样前端可以直接展示有意义的信息，不需要额外映射。

**Q: 你说的 Plan-Execute-Replan 和 ReAct 有什么区别？**

> ReAct 是「思考-行动-观察」的单轮循环，适合简单的工具调用场景。Plan-Execute-Replan 是「先制定计划，再逐步执行，执行中发现问题重新规划」，适合复杂的多步骤 AIOps 分析。比如「分析支付服务最近 1 小时的异常」，ReAct 可能查一次指标就回答了，Plan-Execute-Replan 会先列出需要检查的维度（指标、日志、依赖服务），逐个检查，发现异常后再深入排查。

**Q: 自研 loop 的风险在哪里？**

> 最大的风险不在 loop 本身，而在需要把现有的生产能力重新挂回去：ContextEngine 的预算管理、ProgressiveDisclosure 的工具筛选、MemoryService 的记忆持久化、错误降级、session 锁、输出过滤、审计日志。这些在 Eino ReAct 链路里已经跑通，自研 loop 需要自己接。所以我把自研 loop 放在 P3，并且要求 shadow run 对比，不直接替换线上。

---

## 十二、参考源码

### Pi Agent 核心文件

- `packages/agent/src/agent-loop.ts` — 双层 while 循环
- `packages/agent/src/types.ts` — AgentEvent、AgentTool、AgentLoopConfig
- `packages/agent/src/agent.ts` — Agent 类（高层封装）
- `packages/ai/src/stream.ts` — streamSimple() 统一入口
- `packages/ai/src/providers/` — 各 LLM provider 实现

### OpsCaptain 当前活跃代码

- `internal/controller/chat/chat_v1_chat_stream.go` — ChatStream 入口（257 行）
- `internal/ai/agent/chat_pipeline/orchestration.go` — eino compose.Graph 构建
- `internal/ai/agent/chat_pipeline/flow.go` — react.Agent 配置 + ProgressiveDisclosure
- `internal/ai/agent/chat_pipeline/model.go` — 模型初始化
- `internal/logic/sse/sse.go` — SSE 基础设施
- `internal/ai/contextengine/` — 上下文装配引擎

### OpsCaptain 废弃代码（仅供参考）

- `internal/ai/agent/supervisor/` — 废弃的 supervisor
- `internal/ai/agent/triage/` — 废弃的 triage
- `internal/ai/agent/reporter/` — 废弃的 reporter
- `internal/ai/agent/skillspecialists/` — 废弃的 skillspecialists
- `internal/ai/service/chat_multi_agent.go` — 已删除
