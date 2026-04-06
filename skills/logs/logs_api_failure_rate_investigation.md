# logs_api_failure_rate_investigation

## 用途

用于排查接口失败率升高、5xx 激增、上下游调用异常这类 API 故障。

## 典型触发词

- `api failure rate`
- `error rate`
- `5xx`
- `4xx`
- `response error`
- `endpoint`
- `route`
- `upstream`
- `downstream`

## Focus

命中后会把日志查询重点拉向：

- api name
- route
- status code
- response payload
- upstream
- downstream
- timeout
- dependency failures

## 主要工具

- 日志 MCP 工具

## 输出预期

- 结构化日志 evidence
- `skill_mode=api_failure_rate_investigation`
- `log_focus=...status code/upstream/downstream...`
- 推荐的 `next_actions`

## 适合这个 skill 的原因

- 它比“支付超时”或“通用日志提证”更像独立业务场景
- 可以复用在所有接口失败率飙升的故障排查里
- 后续非常适合和 metrics specialist 联动

## 实现位置

- `internal/ai/agent/skillspecialists/logs/agent.go`
- `internal/ai/agent/skillspecialists/logs/agent_test.go`
