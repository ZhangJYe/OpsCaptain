# Chat Multi-Agent Frontend & Runtime Eval

Date: 2026-04-05

## 1. 目标

本轮工作有两个目标：

1. 让前端可以直接识别 `/chat` 返回的是 `legacy` 还是 `multi-agent`
2. 对 Chat 链路的 legacy / multi-agent 行为做一轮可复盘的效果对比

这不是完整的产品评审，而是一次针对“Chat 已接入 Multi-Agent”后的工程验证。

---

## 2. 前端修改内容

### 2.1 Quick Chat 接入新响应字段

后端 `/api/chat` 现在会返回：

- `answer`
- `mode`
- `trace_id`
- `detail`

前端已经改成在普通 Chat 响应里消费这些字段，而不再只读 `answer`。

实现点：

- `SuperBizAgentFrontend/app.js`
  - `sendQuickMessage(...)`
  - `addAssistantMessageWithMeta(...)`
  - `renderAssistantMeta(...)`
  - `renderAssistantDetails(...)`

### 2.2 统一 assistant meta 渲染

之前只有 AI Ops 消息支持：

- Trace ID
- 查看 Trace
- 查看详细步骤

现在普通 Chat assistant 消息也支持统一 meta：

- `Legacy` pill
- `Multi-Agent` pill
- `AI Ops` pill
- 如果有 `trace_id`，则显示 Trace 按钮
- 如果有 `detail`，则显示步骤折叠区

### 2.3 历史记录恢复

历史消息恢复时，前端不再只对 `aiops` 特判。

现在只要 assistant 历史消息里有：

- `meta.mode`

前端就会恢复对应的：

- 模式标记
- detail 折叠区
- trace 入口

### 2.4 Stream Chat 接入 meta 事件

为支持流式模式下的来源识别与 trace 展示，后端 `/api/chat_stream` 新增了：

- `event: meta`

前端已支持解析该事件，并在流式完成后渲染：

- `mode`
- `trace_id`
- `detail`

这意味着：

- quick 模式和 stream 模式都能看到 Chat Multi-Agent 的来源和 trace

---

## 3. 后端配合修改

为配合前端展示，本轮额外补了两点后端改动：

### 3.1 `/api/chat_stream` 增加 `meta` 事件

在以下文件中实现：

- `internal/controller/chat/chat_v1_chat_stream.go`

行为：

- Multi-Agent stream:
  - 先发送 `meta`
  - 再发送 `message`
  - 最后发送 `done`
- Legacy stream:
  - 也会发送 `meta`
  - 但只标记 `mode=legacy`

### 3.2 Chat response shape 统一

在以下文件中实现：

- `api/chat/v1/chat.go`

`ChatRes` 现在包含：

- `Answer`
- `TraceID`
- `Detail`
- `Mode`

---

## 4. 实测 Case

本轮使用临时本地端口 `:8003` 做验证。

### Case A: `/api/chat` 命中 Multi-Agent

请求：

```bash
curl -sS -X POST http://127.0.0.1:8003/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"Id":"session_chat_eval_ops_1","Question":"请分析当前 Prometheus 告警"}'
```

结果摘要：

- `mode = multi_agent`
- `trace_id` 已返回
- `detail` 已返回
- 响应时延约 `4.635557s`

代表性响应特征：

- 前端可显示 `Multi-Agent` pill
- 可打开 `Trace`
- 可查看执行步骤

### Case B: `/api/chat_stream` 命中 Multi-Agent

请求：

```bash
curl -sS -N -X POST http://127.0.0.1:8003/api/chat_stream \
  -H 'Content-Type: application/json' \
  -d '{"Id":"session_chat_eval_stream_1","Question":"请分析当前 Prometheus 告警"}'
```

结果摘要：

- 首先收到 `event: meta`
- `meta` 中包含：
  - `mode = multi_agent`
  - `trace_id`
  - `detail`
- 之后按 chunk 收到 `event: message`
- 最后收到 `event: done`

这验证了：

- 前端 stream 模式现在也能识别 Chat Multi-Agent 来源
- trace 按钮不再只属于 AI Ops

### Case C: `/api/chat` generic query fallback 到 Legacy

请求：

```bash
curl -sS -X POST http://127.0.0.1:8003/api/chat \
  -H 'Content-Type: application/json' \
  -d '{"Id":"session_chat_eval_generic_1","Question":"你好，介绍一下你自己"}'
```

结果摘要：

- `mode = legacy`
- 没有 `trace_id`
- 没有 `detail`
- 响应时延约 `7.806625s`

这验证了：

- 普通聊天仍然保留旧链路
- 多智能体改造不是强制全量替换

### Case D: Chat Multi-Agent 的 trace 查询

请求：

```bash
curl -sS 'http://127.0.0.1:8003/api/ai_ops_trace?trace_id=208809ee-927d-4618-881b-e66726ee4724'
```

结果摘要：

- trace 查询成功
- 能看到 `supervisor / triage / logs / metrics / knowledge / reporter`
- `reporter` 与 `supervisor` 的完成事件已经折叠成短消息

---

## 5. 对比结论

### 5.1 前端体验层

改造前：

- 普通 Chat 无法区分 `legacy` 还是 `multi-agent`
- 普通 Chat 无法展示 trace
- 普通 Chat 无法展示执行步骤
- 只有 AI Ops 有较完整的可观测 UI

改造后：

- 普通 Chat 能显示 `Legacy / Multi-Agent / AI Ops`
- Chat Multi-Agent 能查看 Trace
- Chat Multi-Agent 能查看执行步骤
- Stream 模式也支持 meta 展示

### 5.2 运行行为层

Legacy Chat：

- 更适合通用问答
- 当前仍依赖原 LLM chat pipeline
- 没有 trace / detail

Chat Multi-Agent：

- 更适合运维、告警、日志、知识检索类问题
- 有 trace / detail / mode
- 更容易 debug 和复盘

### 5.3 关于时延的判断

这轮不能简单用 `4.63s vs 7.81s` 下绝对结论，因为：

- 两个请求不是同一个问题
- 两条链路目标不同

更合理的结论是：

- Multi-Agent Chat 已经在当前环境中具备可用性
- Legacy Chat fallback 仍然保留且可工作
- 现在可以开始真实收集“什么类型的问题更适合走 Multi-Agent”

如果要做严格 A/B：

- 需要增加一个临时强制路由开关
- 用同一问题分别跑 legacy 和 multi-agent

---

## 6. 验证结果

已通过：

- `node --check SuperBizAgentFrontend/app.js`
- `go test ./internal/controller/chat ./internal/ai/service ./internal/ai/agent/supervisor`
- `go test ./...`
- `go build ./...`

---

## 7. 后续建议

最值得继续做的不是再改 UI，而是：

1. 给 Chat 前端加一个更明显的来源提示文案
2. 为 Chat Multi-Agent 补更细的 trace 可视化
3. 如果要做严格效果对比，引入“强制 legacy / 强制 multi-agent”评估开关
4. 继续补 knowledge/Milvus 的 tool-level observability
