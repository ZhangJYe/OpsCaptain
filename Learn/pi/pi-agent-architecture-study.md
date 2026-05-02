# Pi Agent 架构研究与 OpsCaptain 对标分析

> 当前统一口径（2026-05）
> - 当前实现：Chat = `ContextEngine / MemoryService -> Eino ReAct Agent -> Tools / RAG`；AIOps = `Approval / Degradation / Memory -> Runtime -> Plan-Execute-Replan`
> - 本文里如果提到 `supervisor / triage / reporter / skillspecialists / chat_multi_agent`，应理解为历史实验或演进背景。

> 日期：2026-05-02
> 状态：待 Review

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

### 设计思路

**不是抛弃 eino，而是在 eino react.Agent 之上包一层 Pi 风格的 Agent Loop。**

```
当前：
  ChatStream → eino react.Agent (黑盒) → SSE

目标：
  ChatStream → AgentLoop (Pi 风格) → eino react.Agent (作为 LLM+Tool 引擎) → SSE
                      ↑
               这一层是我们自己控制的
```

### P0：Agent Loop 封装层（最高优先级）

在 eino react.Agent 之上封装一个可控的 agent loop：

```go
// internal/ai/loop/loop.go
type AgentLoop struct {
    model    eino.ToolCallingChatModel   // 底层用 eino 的模型接口
    tools    []eino.BaseTool              // 工具集
    hooks    LoopHooks                    // 生命周期钩子
    config   LoopConfig                   // MaxStep、超时等
}

type LoopHooks struct {
    OnToolCallStart  func(ctx context.Context, event ToolCallStartEvent)
    OnToolCallEnd    func(ctx context.Context, event ToolCallEndEvent)
    OnThinking       func(ctx context.Context, delta string)
    OnText           func(ctx context.Context, delta string)
    OnTurnStart      func(ctx context.Context)
    OnTurnEnd        func(ctx context.Context)
    BeforeToolCall   func(ctx context.Context, tc ToolCallContext) (*BeforeResult, error)
    AfterToolCall    func(ctx context.Context, tc ToolCallContext) (*AfterResult, error)
    GetSteering      func(ctx context.Context) []Message
}

func (l *AgentLoop) Run(ctx context.Context, messages []Message) <-chan AgentEvent {
    // 自己实现双层循环，而不是委托给 react.Agent
    // 1. 调用 LLM（通过 eino 的 model.Generate/Stream）
    // 2. 解析响应中的 tool_call
    // 3. 校验参数 schema
    // 4. 调用 beforeToolCall 钩子（可拦截）
    // 5. 执行工具
    // 6. 调用 afterToolCall 钩子（可覆写结果）
    // 7. 检查 steering 消息
    // 8. 发射事件到 channel
    // 9. 循环直到无更多 tool_call 或达到 MaxStep
}
```

这样做的好处：
- 不需要重写 eino 的模型适配（继续用 eino 的 ToolCallingChatModel）
- 不需要重写工具实现（继续用 eino 的 BaseTool）
- 但 agent loop 的控制权回到自己手里
- 可以在每个环节插入钩子、发射事件

### P1：事件流标准化

```go
// internal/ai/loop/events.go
type AgentEventType string

const (
    EventAgentStart       AgentEventType = "agent_start"
    EventAgentEnd         AgentEventType = "agent_end"
    EventTurnStart        AgentEventType = "turn_start"
    EventTurnEnd          AgentEventType = "turn_end"
    EventMessageStart     AgentEventType = "message_start"
    EventTextDelta        AgentEventType = "text_delta"
    EventThinkingDelta    AgentEventType = "thinking_delta"
    EventToolCallStart    AgentEventType = "tool_call_start"
    EventToolCallEnd      AgentEventType = "tool_call_end"
    EventSteeringInjected AgentEventType = "steering_injected"
    EventError            AgentEventType = "error"
)

type AgentEvent struct {
    Type      AgentEventType
    Payload   any
    Timestamp time.Time
}
```

SSE 层订阅这个事件流，前端就能看到完整的 agent 执行过程。

### P2：Steering 机制

```go
type SteeringChannel struct {
    ch chan Message
}

func (s *SteeringChannel) Inject(msg Message) {
    select {
    case s.ch <- msg:
    default:
    }
}

// 在 agent loop 的每轮 tool 执行后检查
pendingSteering := l.hooks.GetSteering(ctx)
if len(pendingSteering) > 0 {
    messages = append(messages, pendingSteering...)
    emit(EventSteeringInjected, pendingSteering)
}
```

### P3：工具 Schema 校验

```go
type ValidatedTool struct {
    Name        string
    Description string
    Schema      *jsonschema.Schema   // JSON Schema 定义
    Execute     func(ctx context.Context, params any) (*ToolResult, error)
    ExecutionMode ExecutionMode      // sequential | parallel
}

func (t *ValidatedTool) ValidateAndExecute(ctx context.Context, rawArgs json.RawMessage) (*ToolResult, error) {
    var params any
    if err := jsonschema.Validate(t.Schema, rawArgs, &params); err != nil {
        return nil, fmt.Errorf("tool %s: schema validation failed: %w", t.Name, err)
    }
    return t.Execute(ctx, params)
}
```

---

## 五、OpsCaptain 现有优势（应保留）

1. **ContextEngine**：预算控制 + 装配 trace，比 Pi 更精细。Pi 没有显式的 context budget 管理。
2. **ProgressiveDisclosure**：分层工具暴露（AlwaysOn / SkillGate / OnDemand），Pi 没有这个机制。
3. **Skill-driven 工具组织**：domain-scoped + keyword matching，结构化程度比 Pi 的 skill 更高。
4. **SSE 基础设施**：已有的 SSE Service 可以直接复用，只需对接新的事件流。
5. **错误降级模式**：ResultStatusDegraded 全链路使用，Pi 没有这个。
6. **Memory 持久化**：MemoryService 封装了记忆的构建和持久化。
7. **GoFrame 生态**：配置管理、中间件、日志等基础设施已就绪。

---

## 六、执行建议

### 短期（1 周）

- P0 最小实现：封装一个 AgentLoop，内部仍然调用 eino react.Agent，但在外层包装事件发射
- 这一步不需要重写 agent loop，只需要 hook 进 eino 的回调机制

```go
// 最小改动：利用 eino 的 compose.WithCallbacks 捕获事件
sr, err := runner.Stream(ctx, userMessage, compose.WithCallbacks(
    &AgentEventCallback{emit: eventCh},
))
```

### 中期（2-3 周）

- P0 完全实现：自己实现 agent loop，不再依赖 react.Agent 黑盒
- P1：事件流标准化，SSE 推送完整事件
- 前端适配新的事件类型

### 长期（1 月+）

- P2：Steering 机制
- P3：工具 Schema 校验
- 清理废弃的 multi-agent pipeline 代码

### 不建议做的事

- 不要一次性重写所有东西，先用 callback 包装验证事件流设计
- 不要抛弃 eino 的模型适配和工具接口，复用它们
- `chat_multi_agent` 入口和聊天条件路由已删除；保留 `supervisor/triage/reporter` 目录的唯一目的，是作为历史实验和复盘材料

---

## 七、参考源码

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
