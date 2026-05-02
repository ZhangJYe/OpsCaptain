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

## 五、Trade-off 分析

### 5.1 为什么要拿回 Agent Loop 控制权？

**一句话结论：不是为了造轮子，而是因为黑盒模式在 AIOps 场景下不可观测、不可干预、不可扩展。**

#### 收益

| 收益 | 具体价值 | 不做的后果 |
|---|---|---|
| **全链路可观测** | 每个工具调用的入参、出参、耗时、错误都可见 | 生产问题只能看最终文本，无法定位是哪个工具出了问题 |
| **实时过程反馈** | 前端能看到「正在查指标」「正在查日志」「正在推理」 | 用户发一个问题等 30 秒只看到一个 loading，不知道在干嘛 |
| **运行中干预** | 用户可以在 agent 执行中发消息纠正方向 | agent 跑偏了只能等它跑完再重新问，浪费 token 和时间 |
| **工具拦截能力** | beforeToolCall 可以做权限校验、参数修正、审计日志 | 工具直接执行，无法在执行前拦截危险操作 |
| **自定义终止逻辑** | 不只 MaxStep，可以根据业务条件提前终止 | 必须跑满 MaxStep 或等 LLM 自己停止 |
| **Schema 校验** | 工具参数不符合 schema 直接拒绝，不浪费一次 LLM 调用 | 参数格式错误导致工具崩溃，再用一轮 LLM 重试 |
| **框架可替换** | 底层可以从 eino 换成其他框架，loop 层不变 | 和 eino 深度耦合，换框架等于重写 |

#### 成本

| 成本 | 严重程度 | 缓解措施 |
|---|---|---|
| **维护负担** | 中 | agent loop 核心逻辑不超过 300 行，复杂度可控 |
| **重实现 eino 特性** | 中 | 工具执行、重试、消息格式化需要自己写，但逻辑简单 |
| **引入 bug 风险** | 中 | eino 的 react.Agent 经过社区验证，自研需要充分测试 |
| **团队学习成本** | 低 | 代码量小，Pi Agent 的设计有成熟参考 |
| **工程时间** | 中 | 分阶段实施，最小改动只需 1 天（callback 包装） |

#### 核心 Trade-off

```
黑盒模式（当前）：
  + 开箱即用，eino 维护 agent loop
  + 工具调用、重试、错误处理都由框架处理
  - 不可观测（中间过程是黑盒）
  - 不可干预（运行中无法注入消息）
  - 不可扩展（无法在工具调用前后插入逻辑）

可控模式（目标）：
  + 全链路可观测
  + 支持 steering 干预
  + 工具调用前后可插入任意逻辑
  + 框架可替换
  - 需要自己维护 agent loop 代码
  - 需要自己处理工具重试、错误恢复
  - 需要 2-4 周工程投入
```

**判断标准：如果你的场景需要「可观测 + 可干预 + 可扩展」，就值得做。如果只是简单的问答，用黑盒就够了。**

OpsCaptain 是 AIOps 场景：
- 用户需要看到 agent 在查什么指标、查什么日志（可观测）
- 用户可能在排查过程中说「不是这个服务，是另一个」（可干预）
- 未来需要在工具调用前做权限校验、审计（可扩展）

→ **值得做。**

### 5.2 为什么不直接抛弃 eino？

**一句话结论：复用 eino 的模型适配和工具接口，只拿回 loop 控制权。**

```
抛弃 eino（不推荐）：
  - 需要自己实现所有 LLM provider 的适配（DeepSeek、豆包、OpenAI...）
  - 需要自己实现工具的 schema 定义、参数解析
  - 工作量大，且不产生业务价值

复用 eino（推荐）：
  + 继续用 eino 的 ToolCallingChatModel 接口（模型适配）
  + 继续用 eino 的 BaseTool 接口（工具定义）
  + 继续用 eino 的 schema.Message（消息格式）
  + 只是在上层包一个自己控制的 loop
  + 工作量小，聚焦在 agent loop 逻辑本身
```

### 5.3 分阶段实施的 Trade-off

| 阶段 | 做什么 | 收益 | 风险 |
|---|---|---|---|
| Phase 1（1天） | 用 eino callback 包装，捕获事件 | 验证事件设计，前端可以看到工具调用 | callback 可能不覆盖所有事件 |
| Phase 2（1-2周） | 自己实现 agent loop | 完全可控，支持 steering | 需要充分测试 |
| Phase 3（1周） | 事件流标准化 + SSE 对接 | 前端体验质变 | 前端需要适配 |
| Phase 4（持续） | Steering + Schema 校验 + 清理废代码 | 架构完整 | 低风险 |

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

## 八、面试回答

### Q1：你的 Agent 架构是怎么设计的？

> 我的项目 OpsCaptain 是一个 AIOps 智能运维助手。最初我尝试了 supervisor → triage → specialists → reporter 的多 agent 管道架构，但发现这个设计过于僵化——每个 specialist 是纯确定性的工具调用器，没有 LLM 推理能力，triage 只是关键词匹配，整体灵活性不够。
>
> 后来我转向了基于 LLM 的 ReAct 模式，用 cloudwego/eino 框架的 react.Agent 作为执行引擎。但我发现 eino 的 react.Agent 是个黑盒——agent loop 的控制权在框架手里，我看不到中间的工具调用过程，无法在运行中干预，也无法在工具调用前后插入逻辑。
>
> 所以我参考了 Pi Agent（一个开源的终端编程 harness）的架构，在 eino 之上包了一层自己可控的 Agent Loop。核心思路是：**复用 eino 的模型适配和工具接口，但拿回 agent loop 的控制权。**
>
> 具体来说，我的 Agent Loop 有三层：
> - 底层：eino 的 ToolCallingChatModel，负责 LLM 调用
> - 中间层：自己实现的双层 while 循环，负责 tool call 调度、事件发射、钩子注入
> - 上层：ProgressiveDisclosure + ContextEngine，负责工具选择和上下文装配

### Q2：为什么不直接用 eino 的 react.Agent？自己写 agent loop 有什么好处？

> 两个核心原因：**可观测性**和**可干预性**。
>
> 第一，可观测性。eino 的 react.Agent 运行时是个黑盒，我只能看到最终的文本输出，看不到它调用了哪些工具、参数是什么、返回了什么。但在 AIOps 场景下，用户需要看到「agent 正在查 Prometheus 指标」「agent 正在搜索日志」这样的过程反馈。没有可观测性，前端只能显示一个 loading，用户体验很差，而且生产问题也无法定位是哪个工具出了问题。
>
> 第二，可干预性。用户在排查故障时可能会中途纠正方向，比如「不是这个服务，是另一个」。eino 的 react.Agent 不支持运行中注入消息，agent 跑偏了只能等它跑完再重新问，浪费 token 和时间。
>
> 我参考了 Pi Agent 的设计，实现了 `getSteeringMessages()` 钩子，在每轮工具执行后检查是否有用户注入的消息。同时通过 `beforeToolCall` / `afterToolCall` 钩子，可以在工具调用前做权限校验、参数修正，调用后做结果过滤、审计日志。
>
> 当然这有代价——需要自己维护 agent loop 代码，处理工具重试和错误恢复。但核心逻辑不超过 300 行，复杂度可控。而且我复用了 eino 的模型适配和工具接口，没有重新造轮子。

### Q3：你的事件流是怎么设计的？

> 我定义了一套 AgentEvent 类型，覆盖 agent 的完整生命周期：agent_start/end、turn_start/end、message_start/update/end、tool_execution_start/end。每个事件携带结构化的 payload。
>
> 前端通过 SSE 订阅这个事件流。当前端收到 `tool_execution_start` 事件时，显示「正在查询 Prometheus 告警」；收到 `text_delta` 时，逐字显示 agent 的推理文本；收到 `tool_execution_end` 时，显示工具返回的摘要。
>
> 这个设计参考了 Pi Agent 的 AgentEvent 联合类型。Pi 用 TypeScript 的 discriminated union 实现了 10 种事件类型，我用 Go 的 struct + string enum 做了等价实现。

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
> - ContextProfile 按场景（chat / aiops / reporter）定义不同的预算分配策略
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
> 第三，**黑盒 vs 白盒的 trade-off**。Pi 自己实现 agent loop 是因为它要支持 SDK 嵌入（OpenClaw 就是用它的 SDK）。这让我意识到：选择黑盒还是白盒，取决于你是否需要控制 loop 的每个环节。对于 AIOps 这种需要高可观测性和可干预性的场景，白盒是必要的。

---

## 九、参考源码

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
