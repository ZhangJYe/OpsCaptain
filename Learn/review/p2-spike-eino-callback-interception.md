# P2 Spike 结论：Eino Callback 拦截能力验证

> 日期：2026-05-02
> 结论：eino callback 是纯观测机制，不支持拦截。如需 steering / beforeToolCall / afterToolCall，必须自研 loop。

---

## 验证结果

| 能力 | 是否可行 | 原因 |
|---|---|---|
| Steering（运行中注入消息） | ❌ 不可行 | callback 只返回 context.Context，无法修改 agent 的消息列表 |
| BeforeToolCall（拦截/阻断工具调用） | ❌ 不可行 | OnStart 无 error 返回值，无法阻止后续执行 |
| AfterToolCall（覆写工具结果） | ❌ 不可行 | OnEnd 返回的 context 被丢弃，原始 output 不变 |

## 源码证据

### 1. Callback 只能修改 context

```go
// internal/callbacks/interface.go
type Handler interface {
    OnStart(ctx context.Context, info *RunInfo, input CallbackInput) context.Context
    OnEnd(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context
    OnError(ctx context.Context, info *RunInfo, err error) context.Context
    // ...
}
```

所有方法只返回 `context.Context`，不返回修改后的 input/output。

### 2. OnStartHandle 丢弃修改

```go
// internal/callbacks/inject.go:107-115
func OnStartHandle[T any](ctx context.Context, input T,
    runInfo *RunInfo, handlers []Handler) (context.Context, T) {
    for i := len(handlers) - 1; i >= 0; i-- {
        ctx = handlers[i].OnStart(ctx, runInfo, input)
    }
    return ctx, input  // ← 返回原始 input，不是 callback 修改后的
}
```

### 3. OnEndHandle 同样丢弃修改

```go
// internal/callbacks/inject.go:117-125
func OnEndHandle[T any](ctx context.Context, output T,
    runInfo *RunInfo, handlers []Handler) (context.Context, T) {
    for _, handler := range handlers {
        ctx = handler.OnEnd(ctx, runInfo, output)
    }
    return ctx, output  // ← 返回原始 output
}
```

### 4. 执行流程无拦截点

```go
// compose/utils.go:100-116
ctx, input = onStart(ctx, input)     // callback 执行，但 input 不变
output, err = r(ctx, input, opts...) // 实际执行，无条件
ctx, output = onEnd(ctx, output)     // callback 执行，但 output 不变
return output, nil
```

callback 和实际执行之间没有检查点。

### 5. React Agent 内部无消息注入机制

react.Agent 的 state 只有 Messages 和 ReturnDirectlyToolCallID。工具执行后，结果直接追加到 Messages，没有回调可以注入新消息。

## 可行的替代方案

eino 内部有一些非 callback 的拦截点，但它们是框架内部 API，不对外暴露：

| 方案 | 能力 | 限制 |
|---|---|---|
| StatePreHandler/StatePostHandler | 可修改 agent state | react.Agent 内部 API，不对外暴露 |
| MessageModifier/MessageRewriter | 可修改消息 | 只在 model 调用前触发，不在工具调用前后 |
| 自定义工具包装 | 可修改工具输入/输出 | 每个工具都要包装，侵入性强 |
| Lambda 节点 | 可在 graph 中插入处理节点 | 需要重新编排 graph，改动大 |

## 结论

**如果 OpsCaptain 需要 steering / beforeToolCall / afterToolCall，必须自研 agent loop。**

eino callback 的定位是「观测」，不是「拦截」。这和我们 P0 的设计完全一致——先用 callback 做可观测性，这是正确的。

对于拦截能力，有两种路径：

### 路径 A：自研 loop（推荐）

自己实现 agent loop，复用 eino 的 model.ToolCallingChatModel 和 tool.BaseTool 接口：

```go
type AgentLoop struct {
    model  model.ToolCallingChatModel
    tools  []tool.BaseTool
    hooks  LoopHooks
}

type LoopHooks struct {
    BeforeToolCall func(ctx, call) (*BeforeResult, error)  // 可拦截
    AfterToolCall  func(ctx, call) (*AfterResult, error)   // 可覆写
    GetSteering    func(ctx) []Message                      // 可注入
    Emit           func(ctx, event)                         // 可观测
}

func (l *AgentLoop) Run(ctx, messages) <-chan AgentEvent {
    for {
        resp := l.model.Generate(ctx, messages)
        if len(resp.ToolCalls) == 0 { break }
        
        for _, tc := range resp.ToolCalls {
            if result, err := l.hooks.BeforeToolCall(ctx, tc); err != nil {
                // 拦截：返回错误结果给 LLM
                messages = append(messages, errorResult(tc, err))
                continue
            }
            result := executeTool(tc)
            if modified, err := l.hooks.AfterToolCall(ctx, tc, result); err == nil {
                result = modified
            }
            messages = append(messages, result)
        }
        
        // steering 检查
        if steering := l.hooks.GetSteering(ctx); len(steering) > 0 {
            messages = append(messages, steering...)
        }
    }
}
```

成本：2-3 周，主要是把现有生产能力（ContextEngine、ProgressiveDisclosure、MemoryService 等）重新挂回去。

### 路径 B：工具包装（低成本替代）

如果只需要 beforeToolCall/afterToolCall，不需要 steering，可以用工具包装：

```go
type WrappedTool struct {
    inner   tool.BaseTool
    before  func(ctx, args) (args, error)
    after   func(ctx, result) (result, error)
}

func (w *WrappedTool) InvokableRun(ctx, args string) (string, error) {
    if w.before != nil {
        args, err = w.before(ctx, args)
        if err != nil { return "", err }
    }
    result, err := w.inner.InvokableRun(ctx, args)
    if w.after != nil && err == nil {
        result, err = w.after(ctx, result)
    }
    return result, err
}
```

成本：3-5 天。但不支持 steering，且每个工具都要手动包装。

## 推进建议

| 需求 | 路径 | 优先级 |
|---|---|---|
| 可观测性 | P0 callback（已完成） | ✅ 已交付 |
| 工具拦截 | 路径 B 工具包装 | 中期 |
| 结果覆写 | 路径 B 工具包装 | 中期 |
| Steering | 路径 A 自研 loop | 长期（需验证必要性） |
| 完整 loop 控制 | 路径 A 自研 loop | 长期（灰度验证） |

**当前阶段建议：继续用 callback 做可观测性，不急着自研 loop。如果 steering 需求不强烈，路径 B 的工具包装可以覆盖大部分拦截场景。**
