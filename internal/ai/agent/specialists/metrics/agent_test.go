package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"SuperBizAgent/internal/ai/protocol"

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

func TestMetricsAgentDegradesOnTimeout(t *testing.T) {
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
	task := protocol.NewRootTask("session-test", "请分析当前告警", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
	if result.Summary != "查询 Prometheus 告警超时，已跳过该步骤。" {
		t.Fatalf("unexpected summary: %q", result.Summary)
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
	task := protocol.NewRootTask("session-test", "请分析当前告警", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Status != protocol.ResultStatusDegraded {
		t.Fatalf("expected degraded status, got %s", result.Status)
	}
}
