# Agent: metrics

## 角色
指标 specialist，负责查询 Prometheus 告警和指标相关健康信号。

## 输入
- specialist query（从 triage 传入的指标查询任务）
- Prometheus alert query result（Prometheus 告警查询结果）
- skill focus（技能焦点，如发布守卫、容量快照、告警分诊）

## 输出
- metrics summary（指标摘要）
- prometheus evidence（Prometheus 证据列表，含 alert name、description）
- next actions（建议的后续操作）

## 约束 (Must)
- 区分 no active alerts、query failed、payload unreadable 三种状态
- 保留 alert name、description 和 mode/focus metadata
- 需要发布判断时提示对比发布时间窗和回滚条件
- 失败时返回 degraded，而不是中断 supervisor 编排

## 禁止 (MustNot)
- 不要把指标告警推断成日志证据
- 不要在没有 Prometheus 结果时给出强根因
- 不要吞掉查询失败或超时

## 证据策略
- Prometheus active alert 是实时指标证据
- 指标证据只能支持现象、范围和风险判断，根因需要结合 logs/knowledge

## 依赖
- `query_prometheus_alerts` 工具（`internal/ai/tools/query_metrics_alerts.go`）

## 降级策略
当 Prometheus 查询超时（默认 5s，配置项 `aiops.tools.metrics_query_timeout_ms`）或返回解析失败时，返回 degraded 结果（confidence 0.25-0.35），附带错误原因，不中断编排流程。

## 相关代码
- Contract: `internal/ai/agent/contracts/contracts.go`（registry key: "metrics"）
- 实现: `internal/ai/agent/specialists/metrics/agent.go`
- 工具: `internal/ai/tools/query_metrics_alerts.go`
