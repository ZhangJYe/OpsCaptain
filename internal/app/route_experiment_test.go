package app

import (
	"context"
	"testing"
	"time"
)

func TestRouteExperimentStableServerAssignmentAndExclusions(t *testing.T) {
	cfg := RouteExperimentConfig{Enabled: true, ExperimentID: "route", Version: "v1", Salt: "server-secret", RolloutStage: RouteExperimentHalf, RolloutPercent: 50, HighRiskExcluded: true, OODExcluded: true}
	first := AssignRouteExperiment(cfg, "server-session-1", AgentRouteRiskLow, false)
	second := AssignRouteExperiment(cfg, "server-session-1", AgentRouteRiskLow, false)
	if first.Variant != second.Variant || first.AssignmentHash != second.AssignmentHash || first.AssignmentHash == "server-session-1" {
		t.Fatalf("assignment is not stable and anonymous: %#v %#v", first, second)
	}
	for _, excluded := range []RouteExperimentAssignment{AssignRouteExperiment(cfg, "risk", AgentRouteRiskHigh, false), AssignRouteExperiment(cfg, "ood", AgentRouteRiskLow, true)} {
		if !excluded.Excluded || excluded.ServedVariant != RouteExperimentA {
			t.Fatalf("unsafe traffic entered B: %#v", excluded)
		}
	}
}

func TestRouteShadowTimeoutNeverChangesServedVariant(t *testing.T) {
	cfg := RouteExperimentConfig{Enabled: true, RolloutStage: RouteExperimentShadow, ShadowEnabled: true, ShadowTimeout: time.Millisecond}
	assignment := AssignRouteExperiment(cfg, "session", AgentRouteRiskLow, false)
	result := RunRouteShadow(context.Background(), cfg, assignment, func(ctx context.Context) (*AgentRouteResult, error) { <-ctx.Done(); return nil, ctx.Err() })
	if !result.TimedOut || result.Assignment.ServedVariant != RouteExperimentA || result.BResult != nil {
		t.Fatalf("shadow affected service result: %#v", result)
	}
}

func TestRouteGuardrailStopsBAndFeedbackDeduplicates(t *testing.T) {
	guard := NewRouteGuardrail(RouteExperimentConfig{MaxErrorRate: 0.05, MaxTimeoutRate: 0.02, MaxP95Latency: time.Second})
	if !guard.Observe(RouteGuardrailObservation{HighRiskFalseRoutes: 1}) {
		t.Fatal("high-risk violation must stop B")
	}
	stopped, reason := guard.Stopped()
	if !stopped || reason != "high_risk_false_route" {
		t.Fatalf("unexpected stop state: %v %q", stopped, reason)
	}
	deduper := NewRouteFeedbackDeduper()
	event := RouteFeedbackEvent{RequestID: "r1", Kind: "route_override"}
	if !deduper.Accept(event) || deduper.Accept(event) {
		t.Fatal("feedback deduplication failed")
	}
}
