# logs_payment_timeout_trace

## 用途

用于排查支付、下单、结算链路里的 timeout 问题。

## 典型触发词

- `payment timeout`
- `checkout timeout`
- `order timeout`
- `支付超时`
- `订单超时`
- `gateway timeout`

## Focus

会把日志查询重点拉向：

- payment
- order
- checkout
- gateway timeout
- retry
- db timeout
- downstream latency

## 主要工具

- 日志 MCP 工具

## 输出预期

- 结构化日志 evidence
- `skill_mode=payment_timeout_trace`
- `log_focus=...payment/order/timeout...`

## 实现位置

- `internal/ai/agent/skillspecialists/logs/agent.go`
