# logs_service_offline_panic_trace

## 用途

用于排查服务下线、Pod 重启、CrashLoopBackOff、panic 这类组合场景。

## 典型触发词

- `service offline`
- `service down`
- `pod restart`
- `crashloop`
- `panic`
- `stack trace`
- `nil pointer`

## Focus

命中后会把日志查询重点拉向：

- panic
- stack trace
- restart reason
- crashloop
- oom
- pod restart count
- latest release

## 主要工具

- 日志 MCP 工具

## 输出预期

- 结构化日志 evidence
- `skill_mode=service_offline_panic_trace`
- `log_focus=...panic/restart/crashloop...`
- 推荐的 `next_actions`

## 适合这个 skill 的原因

- 它不是普通“看日志”，而是稳定的排障套路
- 需要同时满足 `panic` 和 `offline/restart` 两组信号
- 比通用 `logs_evidence_extract` 更适合做服务下线排障

## 实现位置

- `internal/ai/agent/skillspecialists/logs/agent.go`
- `internal/ai/agent/skillspecialists/logs/agent_test.go`
