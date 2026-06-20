# 告警关联分析设计方案

> 目标：让 AI 能识别"这次告警是独立事件还是上游故障的连锁反应"，并给出故障传播链。
> 分阶段实现，每阶段 review。

---

## 1. 核心问题

当前 AI 查告警只能看到"现在有哪些告警在 firing"，不能回答：
- "这些告警是同时发生的吗？还是有先后顺序？"
- "paymentservice 报错是因为 userservice 先挂了吗？"
- "这次告警影响了哪些下游服务？"

## 2. 架构设计

```
┌─────────────────────────────────────────────────────┐
│              correlate_alerts 工具                    │
│                                                     │
│  1. 获取当前告警 (Prometheus)                         │
│  2. 获取 CMDB 拓扑 (服务依赖关系)                     │
│  3. 按时间窗口聚合告警                                │
│  4. 基于拓扑推断传播链                                │
│  5. 输出: 告警组 + 传播链 + 根因候选                  │
└─────────────────────────────────────────────────────┘
```

## 3. 分阶段实现

### Stage 1: 告警时间窗口聚合
- 从 Prometheus 获取当前告警
- 按 `activeAt` 时间排序
- 按 5 分钟窗口分组
- 输出: 告警时间线

### Stage 2: 拓扑关联
- 获取 CMDB 服务拓扑（依赖关系）
- 检测告警是否沿拓扑链传播
- 识别"根因候选"（最早告警 + 上游无告警的服务）

### Stage 3: 新建 AI 工具 `correlate_alerts`
- 结构化输入: lookback_minutes, cluster
- 输出: 告警组 + 传播链 + 根因分析

### Stage 4: 集成到现有工具 + 测试
- 在 `query_prometheus_alerts` 中增加可选的关联分析
- 全量测试

## 4. 数据结构

```go
// AlertCorrelationResult 关联分析结果
type AlertCorrelationResult struct {
    Success        bool                    `json:"success"`
    AlertGroups    []AlertGroup            `json:"alert_groups"`       // 按时间窗口分组的告警
    Propagation    []PropagationChain      `json:"propagation"`        // 传播链
    RootCandidates []RootCauseCandidate    `json:"root_candidates"`    // 根因候选
    Summary        string                  `json:"summary"`            // AI 可读摘要
}

// AlertGroup 时间窗口内的告警组
type AlertGroup struct {
    WindowStart time.Time         `json:"window_start"`
    WindowEnd   time.Time         `json:"window_end"`
    Alerts      []SimplifiedAlert `json:"alerts"`
    Services    []string          `json:"services"`   // 涉及的服务
}

// PropagationChain 故障传播链
type PropagationChain struct {
    Path      []string `json:"path"`       // [userservice, paymentservice, gateway]
    Direction string   `json:"direction"`  // "upstream_to_downstream"
    Confidence float64 `json:"confidence"` // 0-1
}

// RootCauseCandidate 根因候选
type RootCauseCandidate struct {
    Service    string  `json:"service"`
    AlertName  string  `json:"alert_name"`
    ActiveAt   string  `json:"active_at"`
    Reason     string  `json:"reason"`     // 为什么认为是根因
    Confidence float64 `json:"confidence"`
}
```

## 5. 配置

```yaml
alert_correlation:
  enabled: true
  time_window_minutes: 5        # 告警分组时间窗口
  propagation_confidence: 0.7   # 传播链置信度阈值
```
