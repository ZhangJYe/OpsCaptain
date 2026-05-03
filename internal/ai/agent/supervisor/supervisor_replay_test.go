package supervisor

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

type replaySpecialist struct {
	name string
}

func (a *replaySpecialist) Name() string {
	return a.name
}

func (a *replaySpecialist) Capabilities() []string {
	return []string{"replay-test"}
}

func (a *replaySpecialist) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.name,
		Status:     protocol.ResultStatusSucceeded,
		Summary:    fmt.Sprintf("%s domain handled query: %s", a.name, task.Goal),
		Confidence: 0.8,
		Evidence: []protocol.EvidenceItem{
			{
				SourceType: a.name,
				SourceID:   a.name + "-1",
				Title:      a.name,
				Snippet:    "stub evidence",
				Score:      0.8,
			},
		},
	}, nil
}

type configurableSpecialist struct {
	name      string
	status    protocol.ResultStatus
	summary   string
	evidence  []protocol.EvidenceItem
	degradeRsn string
}

func (a *configurableSpecialist) Name() string {
	return a.name
}

func (a *configurableSpecialist) Capabilities() []string {
	return []string{"replay-test"}
}

func (a *configurableSpecialist) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	result := &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.name,
		Status:     a.status,
		Summary:    a.summary,
		Confidence: 0.8,
		Evidence:   a.evidence,
	}
	if a.status == protocol.ResultStatusDegraded {
		result.DegradationReason = a.degradeRsn
		result.Confidence = 0.3
	}
	return result, nil
}

func TestSupervisorReplayCases(t *testing.T) {
	cases := []struct {
		name            string
		query           string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:         "alert-analysis-fanout",
			query:        "请分析当前 Prometheus 告警并结合日志排查",
			wantContains: []string{"### 指标", "### 日志", "### 知识库"},
		},
		{
			name:            "knowledge-only",
			query:           "请查询知识库中的 SOP 文档",
			wantContains:    []string{"### 知识库"},
			wantNotContains: []string{"### 指标", "### 日志"},
		},
		{
			name:            "incident-analysis",
			query:           "请排查支付服务日志错误",
			wantContains:    []string{"### 日志", "### 知识库"},
			wantNotContains: []string{"### 指标"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := runtime.New()
			for _, agent := range []runtime.Agent{
				New(),
				triage.New(),
				reporter.New(),
				&replaySpecialist{name: metrics.AgentName},
				&replaySpecialist{name: logs.AgentName},
				&replaySpecialist{name: knowledge.AgentName},
			} {
				if err := rt.Register(agent); err != nil {
					t.Fatalf("register %s: %v", agent.Name(), err)
				}
			}

			task := protocol.NewRootTask("session-replay", tc.query, AgentName)
			result, err := rt.Dispatch(context.Background(), task)
			if err != nil {
				t.Fatalf("dispatch: %v", err)
			}
			if result == nil || strings.TrimSpace(result.Summary) == "" {
				t.Fatalf("expected non-empty result summary, got %#v", result)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(result.Summary, want) {
					t.Fatalf("expected summary to contain %q, got:\n%s", want, result.Summary)
				}
			}
			for _, unwanted := range tc.wantNotContains {
				if strings.Contains(result.Summary, unwanted) {
					t.Fatalf("expected summary not to contain %q, got:\n%s", unwanted, result.Summary)
				}
			}
		})
	}
}

func TestSupervisorReplayDegradedSpecialist(t *testing.T) {
	rt := runtime.New()
	for _, agent := range []runtime.Agent{
		New(),
		triage.New(),
		reporter.New(),
		&configurableSpecialist{
			name:    metrics.AgentName,
			status:  protocol.ResultStatusSucceeded,
			summary: "no active alerts",
			evidence: []protocol.EvidenceItem{
				{SourceType: "prometheus", Title: "Metrics OK", Snippet: "no alerts"},
			},
		},
		&configurableSpecialist{
			name:       logs.AgentName,
			status:     protocol.ResultStatusDegraded,
			summary:    "log tool unavailable",
			degradeRsn: "MCP connection refused",
		},
		&configurableSpecialist{
			name:    knowledge.AgentName,
			status:  protocol.ResultStatusSucceeded,
			summary: "found 2 SOP documents",
			evidence: []protocol.EvidenceItem{
				{SourceType: "knowledge_base", Title: "SOP-001", Snippet: "check logs first"},
			},
		},
	} {
		if err := rt.Register(agent); err != nil {
			t.Fatalf("register %s: %v", agent.Name(), err)
		}
	}

	task := protocol.NewRootTask("session-degraded", "payment service 日志报错排查", AgentName)
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil || strings.TrimSpace(result.Summary) == "" {
		t.Fatalf("expected non-empty result summary, got %#v", result)
	}
	if !strings.Contains(result.Summary, "### 指标") && !strings.Contains(result.Summary, "### 日志") {
		t.Fatalf("expected metrics or logs section in summary, got:\n%s", result.Summary)
	}
	if !strings.Contains(result.Summary, "### 知识库") {
		t.Fatalf("expected knowledge section in summary, got:\n%s", result.Summary)
	}
}

func TestSupervisorReplayEmptyKnowledge(t *testing.T) {
	rt := runtime.New()
	for _, agent := range []runtime.Agent{
		New(),
		triage.New(),
		reporter.New(),
		&configurableSpecialist{
			name:    metrics.AgentName,
			status:  protocol.ResultStatusSucceeded,
			summary: "CPU normal",
			evidence: []protocol.EvidenceItem{
				{SourceType: "prometheus", Title: "CPU OK", Snippet: "50%"},
			},
		},
		&configurableSpecialist{
			name:    logs.AgentName,
			status:  protocol.ResultStatusSucceeded,
			summary: "no errors found",
			evidence: []protocol.EvidenceItem{
				{SourceType: "mcp_log", Title: "Log Search", Snippet: "0 errors"},
			},
		},
		&configurableSpecialist{
			name:       knowledge.AgentName,
			status:     protocol.ResultStatusDegraded,
			summary:    "no matching documents",
			degradeRsn: "retriever returned 0 results for query",
		},
	} {
		if err := rt.Register(agent); err != nil {
			t.Fatalf("register %s: %v", agent.Name(), err)
		}
	}

	task := protocol.NewRootTask("session-empty-kb", "unknown_error_code_999 排查", AgentName)
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil || strings.TrimSpace(result.Summary) == "" {
		t.Fatalf("expected non-empty result summary, got %#v", result)
	}
}
