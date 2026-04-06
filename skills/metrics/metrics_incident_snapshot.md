# metrics_incident_snapshot

## 用途

metrics 域的默认回退 skill。  
适用于“系统现在健康吗”“当前有哪些告警”这类更宽泛的问题。

## 典型触发场景

- incident snapshot
- health snapshot
- 无明显具体关键词的 metrics 问题

## 主要工具

- `query_prometheus_alerts`

## 输出预期

- 当前 alert 快照
- `metrics_mode=incident_snapshot`
- `metrics_focus=incident_snapshot`

## 实现位置

- `internal/ai/agent/skillspecialists/metrics/agent.go`
