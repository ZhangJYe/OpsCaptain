# logs_evidence_extract

## 用途

logs 域的通用结构化证据提取 skill。  
适合 error、exception、panic、stack trace 这类问题。

## 典型触发词

- `error`
- `exception`
- `panic`
- `stack`
- `超时`
- `错误`
- `异常`

## Focus

会把日志查询重点拉向：

- error
- timeout
- exception
- panic
- stack trace signals

## 主要工具

- 日志 MCP 工具

## 输出预期

- 结构化日志 evidence
- `skill_mode=evidence_extract`

## 实现位置

- `internal/ai/agent/skillspecialists/logs/agent.go`
