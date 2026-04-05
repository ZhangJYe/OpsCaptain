package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
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

type Agent struct{}

func New() *Agent {
	return &Agent{}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return []string{"prometheus-alerts"}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	tool := newPrometheusAlertsQueryTool()
	queryCtx, cancel := context.WithTimeout(ctx, metricsQueryTimeout(ctx))
	defer cancel()

	output, err := tool.InvokableRun(queryCtx, "{}")
	if err != nil {
		summary := fmt.Sprintf("查询 Prometheus 告警失败: %v", err)
		if queryCtx.Err() == context.DeadlineExceeded {
			summary = "查询 Prometheus 告警超时，已跳过该步骤。"
		}
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusDegraded,
			Summary:    summary,
			Confidence: 0.25,
			Metadata: map[string]any{
				"error": err.Error(),
			},
		}, nil
	}

	var parsed tools.PrometheusAlertsOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusDegraded,
			Summary:    "告警查询已执行，但结果解析失败，已返回原始输出。",
			Confidence: 0.35,
			Metadata: map[string]any{
				"raw_output": output,
			},
		}, nil
	}

	if !parsed.Success {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusDegraded,
			Summary:    fallbackText(parsed.Message, "Prometheus 告警查询失败，已跳过该步骤。"),
			Confidence: 0.35,
			Metadata: map[string]any{
				"error": parsed.Error,
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

	summary := "当前没有发现活跃告警。"
	if len(parsed.Alerts) > 0 {
		summary = fmt.Sprintf("发现 %d 条活跃告警：%s。", len(parsed.Alerts), strings.Join(alertNames, "、"))
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    summary,
		Confidence: 0.88,
		Evidence:   evidence,
		Metadata: map[string]any{
			"alerts": parsed.Alerts,
		},
	}, nil
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
