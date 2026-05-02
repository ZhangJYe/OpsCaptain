# MCP 工具调用链路 Review

> 日期：2026-05-03
> 范围：MCP 工具从发现、注册到调用的完整链路
> 涉及文件：query_log.go, tiered_tools.go, flow.go, tool_wrapper.go, eino_callback.go

---

## 1. 整体架构

```
用户 query
  → ProgressiveDisclosure 选择工具（按 domain 匹配）
  → eino ReAct Agent（LLM 决策调用哪个工具）
  → ToolWrapper.InvokableRun（before/after 拦截）
  → pooledToolWrapper → pooledClient.CallTool（连接池 + 超时 + 重连）
  → MCP Server 执行 → 返回结果
  → after hook 截断 → LLM 处理结果
```

## 2. 各层分析

### 2.1 MCP 客户端层 (`internal/ai/tools/query_log.go`)

**连接池复用**：`mcpClientPool` 按 URL 单例缓存 `pooledClient`，double-check locking 避免重复建连。工具发现结果也按 URL 缓存，避免重复 `e_mcp.GetTools`。

**超时保护**：
- connect 阶段：`context.WithTimeout`，默认 10s，配置项 `mcp.connect_timeout_ms`
- tool call 阶段：`context.WithTimeout`，默认 120s，配置项 `mcp.tool_timeout_ms`

**断线重连**：`pooledClient.reconnect()` 指数退避重连（1s/2s/4s，最多 3 次）。`CallTool` 检测到连接错误时自动触发重连 + 重试一次。

**并发安全**：双锁设计
- `mu`（Mutex）：防止多个 goroutine 同时执行 reconnect
- `rw`（RWMutex）：保护 `cli`/`connected` 字段的并发读写
- `CallTool` 读 `pc.cli` 用 `rw.RLock()`
- `reconnect` 更新 `pc.cli` 用 `rw.Lock()`
- 两者互不阻塞

**错误缓存 TTL**：初始连接失败的错误缓存 5 分钟后自动失效，允许重试连接。避免 MCP Server 暂时不可用导致永久禁用。

**连接错误判定**：`isConnectionError` 精确匹配连接层错误（connection refused/reset/closed, broken pipe, EOF, reset by peer），排除 `context.DeadlineExceeded`（业务超时不触发重连）。

**结果序列化**：`json.Marshal(result)` 输出合法 JSON，而非 `fmt.Sprintf("%v", ...)`。

### 2.2 工具分层 (`internal/ai/tools/tiered_tools.go`)

工具按暴露层级分为三档：

| 层级 | 常量 | 含义 | 示例 |
|------|------|------|------|
| L0 | TierAlwaysOn | 始终暴露 | GetCurrentTime, QueryInternalDocs |
| L1 | TierSkillGate | 按 domain 匹配暴露 | MCP 日志工具(logs), Prometheus(metrics) |
| L2 | TierOnDemand | 按需暴露 | MySQL CRUD |

### 2.3 工具拦截层 (`internal/ai/events/tool_wrapper.go`)

- **beforeToolCall**：`ValidateBeforeToolCall()` — 校验参数非空 + 合法 JSON
- **afterToolCall**：`SummaryAfterToolCall(4000)` — 截断超过 4000 字符的结果

错误处理：工具调用失败时返回 `(formattedError, nil)`，把错误包装成工具结果让 LLM 自行处理。

### 2.4 ReAct Agent 组装 (`internal/ai/agent/chat_pipeline/flow.go`)

```go
config.ToolsConfig.Tools = events.WrapTools(
    config.ToolsConfig.Tools, emitter, traceID,
    events.ValidateBeforeToolCall(),
    events.SummaryAfterToolCall(4000),
)
```

## 3. 已修复的问题清单

| # | 问题 | 严重度 | 修复方案 |
|---|------|--------|----------|
| 1 | `result.Content` 用 `%v` 序列化，输出 Go 内部表示 | 🔴 | 改用 `json.Marshal(result)` |
| 2 | reconnect 持锁 sleep，阻塞 CallTool | 🔴 | 双锁设计：mu 防并发重连，rw 保护 cli 读写 |
| 3 | `isConnectionError` 误判业务超时触发无意义重连 | 🟡 | 排除 `context.DeadlineExceeded`，精确匹配连接错误 |
| 4 | `GetLogMcpTool` 未缓存发现结果 | 🟡 | 新增 `toolCache` 按 URL 缓存 |
| 5 | `pooledToolWrapper` 每次调用查 Info 获取工具名 | 🟢 | 构造时缓存到 `toolName` 字段 |
| 6 | reconnect 竞态：多 goroutine 同时重连 | 🟡 | `mu` Mutex 保护，只有一个 goroutine 执行重连 |
| 7 | 初始连接失败永久缓存 | 🟡 | 错误缓存 TTL 5 分钟，过期自动重试 |
| 8 | `pc.cli` 并发读写无保护 | 🟡 | `rw.RWMutex` 读写锁保护 |

## 4. 当前评分

| 维度 | 评分 | 说明 |
|------|------|------|
| 正确性 | ★★★★★ | MCP 协议完整，序列化正确，并发安全 |
| 可靠性 | ★★★★☆ | 超时 + 重连 + 重试 + 错误 TTL 重试 |
| 性能 | ★★★★☆ | 连接池 + 工具发现缓存 + ProgressiveDisclosure |
| 安全性 | ★★★★☆ | before hook 校验参数，after hook 截断结果 |
| 可观测性 | ★★★★☆ | 事件发射、耗时统计、结果摘要、重连日志 |
| 可维护性 | ★★★★☆ | 分层清晰，职责明确 |

## 5. 剩余可优化项（非阻塞，可后续迭代）

1. **连接池 metrics**：暴露连接数、重连次数、调用耗时等 Prometheus 指标
2. **Graceful shutdown**：agent 退出时关闭所有 MCP 连接
3. **ProgressiveDisclosure 多 domain**：MCP 工具绑定多个 domain（logs + metrics），支持跨域关联
4. **重连后健康检查**：重连成功后 `Ping()` 确认 MCP Server 真正可用
