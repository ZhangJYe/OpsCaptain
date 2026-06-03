# Agent: logs

## 角色
日志 specialist，负责通过 MCP 日志工具抽取错误、超时、panic、依赖失败等证据。

## 输入
- specialist query（从 triage 传入的日志查询任务）
- log MCP tools（MCP 日志工具列表）
- skill focus（技能焦点，如 panic trace、API failure、payment timeout）

## 输出
- log summary（日志摘要）
- log evidence（日志证据列表，含来源工具、标题、片段）
- tool errors（工具调用错误列表）

## 约束 (Must)
- 区分结构化日志证据和 raw log fallback
- 保留 successful_tool、tool_errors、log_mode、log_focus metadata
- 日志工具不可用时返回 degraded

## 禁止 (MustNot)
- 不要把历史复盘标签当实时日志证据
- 不要把 raw output 伪装成已结构化验证的结论
- 不要因为单个日志工具失败就终止全部日志排查

## 证据策略
- 日志 evidence 必须包含来源工具、标题和片段
- raw log fallback 只能作为弱证据（score 0.44），需提示后续验证

## 依赖
- `query_log` 工具（MCP 日志查询，`internal/ai/tools/query_log.go`）
- 工具发现: `tools.GetLogMcpTool`

## 降级策略
当 MCP 日志工具初始化失败、所有工具调用失败、或仅获取到 raw output 时，返回 degraded 结果（confidence 0.28-0.42）。单个工具失败不终止整体排查，继续尝试其他工具。

配置项：
- `aiops.tools.log_query_timeout_ms`：单次查询超时（默认 3s）
- `aiops.tools.log_evidence_limit`：最大证据条数（默认 3）

## 相关代码
- Contract: `internal/ai/agent/contracts/contracts.go`（registry key: "logs"）
- 实现: `internal/ai/agent/specialists/logs/agent.go`
- 工具: `internal/ai/tools/query_log.go`
