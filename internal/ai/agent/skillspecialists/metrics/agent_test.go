package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/tools"

	toolapi "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeMetricsTool struct {
	output string
	err    error
	block  bool
}

func (f *fakeMetricsTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "query_prometheus_alerts"}, nil
}

func (f *fakeMetricsTool) InvokableRun(ctx context.Context, input string, opts ...toolapi.Option) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if f.err != nil {
		return "", f.err
	}
	return f.output, nil
}

func TestMetricsAgentUsesAlertTriageSkill(t *testing.T) {
	oldFactory := newPrometheusAlertsQueryTool
	oldTimeout := metricsQueryTimeout
	defer func() {
		newPrometheusAlertsQueryTool = oldFactory
		metricsQueryTimeout = oldTimeout
	}()

	newPrometheusAlertsQueryTool = func() toolapi.InvokableTool {
		return &fakeMetricsTool{block: true}
	}
	metricsQueryTimeout = func(context.Context) time.Duration {
		return 20 * time.Millisecond
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "请分析当前 Prometheus 告警", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Summary != "prometheus alert query timed out; skipped" {
		t.Fatalf("unexpected summary: %q", result.Summary)
	}
	if result.Metadata["skill_name"] != "metrics_alert_triage" {
		t.Fatalf("expected metrics_alert_triage, got %#v", result.Metadata)
	}
}

func TestMetricsAgentFallsBackToSnapshotSkill(t *testing.T) {
	oldFactory := newPrometheusAlertsQueryTool
	defer func() {
		newPrometheusAlertsQueryTool = oldFactory
	}()

	payload, err := json.Marshal(tools.PrometheusAlertsOutput{
		Success: true,
		Alerts: []tools.SimplifiedAlert{
			{AlertName: "HighLatency", Description: "latency > 1s"},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	newPrometheusAlertsQueryTool = func() toolapi.InvokableTool {
		return &fakeMetricsTool{output: string(payload)}
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "系统现在健康吗", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %s", result.Status)
	}
	if result.Metadata["skill_name"] != "metrics_incident_snapshot" {
		t.Fatalf("expected metrics_incident_snapshot, got %#v", result.Metadata)
	}
}

func TestMetricsAgentDegradesOnInvocationError(t *testing.T) {
	oldFactory := newPrometheusAlertsQueryTool
	defer func() {
		newPrometheusAlertsQueryTool = oldFactory
	}()

	newPrometheusAlertsQueryTool = func() toolapi.InvokableTool {
		return &fakeMetricsTool{err: errors.New("prometheus unavailable")}
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "请分析当前 Prometheus 告警", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
}
