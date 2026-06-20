package incidentlifecycle

import (
	"fmt"
	"strings"
	"time"
)

// IncidentSeverity represents the severity level of an incident.
type IncidentSeverity string

const (
	SeverityP0 IncidentSeverity = "P0"
	SeverityP1 IncidentSeverity = "P1"
	SeverityP2 IncidentSeverity = "P2"
	SeverityP3 IncidentSeverity = "P3"
	SeverityP4 IncidentSeverity = "P4"
)

// LifecycleStatus represents the lifecycle stage of an incident.
type LifecycleStatus string

const (
	LifecycleDetected   LifecycleStatus = "detected"
	LifecycleTriaged    LifecycleStatus = "triaged"
	LifecycleResponding LifecycleStatus = "responding"
	LifecycleMitigated  LifecycleStatus = "mitigated"
	LifecycleResolved   LifecycleStatus = "resolved"
	LifecyclePostmortem LifecycleStatus = "postmortem"
	LifecycleCancelled  LifecycleStatus = "cancelled"
)

// ValidTransitions defines which state transitions are allowed.
var ValidTransitions = map[LifecycleStatus][]LifecycleStatus{
	LifecycleDetected:   {LifecycleTriaged, LifecycleCancelled},
	LifecycleTriaged:    {LifecycleResponding, LifecycleCancelled},
	LifecycleResponding: {LifecycleMitigated, LifecycleTriaged, LifecycleCancelled},
	LifecycleMitigated:  {LifecycleResolved, LifecycleCancelled},
	LifecycleResolved:   {LifecyclePostmortem},
	LifecyclePostmortem: {},
	LifecycleCancelled:  {},
}

// IncidentLifecycle tracks the full lifecycle of an incident.
type IncidentLifecycle struct {
	Severity         IncidentSeverity `json:"severity"`
	Status           LifecycleStatus  `json:"lifecycle_status"`
	AffectedServices []string         `json:"affected_services"`
	ImpactSummary    string           `json:"impact_summary"`
	DetectedAt       int64            `json:"detected_at"`
	TriagedAt        int64            `json:"triaged_at"`
	RespondingAt     int64            `json:"responding_at"`
	MitigatedAt      int64            `json:"mitigated_at"`
	ResolvedAt       int64            `json:"resolved_at"`
	PostmortemAt     int64            `json:"postmortem_at"`
	MTTD             int64            `json:"mttd_ms"`
	MTTA             int64            `json:"mtta_ms"`
	MTTR             int64            `json:"mttr_ms"`
}

// Transition attempts to move the lifecycle to a new status.
func (l *IncidentLifecycle) Transition(to LifecycleStatus) error {
	allowed := ValidTransitions[l.Status]
	valid := false
	for _, s := range allowed {
		if s == to {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("cannot transition from %s to %s", l.Status, to)
	}

	now := time.Now().UnixMilli()
	l.Status = to

	switch to {
	case LifecycleTriaged:
		l.TriagedAt = now
		if l.DetectedAt > 0 {
			l.MTTA = now - l.DetectedAt
		}
	case LifecycleResponding:
		l.RespondingAt = now
	case LifecycleMitigated:
		l.MitigatedAt = now
	case LifecycleResolved:
		l.ResolvedAt = now
		if l.DetectedAt > 0 {
			l.MTTR = now - l.DetectedAt
		}
	case LifecyclePostmortem:
		l.PostmortemAt = now
	}

	return nil
}

// InferSeverity guesses the severity based on affected services and alert patterns.
func InferSeverity(affectedServices []string, alertCount int, hasDownstream bool) IncidentSeverity {
	serviceCount := len(affectedServices)

	switch {
	case serviceCount >= 5 || (serviceCount >= 3 && hasDownstream):
		return SeverityP0
	case serviceCount >= 3 || alertCount >= 5:
		return SeverityP1
	case serviceCount >= 2 || alertCount >= 3:
		return SeverityP2
	case serviceCount >= 1:
		return SeverityP3
	default:
		return SeverityP4
	}
}

// SeverityDescription returns a human-readable description of the severity.
func SeverityDescription(s IncidentSeverity) string {
	switch s {
	case SeverityP0:
		return "全站影响：核心链路中断，大量用户受影响"
	case SeverityP1:
		return "严重：核心功能受损，部分用户受影响"
	case SeverityP2:
		return "中等：非核心功能受损，影响有限"
	case SeverityP3:
		return "轻微：单一服务异常，影响较小"
	case SeverityP4:
		return "低优：告警但无明显用户影响"
	default:
		return "未定级"
	}
}

// FormatDuration formats milliseconds as a human-readable duration.
func FormatDuration(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	d := time.Duration(ms) * time.Millisecond
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// ParseSeverity parses a severity string.
func ParseSeverity(s string) IncidentSeverity {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch s {
	case "P0":
		return SeverityP0
	case "P1":
		return SeverityP1
	case "P2":
		return SeverityP2
	case "P3":
		return SeverityP3
	case "P4":
		return SeverityP4
	default:
		return SeverityP3
	}
}
