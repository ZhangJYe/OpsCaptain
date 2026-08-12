package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

func testAgentRouter(output string, cfg AgentRouterConfig) *AgentRouterApp {
	router := NewAgentRouterApp()
	router.SetConfigLoader(func(context.Context) AgentRouterConfig { return cfg })
	router.SetGenerate(func(context.Context, string) (string, error) { return output, nil })
	return router
}

func testAgentRouterConfig() AgentRouterConfig {
	return AgentRouterConfig{
		Enabled:                  true,
		Timeout:                  time.Second,
		ConfidenceThreshold:      0.75,
		DefaultDiagnosisStrategy: AgentDiagnosisStrategyPlan,
		AllowedDiagnosisStrategies: map[AgentDiagnosisStrategy]struct{}{
			AgentDiagnosisStrategyPlan: {},
			AgentDiagnosisStrategyGoS:  {},
		},
	}
}

func TestAgentRouterAutoRoutesChat(t *testing.T) {
	router := testAgentRouter(`{"intent":"chat","recommended_strategy":"plan_execute_replan","confidence":0.91,"reason":"概念问答"}`, testAgentRouterConfig())
	result, err := router.Decide(context.Background(), &AgentRouteInput{Query: "什么是 Redis？", RouteMode: AgentRouteModeAuto})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if result.Decision != AgentRouteDecisionChat || result.Strategy != "" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRouterAutoUsesSelectedPlan(t *testing.T) {
	router := testAgentRouter(`{"intent":"incident","recommended_strategy":"gos_engine","confidence":0.91,"reason":"存在告警"}`, testAgentRouterConfig())
	result, err := router.Decide(context.Background(), &AgentRouteInput{
		Query: "paymentservice P95 持续升高", RouteMode: AgentRouteModeAuto, DiagnosisStrategy: AgentDiagnosisStrategyPlan,
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if result.Decision != AgentRouteDecisionIncident || result.Strategy != AgentDiagnosisStrategyPlan {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRouterDiagnosisAutoUsesFlashGoS(t *testing.T) {
	router := testAgentRouter(`{"intent":"incident","recommended_strategy":"gos_engine","confidence":0.91,"reason":"多候选根因"}`, testAgentRouterConfig())
	result, err := router.Decide(context.Background(), &AgentRouteInput{
		Query: "接口超时并伴随多项异常", RouteMode: AgentRouteModeDiagnosis, DiagnosisStrategy: AgentDiagnosisStrategyAuto,
	})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if result.Decision != AgentRouteDecisionIncident || result.Strategy != AgentDiagnosisStrategyGoS {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRouterLowConfidenceRequiresConfirmation(t *testing.T) {
	router := testAgentRouter(`{"intent":"incident","recommended_strategy":"plan_execute_replan","confidence":0.4,"reason":"信息不足"}`, testAgentRouterConfig())
	result, err := router.Decide(context.Background(), &AgentRouteInput{Query: "服务好像有问题", RouteMode: AgentRouteModeAuto})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if result.Decision != AgentRouteDecisionConfirm || !result.Degraded {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRouterRejectsUnknownAndDisabledStrategies(t *testing.T) {
	router := testAgentRouter(`{}`, testAgentRouterConfig())
	_, err := router.Decide(context.Background(), &AgentRouteInput{
		Query: "诊断 Redis", RouteMode: AgentRouteModeDiagnosis, DiagnosisStrategy: "unknown",
	})
	if !errors.Is(err, ErrDiagnosisStrategyUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}

	cfg := testAgentRouterConfig()
	cfg.AllowedDiagnosisStrategies = map[AgentDiagnosisStrategy]struct{}{AgentDiagnosisStrategyPlan: {}}
	router = testAgentRouter(`{}`, cfg)
	_, err = router.Decide(context.Background(), &AgentRouteInput{
		Query: "诊断 Redis", RouteMode: AgentRouteModeDiagnosis, DiagnosisStrategy: AgentDiagnosisStrategyGoS,
	})
	if !errors.Is(err, ErrDiagnosisStrategyUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func TestAgentRouterAutoUsesRouteCacheBeforeFlash(t *testing.T) {
	cfg := testAgentRouterConfig()
	cfg.RouteCacheTTL = time.Minute
	cfg.RouteCacheMaxEntries = 8
	router := NewAgentRouterApp()
	router.SetConfigLoader(func(context.Context) AgentRouterConfig { return cfg })
	generated := 0
	router.SetGenerate(func(context.Context, string) (string, error) {
		generated++
		return `{"intent":"chat","recommended_strategy":"plan_execute_replan","confidence":0.91,"reason":"概念问答"}`, nil
	})

	for range 2 {
		result, err := router.Decide(context.Background(), &AgentRouteInput{Query: "什么是 Redis？", RouteMode: AgentRouteModeAuto})
		if err != nil || result.Decision != AgentRouteDecisionChat {
			t.Fatalf("unexpected result: %#v, err=%v", result, err)
		}
	}
	if generated != 1 {
		t.Fatalf("Flash generated %d times, want 1", generated)
	}
}

func TestAgentRouterAutoUsesHighConfidenceKeywordBeforeFlash(t *testing.T) {
	cfg := testAgentRouterConfig()
	cfg.HighConfidenceKeywords = []string{"p95 升高"}
	router := NewAgentRouterApp()
	router.SetConfigLoader(func(context.Context) AgentRouterConfig { return cfg })
	router.SetGenerate(func(context.Context, string) (string, error) {
		t.Fatal("Flash must not be called when a high-confidence keyword matches")
		return "", nil
	})

	result, err := router.Decide(context.Background(), &AgentRouteInput{Query: "paymentservice P95 升高", RouteMode: AgentRouteModeAuto})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if result.Decision != AgentRouteDecisionIncident || result.Strategy != AgentDiagnosisStrategyPlan || result.Confidence != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestAgentRouterExpiredCacheFallsBackToFlash(t *testing.T) {
	cfg := testAgentRouterConfig()
	cfg.RouteCacheTTL = time.Minute
	cfg.RouteCacheMaxEntries = 8
	router := NewAgentRouterApp()
	router.SetConfigLoader(func(context.Context) AgentRouterConfig { return cfg })
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	router.now = func() time.Time { return now }
	generated := 0
	router.SetGenerate(func(context.Context, string) (string, error) {
		generated++
		return `{"intent":"chat","recommended_strategy":"plan_execute_replan","confidence":0.91,"reason":"概念问答"}`, nil
	})

	_, _ = router.Decide(context.Background(), &AgentRouteInput{Query: "什么是 Redis？", RouteMode: AgentRouteModeAuto})
	now = now.Add(time.Minute + time.Nanosecond)
	_, _ = router.Decide(context.Background(), &AgentRouteInput{Query: "什么是 Redis？", RouteMode: AgentRouteModeAuto})
	if generated != 2 {
		t.Fatalf("Flash generated %d times after cache expiry, want 2", generated)
	}
}
