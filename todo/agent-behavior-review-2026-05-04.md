# Agent 行为 Review：为什么"一点都不像 Agent"

日期：2026-05-04
状态：修复中

---

## 现象

用户反馈：系统完全不像 Agent，不会主动调用工具获取数据，只是直接回复文本。

## 全链路追踪

```
用户发消息
  → 前端默认 mode: "quick"
    → 后端 /chat
      → runner.Invoke()（单次 LLM 调用）
        → 模型返回文本
          → 结束（从未进入工具调用循环）
```

---

## 问题清单

### 问题 1（致命）：前端默认 quick 模式，完全绕过 Agent

文件：`SuperBizAgentFrontend/src/hooks/useChat.ts:334`

```typescript
const [mode, setMode] = useState<ChatMode>("quick");
```

Quick 模式调用 `/chat`，后端走 `runner.Invoke()` — 单次 LLM 调用，不走流式，不展示工具调用过程。模型回复一段文本就结束了。

### 问题 2（致命）：StreamToolCallChecker 只看第一个 chunk

文件：eino 框架 `flow/agent/react/react.go:129-151`

默认的 `firstChunkStreamToolCallChecker` 只检查第一个流式 chunk：

```go
if len(msg.ToolCalls) > 0 { return true, nil }   // 有 tool_calls → 调工具
if len(msg.Content) == 0 { continue }             // 跳过空 chunk
return false, nil                                  // 有文本内容 → 直接结束！
```

GLM-4.5-AIR 的行为：先输出文本（如"我来帮你查一下"），再（可能）输出 tool_calls。
但 checker 看到第一个 chunk 有文本，立刻返回 false → Agent 认为"不需要调工具" → compose.END。

结果：Agent 永远不会调用工具，只是一个普通聊天机器人。

### 问题 3（根本）：GLM-4.5-AIR 能力不足

配置文件：`manifest/config/config.yaml`

```yaml
glm_chat_model:       → GLM-4.5-AIR
glm_chat_model_fast:  → GLM-4.5-AIR  # 和 regular 用同一个模型
```

GLM-4.5-AIR 是轻量模型，function calling 能力不可靠：
- 不一定输出标准的 tool_calls 格式
- 即使输出，也可能在文本之后
- 参数解析准确率低

### 问题 4：System Prompt 没有强制工具调用

当前 prompt 说"当用户提到运维问题时，必须先调用工具"，但这是建议，不是强制。
弱模型会忽略这类软约束。

### 问题 5：工具被 Progressive Disclosure 限制

Always-on 工具只有：
- `get_current_time` — 几乎没用
- `query_internal_docs` — 检索知识库

真正有用的运维工具被关在 TierSkillGate：
- `query_logs` — 需要匹配 "logs" domain
- `query_prometheus_alerts` — 需要匹配 "metrics" domain

如果 Progressive Disclosure 的关键词匹配没命中，这些工具根本不会注册到 Agent 里。

### 问题 6：每次请求都重建 Agent

文件：`internal/controller/chat/chat_v1_chat.go:136`

```go
runner, err := buildChatAgent(ctx, msg)  // 每次请求都新建
```

每次聊天都重新创建 react.Agent、重新编译 graph、重新初始化模型。浪费资源。

---

## 修复计划（不换模型）

| # | 修复项 | 文件 | 优先级 |
|---|--------|------|--------|
| 1 | 前端默认改为 stream 模式 | useChat.ts | P0 |
| 2 | 自定义 StreamToolCallChecker | chat_pipeline/flow.go | P0 |
| 3 | query_logs/query_alerts 移到 TierAlwaysOn | tools/tiered_tools.go | P1 |
| 4 | System prompt 增加强制工具调用指令 | chat_pipeline/prompt.go | P1 |
| 5 | 缓存 Agent 实例 | chat_pipeline/flow.go | P2 |

---

## 修复记录

（修复完成后更新）
