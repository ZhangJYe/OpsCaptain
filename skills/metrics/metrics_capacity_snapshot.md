# metrics_capacity_snapshot

## 用途

用于看 CPU、内存、延迟、吞吐、饱和度这类容量或性能退化信号。

## 典型触发词

- `capacity`
- `latency`
- `cpu`
- `memory`
- `load`
- `throughput`
- `性能`
- `容量`
- `延迟`

## 行为差异

这个 skill 会给出容量相关的 `NextActions`：

- 看 CPU / memory / latency / saturation dashboard
- 判断是否要扩容、限流或调 autoscaling

## 主要工具

- `query_prometheus_alerts`

## 输出预期

- Prometheus alert evidence
- `metrics_mode=capacity_snapshot`
- `metrics_focus=capacity_snapshot`

## 实现位置

- `internal/ai/agent/skillspecialists/metrics/agent.go`
