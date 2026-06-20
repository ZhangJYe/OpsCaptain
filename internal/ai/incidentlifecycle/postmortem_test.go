package incidentlifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssembleTimeline_Empty(t *testing.T) {
	result := AssembleTimeline(nil)
	assert.Nil(t, result)
}

func TestAssembleTimeline_Sorted(t *testing.T) {
	events := []RawEvent{
		{Timestamp: 3000, Type: "turn_started", Message: "第三步"},
		{Timestamp: 1000, Type: "turn_started", Message: "第一步"},
		{Timestamp: 2000, Type: "turn_started", Message: "第二步"},
	}

	result := AssembleTimeline(events)

	require.Len(t, result, 3)
	assert.Equal(t, "第一步", result[0].Event)
	assert.Equal(t, "第二步", result[1].Event)
	assert.Equal(t, "第三步", result[2].Event)
}

func TestAssembleTimeline_WithAgent(t *testing.T) {
	events := []RawEvent{
		{Timestamp: 1000, Type: "aiops_analysis", Message: "分析完成", Agent: "plan_execute_replan"},
	}

	result := AssembleTimeline(events)

	require.Len(t, result, 1)
	assert.Equal(t, "plan_execute_replan", result[0].Agent)
}

func TestGeneratePostmortem(t *testing.T) {
	lc := IncidentLifecycle{
		Severity:         SeverityP1,
		Status:           LifecycleResolved,
		AffectedServices: []string{"paymentservice", "userservice"},
		DetectedAt:       1000,
		MitigatedAt:      3600000,
		ResolvedAt:       3600000,
		MTTR:             3600000,
	}

	timeline := []TimelineEntry{
		{Time: "10:00:00", Event: "告警触发"},
		{Time: "10:05:00", Event: "开始排查"},
		{Time: "11:00:00", Event: "故障恢复"},
	}

	pm := GeneratePostmortem(
		"paymentservice 故障",
		lc,
		timeline,
		[]string{"paymentservice", "userservice"},
		"支付功能中断 1 小时",
	)

	require.NotNil(t, pm)
	assert.Equal(t, "paymentservice 故障", pm.Title)
	assert.Equal(t, "P1", pm.Severity)
	assert.Equal(t, "1h0m0s", pm.Duration)
	assert.Len(t, pm.Timeline, 3)
	assert.NotEmpty(t, pm.ActionItems)
	assert.NotEmpty(t, pm.LessonsLearned)
}

func TestFormatPostmortemMarkdown(t *testing.T) {
	pm := &Postmortem{
		Title:    "Test Incident",
		Severity: "P2",
		Duration: "30m",
		Summary:  "测试摘要",
		Timeline: []TimelineEntry{
			{Time: "10:00:00", Event: "告警"},
		},
		ActionItems: []ActionItem{
			{Assignee: "SRE", Action: "补监控", Priority: "high"},
		},
		LessonsLearned: []string{"教训1"},
	}

	md := FormatPostmortemMarkdown(pm)

	assert.Contains(t, md, "# Postmortem: Test Incident")
	assert.Contains(t, md, "**严重等级**: P2")
	assert.Contains(t, md, "## 时间线")
	assert.Contains(t, md, "告警")
	assert.Contains(t, md, "## Action Items")
	assert.Contains(t, md, "补监控")
	assert.Contains(t, md, "## 经验教训")
	assert.Contains(t, md, "教训1")
}

func TestGeneratePostmortem_ZeroDuration(t *testing.T) {
	lc := IncidentLifecycle{
		Severity: SeverityP3,
	}

	pm := GeneratePostmortem("test", lc, nil, nil, "")

	assert.Equal(t, "-", pm.Duration)
}
