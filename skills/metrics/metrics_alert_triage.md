# metrics_alert_triage

## 用途

这是 metrics 域的通用告警分诊 skill。

## 典型触发词

- `alert`
- `alerts`
- `prometheus`
- `firing`
- `severity`
- `告警`
- `报警`

## 主要工具

- `query_prometheus_alerts`

## 输出预期

- 当前 firing alerts
- `metrics_mode=alert_triage`
- `metrics_focus=alert_triage`

## 边界

如果 query 更像发布评估或容量快照，应优先由更具体的 skill 命中。

## 实现位置

- `internal/ai/agent/skillspecialists/metrics/agent.go`
