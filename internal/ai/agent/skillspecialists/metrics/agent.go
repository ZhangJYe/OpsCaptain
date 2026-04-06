package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"

	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "metrics"

const defaultMetricsQueryTimeout = 5 * time.Second

var (
	newPrometheusAlertsQueryTool = tools.NewPrometheusAlertsQueryTool
	metricsQueryTimeout          = func(ctx context.Context) time.Duration {
		v, err := g.Cfg().Get(ctx, "multi_agent.metrics_query_timeout_ms")
		if err == nil && v.Int64() > 0 {
			return time.Duration(v.Int64()) * time.Millisecond
		}
		return defaultMetricsQueryTimeout
	}
)

type Agent struct {
	registry *skills.Registry
}

type metricsSkill struct {
	name        string
	description string
	mode        string
	keywords    []string
}

func New() *Agent {
	return &Agent{registry: mustNewSkillRegistry()}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return skills.PrefixedCapabilities([]string{"prometheus-alerts"}, a.registry.SkillNames())
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	execution, err := a.registry.Execute(ctx, task)
	if err != nil {
		return nil, err
	}
	if rt, ok := runtime.FromContext(ctx); ok {
		rt.EmitInfo(ctx, task, a.Name(), fmt.Sprintf("selected skill=%s", execution.Skill.Name()), map[string]any{
			"skill_name":        execution.Skill.Name(),
			"skill_description": execution.Skill.Description(),
		})
	}
	return execution.Result, nil
}

func (s *metricsSkill) Name() string {
	return s.name
}

func (s *metricsSkill) Description() string {
	return s.description
}

func (s *metricsSkill) Match(task *protocol.TaskEnvelope) bool {
	if task == nil || len(s.keywords) == 0 {
		return len(s.keywords) == 0
	}
	return skills.ContainsAny(task.Goal, s.keywords...)
}

func (s *metricsSkill) Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return runPrometheusAlertQuery(ctx, task, s.mode)
}

func mustNewSkillRegistry() *skills.Registry {
	registry, err := skills.NewRegistry(
		AgentName,
		&metricsSkill{
			name:        "metrics_alert_triage",
			description: "Investigate active Prometheus alerts for explicit alert and severity questions.",
			mode:        "alert_triage",
			keywords: []string{
				"alert", "alerts", "prometheus", "firing", "severity",
				"告警", "报警", "prom", "alertmanager",
			},
		},
		&metricsSkill{
			name:        "metrics_incident_snapshot",
			description: "Fallback Prometheus snapshot for broader incident health checks.",
			mode:        "incident_snapshot",
		},
	)
	if err != nil {
		panic(fmt.Sprintf("failed to build metrics skills registry: %v", err))
	}
	return registry
}

func runPrometheusAlertQuery(ctx context.Context, task *protocol.TaskEnvelope, mode string) (*protocol.TaskResult, error) {
	tool := newPrometheusAlertsQueryTool()
	queryCtx, cancel := context.WithTimeout(ctx, metricsQueryTimeout(ctx))
	defer cancel()

	output, err := tool.InvokableRun(queryCtx, "{}")
	if err != nil {
		summary := fmt.Sprintf("prometheus alert query failed: %v", err)
		if queryCtx.Err() == context.DeadlineExceeded {
			summary = "prometheus alert query timed out; skipped"
		}
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    summary,
			Confidence: 0.25,
			Metadata: map[string]any{
				"error":        err.Error(),
				"metrics_mode": mode,
			},
		}, nil
	}

	var parsed tools.PrometheusAlertsOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    "prometheus alert query returned an unreadable payload",
			Confidence: 0.35,
			Metadata: map[string]any{
				"raw_output":    output,
				"metrics_mode":  mode,
				"decode_failed": true,
			},
		}, nil
	}

	if !parsed.Success {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    fallbackText(parsed.Message, "prometheus alert query failed"),
			Confidence: 0.35,
			Metadata: map[string]any{
				"error":        parsed.Error,
				"metrics_mode": mode,
			},
		}, nil
	}

	evidence := make([]protocol.EvidenceItem, 0, len(parsed.Alerts))
	alertNames := make([]string, 0, len(parsed.Alerts))
	for _, alert := range parsed.Alerts {
		alertNames = append(alertNames, alert.AlertName)
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "prometheus",
			SourceID:   alert.AlertName,
			Title:      alert.AlertName,
			Snippet:    strings.TrimSpace(alert.Description),
			Score:      0.82,
		})
	}

	summary := "no active alerts found in Prometheus"
	if len(parsed.Alerts) > 0 {
		prefix := "found"
		if mode == "incident_snapshot" {
			prefix = "prometheus snapshot found"
		}
		summary = fmt.Sprintf("%s %d active alerts: %s", prefix, len(parsed.Alerts), strings.Join(alertNames, ", "))
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      AgentName,
		Status:     protocol.ResultStatusSucceeded,
		Summary:    summary,
		Confidence: 0.88,
		Evidence:   evidence,
		Metadata: map[string]any{
			"alerts":       parsed.Alerts,
			"metrics_mode": mode,
		},
	}, nil
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
