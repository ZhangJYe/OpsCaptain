package evaladapter

import (
	"context"
	"encoding/json"
	"testing"

	"SuperBizAgent/internal/ai/evalharness"
)

func TestRouteAdapterUsesOriginalQueryAndReportsMetrics(t *testing.T) {
	payload := RoutePayload{ExpectedDecision: "incident", ExpectedStrategy: "plan_execute_replan", ModelOutput: `{"intent":"incident","recommended_strategy":"plan_execute_replan","confidence":0.91,"reason":"incident"}`}
	raw, _ := json.Marshal(payload)
	evalCase := evalharness.CaseEnvelope{SchemaVersion: evalharness.CaseSchemaVersion, ID: "route-1", Suite: evalharness.SuiteRoute, Input: evalharness.CaseInput{Query: "payment P95 elevated", Memory: []string{"this is casual chat"}}, Expectation: json.RawMessage(`{"decision":"incident"}`), PayloadSchemaVersion: RoutePayloadSchema, Payload: raw}
	adapter := NewRouteAdapter()
	result := adapter.RunCase(context.Background(), evalCase)
	if !result.Matched || result.Status != evalharness.StatusSucceeded {
		t.Fatalf("unexpected route result: %#v", result)
	}
	var domain routeCaseDomain
	if err := json.Unmarshal(result.Domain, &domain); err != nil {
		t.Fatal(err)
	}
	if domain.RoutingInputHash != evalharness.QueryHash(evalCase.Input.Query) || !domain.MemoryPresent || domain.Source != "model" {
		t.Fatalf("route did not preserve raw query: %#v", domain)
	}
	_, metricsRaw, _, err := adapter.Aggregate([]evalharness.CaseResult{result})
	if err != nil {
		t.Fatal(err)
	}
	metrics := evalharness.DomainMetricMap(metricsRaw)
	if metrics["macro_f1"].Value <= 0 {
		t.Fatalf("unexpected route metrics: %s", metricsRaw)
	}
}

func TestRouteAdapterKeywordSource(t *testing.T) {
	payload := RoutePayload{ExpectedDecision: "incident", ExpectedStrategy: "plan_execute_replan", HighConfidenceKeywords: []string{"p95 elevated"}, Repeat: 2}
	raw, _ := json.Marshal(payload)
	evalCase := evalharness.CaseEnvelope{SchemaVersion: evalharness.CaseSchemaVersion, ID: "route-keyword", Suite: evalharness.SuiteRoute, Input: evalharness.CaseInput{Query: "payment p95 elevated"}, Expectation: json.RawMessage(`{"decision":"incident"}`), PayloadSchemaVersion: RoutePayloadSchema, Payload: raw}
	result := NewRouteAdapter().RunCase(context.Background(), evalCase)
	var domain routeCaseDomain
	_ = json.Unmarshal(result.Domain, &domain)
	if !result.Matched || domain.Source != "cache" {
		t.Fatalf("expected cached keyword route: %#v", domain)
	}
}

func TestRouteAdapterReportsLowConfidenceFallback(t *testing.T) {
	payload := RoutePayload{ExpectedDecision: "confirm", ConfidenceThreshold: 0.75, ModelOutput: `{"intent":"incident","recommended_strategy":"plan_execute_replan","confidence":0.4,"reason":"uncertain"}`}
	raw, _ := json.Marshal(payload)
	evalCase := evalharness.CaseEnvelope{SchemaVersion: evalharness.CaseSchemaVersion, ID: "route-fallback", Suite: evalharness.SuiteRoute, Input: evalharness.CaseInput{Query: "不确定是否故障"}, Expectation: json.RawMessage(`{"decision":"confirm"}`), PayloadSchemaVersion: RoutePayloadSchema, Payload: raw}
	result := NewRouteAdapter().RunCase(context.Background(), evalCase)
	var domain routeCaseDomain
	_ = json.Unmarshal(result.Domain, &domain)
	if !result.Matched || result.Status != evalharness.StatusSucceeded || domain.Source != "fallback" || domain.Confidence != 0.4 || !domain.Degraded {
		t.Fatalf("expected low-confidence fallback: %#v", result)
	}
	_, metricsRaw, _, err := NewRouteAdapter().Aggregate([]evalharness.CaseResult{result})
	if err != nil {
		t.Fatal(err)
	}
	metrics := evalharness.DomainMetricMap(metricsRaw)
	if metrics["low_confidence_rate"].Value != 1 || metrics["fallback_rate"].Value != 1 {
		t.Fatalf("unexpected fallback metrics: %s", metricsRaw)
	}
}

func TestRouteAdapterV2ReportsFunnelMetrics(t *testing.T) {
	needClarification := false
	payload := RoutePayload{AcceptableIntents: []string{"incident_diagnosis"}, ExpectedPublicRoute: "incident", NeedClarification: &needClarification, RiskLevel: "low", GroupID: "group-1", ModelOutput: `{"candidates":[{"intent":"incident_diagnosis","confidence":0.9,"reason_codes":["symptom_present"]},{"intent":"knowledge_qa","confidence":0.05}],"entities":{"service":"payment-api"},"required_slots":[],"risk_hint":"low","recommended_strategy":"plan_execute_replan"}`}
	raw, _ := json.Marshal(payload)
	evalCase := evalharness.CaseEnvelope{SchemaVersion: evalharness.CaseSchemaVersion, ID: "route-v2", Suite: evalharness.SuiteRoute, Input: evalharness.CaseInput{Query: "payment-api P95 升高"}, PayloadSchemaVersion: RoutePayloadSchemaV2, Payload: raw}
	adapter := NewRouteAdapter()
	result := adapter.RunCase(context.Background(), evalCase)
	if !result.Matched || result.Usage.LLMCalls != 1 {
		t.Fatalf("unexpected v2 result: %#v", result)
	}
	schema, metricsRaw, _, err := adapter.Aggregate([]evalharness.CaseResult{result})
	if err != nil || schema != "route-metrics/v2" {
		t.Fatalf("unexpected metrics: %s err=%v", schema, err)
	}
	metrics := evalharness.DomainMetricMap(metricsRaw)
	if metrics["top2_recall"].Value != 1 || metrics["high_risk_false_routing"].Value != 0 {
		t.Fatalf("unexpected v2 metrics: %s", metricsRaw)
	}
}

func TestCompareRouteVariantsUsesSameCases(t *testing.T) {
	payload := RoutePayload{AcceptableIntents: []string{"knowledge_qa"}, ExpectedPublicRoute: "chat", ModelOutput: `{"candidates":[{"intent":"knowledge_qa","confidence":0.9}],"risk_hint":"low"}`}
	raw, _ := json.Marshal(payload)
	cases := []evalharness.CaseEnvelope{{SchemaVersion: evalharness.CaseSchemaVersion, ID: "variant-1", Suite: evalharness.SuiteRoute, Input: evalharness.CaseInput{Query: "什么是 Redis"}, PayloadSchemaVersion: RoutePayloadSchemaV2, Payload: raw}}
	reports, err := CompareRouteVariants(context.Background(), cases, []string{"A", "B-full", "B-no-context", "B-no-fast-path"})
	if err != nil || len(reports) != 4 {
		t.Fatalf("unexpected variant reports: %#v err=%v", reports, err)
	}
	for variant, report := range reports {
		if report.Cases != 1 || report.Schema != "route-metrics/v2" {
			t.Fatalf("variant %s report invalid: %#v", variant, report)
		}
	}
}
