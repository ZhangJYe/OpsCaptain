package triage

import (
	"context"
	"errors"
	"testing"

	agentcontracts "SuperBizAgent/internal/ai/agent/contracts"
	"SuperBizAgent/internal/ai/protocol"
)

func TestTriageAlertAnalysis(t *testing.T) {
	agent := New()
	task := protocol.NewRootTask("session-test", "请分析当前 Prometheus 告警并结合日志排查", agent.Name())

	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	intent, _ := result.Metadata["intent"].(string)
	if intent != "alert_analysis" {
		t.Fatalf("expected alert_analysis intent, got %q", intent)
	}

	domains, _ := result.Metadata["domains"].([]string)
	if len(domains) != 3 {
		t.Fatalf("expected 3 routed domains, got %v", domains)
	}
	if result.Metadata["agent_contract_id"] != "triage:"+agentcontracts.Version {
		t.Fatalf("expected triage contract metadata, got %#v", result.Metadata)
	}
}

func TestHybridTriageUsesRuleFastPath(t *testing.T) {
	oldMode := triageMode
	oldClassifier := classifyTriageWithLLM
	defer func() {
		triageMode = oldMode
		classifyTriageWithLLM = oldClassifier
	}()

	triageMode = func(context.Context) string { return "hybrid" }
	classifyTriageWithLLM = func(context.Context, string) (decision, error) {
		t.Fatal("hybrid triage should not call LLM when rule matches")
		return decision{}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "Prometheus 告警持续 firing", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["triage_source"] != "rule" {
		t.Fatalf("expected rule source, got %#v", result.Metadata)
	}
	if result.Metadata["triage_fallback"] != false {
		t.Fatalf("expected no fallback, got %#v", result.Metadata)
	}
}

func TestTriageExpandedRuleCoverage(t *testing.T) {
	cases := []struct {
		name              string
		query             string
		wantIntent        string
		wantDomains       []string
		wantUseMultiAgent bool
	}{
		{
			name:              "hpa knowledge",
			query:             "什么是 Kubernetes HPA？怎么配置？",
			wantIntent:        "kb_qa",
			wantDomains:       []string{"knowledge"},
			wantUseMultiAgent: true,
		},
		{
			name:              "5xx incident",
			query:             "orderservice 5xx 错误率从 1% 升到 12%",
			wantIntent:        "incident_analysis",
			wantDomains:       []string{"metrics", "logs", "knowledge"},
			wantUseMultiAgent: true,
		},
		{
			name:              "mysql connection alert",
			query:             "MySQL 连接数突然飙升到 500",
			wantIntent:        "alert_analysis",
			wantDomains:       []string{"metrics", "logs", "knowledge"},
			wantUseMultiAgent: true,
		},
		{
			name:              "generic greeting",
			query:             "你好，今天天气怎么样",
			wantIntent:        "kb_qa",
			wantDomains:       nil,
			wantUseMultiAgent: false,
		},
	}

	agent := New()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := protocol.NewRootTask("session-test", tc.query, agent.Name())
			result, err := agent.Handle(context.Background(), task)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Metadata["intent"] != tc.wantIntent {
				t.Fatalf("expected intent %q, got %#v", tc.wantIntent, result.Metadata)
			}
			if result.Metadata["use_multi_agent"] != tc.wantUseMultiAgent {
				t.Fatalf("expected use_multi_agent=%t, got %#v", tc.wantUseMultiAgent, result.Metadata)
			}
			domains, _ := result.Metadata["domains"].([]string)
			if len(domains) != len(tc.wantDomains) {
				t.Fatalf("expected domains %#v, got %#v", tc.wantDomains, domains)
			}
			for i := range tc.wantDomains {
				if domains[i] != tc.wantDomains[i] {
					t.Fatalf("expected domains %#v, got %#v", tc.wantDomains, domains)
				}
			}
			if result.Metadata["triage_fallback"] != false {
				t.Fatalf("expected rule hit without fallback, got %#v", result.Metadata)
			}
		})
	}
}

func TestHybridTriageUsesLLMOnRuleMiss(t *testing.T) {
	oldMode := triageMode
	oldClassifier := classifyTriageWithLLM
	defer func() {
		triageMode = oldMode
		classifyTriageWithLLM = oldClassifier
	}()

	triageMode = func(context.Context) string { return "hybrid" }
	classifyTriageWithLLM = func(context.Context, string) (decision, error) {
		return decision{
			intent:        "kb_qa",
			domains:       []string{"knowledge"},
			priority:      "medium",
			useMultiAgent: true,
			summary:       "LLM classified as kb_qa",
			source:        "llm",
			confidence:    0.82,
		}, nil
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "服务最近有点不稳定", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["intent"] != "kb_qa" {
		t.Fatalf("expected kb_qa, got %#v", result.Metadata)
	}
	if result.Metadata["triage_source"] != "llm" || result.Metadata["triage_fallback"] != false {
		t.Fatalf("expected llm source without fallback, got %#v", result.Metadata)
	}
	domains, _ := result.Metadata["domains"].([]string)
	if len(domains) != 1 || domains[0] != "knowledge" {
		t.Fatalf("expected knowledge domain, got %#v", domains)
	}
}

func TestHybridTriageFallsBackWhenLLMFails(t *testing.T) {
	oldMode := triageMode
	oldClassifier := classifyTriageWithLLM
	defer func() {
		triageMode = oldMode
		classifyTriageWithLLM = oldClassifier
	}()

	triageMode = func(context.Context) string { return "hybrid" }
	classifyTriageWithLLM = func(context.Context, string) (decision, error) {
		return decision{}, errors.New("timeout")
	}

	agent := New()
	task := protocol.NewRootTask("session-test", "服务最近有点不稳定", agent.Name())
	result, err := agent.Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Metadata["triage_source"] != "fallback" || result.Metadata["triage_fallback"] != true {
		t.Fatalf("expected fallback metadata, got %#v", result.Metadata)
	}
	if result.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded fallback result, got %s", result.Status)
	}
}

func TestParseLLMDecisionNormalizesOutput(t *testing.T) {
	route, err := parseLLMDecision(`{"intent":"incident_analysis","domains":["logs","knowledge","logs"],"priority":"high","use_multi_agent":true}`)
	if err != nil {
		t.Fatalf("parse llm decision: %v", err)
	}
	if route.intent != "incident_analysis" || route.priority != "high" || !route.useMultiAgent {
		t.Fatalf("unexpected route: %#v", route)
	}
	if len(route.domains) != 2 || route.domains[0] != "logs" || route.domains[1] != "knowledge" {
		t.Fatalf("expected deduped domains, got %#v", route.domains)
	}
}
