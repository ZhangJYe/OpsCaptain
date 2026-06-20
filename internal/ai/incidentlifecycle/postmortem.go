package incidentlifecycle

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// TimelineEntry represents a single event in the incident timeline.
type TimelineEntry struct {
	Time   string `json:"time"`
	Event  string `json:"event"`
	Agent  string `json:"agent,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// ActionItem represents a follow-up action from postmortem.
type ActionItem struct {
	Assignee string `json:"assignee"`
	Action   string `json:"action"`
	Priority string `json:"priority"` // high, medium, low
}

// Postmortem is the incident postmortem document.
type Postmortem struct {
	Title          string           `json:"title"`
	Summary        string           `json:"summary"`
	Severity       string           `json:"severity"`
	Duration       string           `json:"duration"`
	DurationMs     int64            `json:"duration_ms"`
	Timeline       []TimelineEntry  `json:"timeline"`
	RootCause      string           `json:"root_cause"`
	Impact         string           `json:"impact"`
	ActionItems    []ActionItem     `json:"action_items"`
	LessonsLearned []string         `json:"lessons_learned"`
}

// RawEvent is a minimal event representation for timeline assembly.
type RawEvent struct {
	Timestamp int64
	Type      string
	Message   string
	Agent     string
}

// AssembleTimeline converts raw events into a sorted timeline.
func AssembleTimeline(events []RawEvent) []TimelineEntry {
	if len(events) == 0 {
		return nil
	}

	sorted := make([]RawEvent, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp < sorted[j].Timestamp
	})

	var timeline []TimelineEntry
	for _, e := range sorted {
		t := time.UnixMilli(e.Timestamp).Format("15:04:05")
		timeline = append(timeline, TimelineEntry{
			Time:   t,
			Event:  e.Message,
			Agent:  e.Agent,
			Detail: e.Type,
		})
	}
	return timeline
}

// GeneratePostmortem creates a postmortem from lifecycle and timeline data.
func GeneratePostmortem(
	title string,
	severity IncidentLifecycle,
	timeline []TimelineEntry,
	affectedServices []string,
	impactSummary string,
) *Postmortem {
	durationMs := severity.MTTR
	if durationMs <= 0 && severity.MitigatedAt > 0 && severity.DetectedAt > 0 {
		durationMs = severity.MitigatedAt - severity.DetectedAt
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## 事件摘要\n\n"))
	sb.WriteString(fmt.Sprintf("严重等级：%s\n", severity.Severity))
	sb.WriteString(fmt.Sprintf("持续时间：%s\n", FormatDuration(durationMs)))
	sb.WriteString(fmt.Sprintf("受影响服务：%s\n", strings.Join(affectedServices, ", ")))
	sb.WriteString(fmt.Sprintf("\n%s\n", impactSummary))

	postmortem := &Postmortem{
		Title:      title,
		Summary:    sb.String(),
		Severity:   string(severity.Severity),
		Duration:   FormatDuration(durationMs),
		DurationMs: durationMs,
		Timeline:   timeline,
		Impact:     impactSummary,
		ActionItems: []ActionItem{
			{Assignee: "SRE Team", Action: "补充监控覆盖", Priority: "high"},
			{Assignee: "SRE Team", Action: "更新排障 Runbook", Priority: "medium"},
			{Assignee: "开发团队", Action: "增加服务韧性测试", Priority: "medium"},
		},
		LessonsLearned: []string{
			"告警关联分析帮助快速定位根因",
			"服务依赖拓扑对故障传播分析有价值",
		},
	}

	return postmortem
}

// FormatPostmortemMarkdown formats the postmortem as Markdown.
func FormatPostmortemMarkdown(pm *Postmortem) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Postmortem: %s\n\n", pm.Title))
	sb.WriteString(fmt.Sprintf("**严重等级**: %s\n", pm.Severity))
	sb.WriteString(fmt.Sprintf("**持续时间**: %s\n\n", pm.Duration))

	sb.WriteString("## 摘要\n\n")
	sb.WriteString(pm.Summary)
	sb.WriteString("\n")

	if len(pm.Timeline) > 0 {
		sb.WriteString("\n## 时间线\n\n")
		sb.WriteString("| 时间 | 事件 | Agent |\n")
		sb.WriteString("|------|------|-------|\n")
		for _, t := range pm.Timeline {
			agent := t.Agent
			if agent == "" {
				agent = "-"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", t.Time, t.Event, agent))
		}
	}

	if pm.RootCause != "" {
		sb.WriteString(fmt.Sprintf("\n## 根因\n\n%s\n", pm.RootCause))
	}

	if pm.Impact != "" {
		sb.WriteString(fmt.Sprintf("\n## 影响\n\n%s\n", pm.Impact))
	}

	if len(pm.ActionItems) > 0 {
		sb.WriteString("\n## Action Items\n\n")
		sb.WriteString("| 负责人 | 动作 | 优先级 |\n")
		sb.WriteString("|--------|------|--------|\n")
		for _, ai := range pm.ActionItems {
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", ai.Assignee, ai.Action, ai.Priority))
		}
	}

	if len(pm.LessonsLearned) > 0 {
		sb.WriteString("\n## 经验教训\n\n")
		for _, l := range pm.LessonsLearned {
			sb.WriteString(fmt.Sprintf("- %s\n", l))
		}
	}

	return sb.String()
}
