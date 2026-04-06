# metrics_release_guard

## 用途

用于发布或灰度期间判断 Prometheus 告警是否会阻断继续 rollout。

## 典型触发词

- `release`
- `deploy`
- `rollout`
- `上线`
- `发版`
- `部署`

## 行为差异

和通用 metrics skill 不同，这个 skill 会补充面向发布场景的 `NextActions`：

- 对比告警开始时间和发布窗口
- 检查 canary / rollback criteria

## 主要工具

- `query_prometheus_alerts`

## 输出预期

- Prometheus alert evidence
- `metrics_mode=release_guard`
- `metrics_focus=release_guard`
- 发布相关 `NextActions`

## 实现位置

- `internal/ai/agent/skillspecialists/metrics/agent.go`
