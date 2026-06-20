# 事件全生命周期管理设计方案

> 目标：把 incident 从"对话式排障"升级为"完整事件管理"，覆盖 detected → triaged → responding → mitigated → resolved → postmortem。

---

## 1. 现有系统

当前 `IncidentSession` 有：
- 基础状态：active / running / waiting_approval / completed / degraded / failed
- 多轮排障（Turns）
- 事件日志（Events）

缺失：
- 正式的状态机（detected → triaged → responding → mitigated → resolved → postmortem）
- 严重等级（P0-P4）
- 服务影响范围
- 事件时间线自动拼装
- Postmortem 模板生成

## 2. 生命周期状态机

```
  detected
      │
      ▼
   triaged ──→ responding ──→ mitigated ──→ resolved ──→ postmortem
      │              │            │
      ▼              ▼            ▼
   cancelled     escalated    degraded
```

状态转换规则：
| 当前状态 | 允许转换到 | 触发条件 |
|---------|-----------|---------|
| detected | triaged, cancelled | 人工定级或自动定级 |
| triaged | responding | 开始处置 |
| responding | mitigated, degraded, escalated | 处置结果 |
| mitigated | resolved | 确认恢复 |
| resolved | postmortem | 生成复盘 |
| any | cancelled | 取消事件 |

## 3. 数据模型扩展

```go
type IncidentSeverity string // P0, P1, P2, P3, P4

type IncidentLifecycle struct {
    Severity        IncidentSeverity `json:"severity"`
    LifecycleStatus string           `json:"lifecycle_status"` // detected/triaged/responding/mitigated/resolved/postmortem
    AffectedServices []string        `json:"affected_services"`
    ImpactSummary   string           `json:"impact_summary"`
    DetectedAt      int64            `json:"detected_at"`
    TriagedAt       int64            `json:"triaged_at"`
    RespondingAt    int64            `json:"responding_at"`
    MitigatedAt     int64            `json:"mitigated_at"`
    ResolvedAt      int64            `json:"resolved_at"`
    PostmortemAt    int64            `json:"postmortem_at"`
    MTTD            int64            `json:"mttd_ms"` // Mean Time To Detect
    MTTA            int64            `json:"mtta_ms"` // Mean Time To Acknowledge
    MTTR            int64            `json:"mttr_ms"` // Mean Time To Resolve
    Postmortem      *Postmortem      `json:"postmortem,omitempty"`
}
```

## 4. Postmortem 模板

```go
type Postmortem struct {
    Title           string            `json:"title"`
    Summary         string            `json:"summary"`
    Severity        string            `json:"severity"`
    Duration        string            `json:"duration"`
    Timeline        []TimelineEntry   `json:"timeline"`
    RootCause       string            `json:"root_cause"`
    Impact          string            `json:"impact"`
    ActionItems     []ActionItem      `json:"action_items"`
    LessonsLearned  []string          `json:"lessons_learned"`
}

type TimelineEntry struct {
    Time    string `json:"time"`
    Event   string `json:"event"`
    Agent   string `json:"agent,omitempty"`
    Detail  string `json:"detail,omitempty"`
}
```

## 5. 分阶段实现

### Stage 1: 生命周期状态机 + 严重等级
- 扩展 IncidentSession 数据模型
- 实现状态转换逻辑
- 自动严重等级推断

### Stage 2: 服务影响追踪
- 从告警/拓扑自动推断受影响服务
- 计算 MTD/MTTA/MTTR

### Stage 3: 时间线拼装 + Postmortem
- 从 Events 自动组装 Timeline
- 生成 Postmortem 模板
- HTTP API 暴露

### Stage 4: 测试 + 验证
