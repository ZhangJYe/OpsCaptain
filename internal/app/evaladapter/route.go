package evaladapter

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/evalharness"
	"SuperBizAgent/internal/app"
)

const (
	RoutePayloadSchema   = "route-eval/v1"
	RoutePayloadSchemaV2 = "route-eval/v2"
)

type RoutePayload struct {
	ExpectedDecision       string                      `json:"expected_decision"`
	ExpectedStrategy       string                      `json:"expected_strategy,omitempty"`
	ModelOutput            string                      `json:"model_output"`
	HighConfidenceKeywords []string                    `json:"high_confidence_keywords,omitempty"`
	ConfidenceThreshold    float64                     `json:"confidence_threshold,omitempty"`
	Repeat                 int                         `json:"repeat,omitempty"`
	AcceptableIntents      []string                    `json:"acceptable_intents,omitempty"`
	ExpectedPublicRoute    string                      `json:"expected_public_route,omitempty"`
	NeedClarification      *bool                       `json:"need_clarification,omitempty"`
	Entities               map[string]string           `json:"entities,omitempty"`
	MissingSlots           []string                    `json:"missing_slots,omitempty"`
	RiskLevel              string                      `json:"risk_level,omitempty"`
	GroupID                string                      `json:"group_id,omitempty"`
	ContextSnapshot        *app.RoutingContextSnapshot `json:"context_snapshot,omitempty"`
	Variant                string                      `json:"variant,omitempty"`
}

type RouteAdapter struct{}

type RouteVariantReport struct {
	Variant string          `json:"variant"`
	Cases   int             `json:"cases"`
	Schema  string          `json:"schema"`
	Metrics json.RawMessage `json:"metrics"`
}

// CompareRouteVariants replays the same frozen cases with identical fixtures
// and budgets. It is deterministic/recorded evaluation only; live callers
// must provide their own authorized model adapter and frozen dataset.
func CompareRouteVariants(ctx context.Context, cases []evalharness.CaseEnvelope, variants []string) (map[string]RouteVariantReport, error) {
	adapter := NewRouteAdapter()
	reports := make(map[string]RouteVariantReport, len(variants))
	for _, variant := range variants {
		if strings.TrimSpace(variant) == "" {
			continue
		}
		results := make([]evalharness.CaseResult, 0, len(cases))
		for _, original := range cases {
			payload := RoutePayload{}
			if err := json.Unmarshal(original.Payload, &payload); err != nil {
				return nil, fmt.Errorf("case %s: %w", original.ID, err)
			}
			payload.Variant = variant
			encoded, err := json.Marshal(payload)
			if err != nil {
				return nil, fmt.Errorf("case %s: %w", original.ID, err)
			}
			current := original
			current.PayloadSchemaVersion = RoutePayloadSchemaV2
			current.Payload = encoded
			results = append(results, adapter.RunCase(ctx, current))
		}
		schema, metrics, _, err := adapter.Aggregate(results)
		if err != nil {
			return nil, fmt.Errorf("variant %s: %w", variant, err)
		}
		reports[variant] = RouteVariantReport{Variant: variant, Cases: len(results), Schema: schema, Metrics: metrics}
	}
	return reports, nil
}

type routeCaseDomain struct {
	Matched             bool                      `json:"matched"`
	ExpectedDecision    string                    `json:"expected_decision"`
	ActualDecision      string                    `json:"actual_decision"`
	ExpectedStrategy    string                    `json:"expected_strategy,omitempty"`
	ActualStrategy      string                    `json:"actual_strategy,omitempty"`
	Confidence          float64                   `json:"confidence"`
	Source              string                    `json:"source"`
	RoutingInputHash    string                    `json:"routing_input_hash"`
	MemoryPresent       bool                      `json:"memory_present"`
	Degraded            bool                      `json:"degraded"`
	Schema              string                    `json:"schema"`
	AcceptableIntents   []string                  `json:"acceptable_intents,omitempty"`
	ExpectedPublicRoute string                    `json:"expected_public_route,omitempty"`
	NeedClarification   bool                      `json:"need_clarification"`
	ActualCandidates    []app.AgentRouteCandidate `json:"actual_candidates,omitempty"`
	Top2Hit             bool                      `json:"top2_hit"`
	ActualClarification bool                      `json:"actual_clarification"`
	ContextUsed         bool                      `json:"context_used"`
	ContextMisuse       bool                      `json:"context_misuse"`
	RiskLevel           string                    `json:"risk_level,omitempty"`
	HighRiskFalseRoute  bool                      `json:"high_risk_false_route"`
	OODRejected         bool                      `json:"ood_rejected"`
	Ambiguous           bool                      `json:"ambiguous"`
	Variant             string                    `json:"variant,omitempty"`
}

func NewRouteAdapter() *RouteAdapter                { return &RouteAdapter{} }
func (a *RouteAdapter) Name() evalharness.SuiteName { return evalharness.SuiteRoute }
func (a *RouteAdapter) PayloadSchema() string       { return RoutePayloadSchema }
func (a *RouteAdapter) SupportedPayloadSchemas() []string {
	return []string{RoutePayloadSchema, RoutePayloadSchemaV2}
}
func (a *RouteAdapter) Validate(_ evalharness.SuiteConfig, _ evalharness.DatasetRole, profile evalharness.Profile) error {
	return evalharness.RejectLiveProfile(profile)
}
func (a *RouteAdapter) RunCase(ctx context.Context, evalCase evalharness.CaseEnvelope) evalharness.CaseResult {
	start := time.Now()
	var payload RoutePayload
	if err := json.Unmarshal(evalCase.Payload, &payload); err != nil {
		return evalharness.CaseResult{CaseID: evalCase.ID, Status: evalharness.StatusFailed, FailurePhase: "route", Reason: err.Error()}
	}
	cfg := app.AgentRouterConfig{
		Enabled: true, Timeout: time.Second, ConfidenceThreshold: payload.ConfidenceThreshold,
		DefaultDiagnosisStrategy:   app.AgentDiagnosisStrategyPlan,
		AllowedDiagnosisStrategies: map[app.AgentDiagnosisStrategy]struct{}{app.AgentDiagnosisStrategyPlan: {}, app.AgentDiagnosisStrategyGoS: {}},
		HighConfidenceKeywords:     payload.HighConfidenceKeywords,
		RouteCacheTTL:              time.Minute,
		RouteCacheMaxEntries:       16,
	}
	isV2 := evalCase.PayloadSchemaVersion == RoutePayloadSchemaV2
	if isV2 {
		cfg.IntentFunnelEnabled = true
		cfg.IntentFunnelTopK = 2
		cfg.IntentFunnelAcceptThreshold = 0.75
		cfg.IntentFunnelMarginThreshold = 0.15
		cfg.IntentFunnelContextTTL = 15 * time.Minute
		cfg.IntentFunnelMaxClarifications = 1
		cfg.IntentFunnelPolicyVersion = "eval-v2"
		cfg.HighRiskActionKeywords = []string{"删除", "重启", "修改配置", "restart", "drop database"}
		cfg.InjectionKeywords = []string{"ignore previous instructions", "绕过审批", "忽略系统规则"}
	}
	switch payload.Variant {
	case "A", "a":
		cfg.IntentFunnelEnabled = false
	case "B-no-context":
		cfg.IntentFunnelEnabled = true
		payload.ContextSnapshot = nil
	case "B-no-fast-path":
		cfg.IntentFunnelEnabled = true
		cfg.HighConfidenceKeywords = nil
	case "B-full", "B", "b":
		cfg.IntentFunnelEnabled = true
	}
	if cfg.ConfidenceThreshold <= 0 {
		cfg.ConfidenceThreshold = 0.75
	}
	router := app.NewAgentRouterApp()
	router.SetConfigLoader(func(context.Context) app.AgentRouterConfig { return cfg })
	router.SetGenerate(func(context.Context, string) (string, error) {
		if payload.ModelOutput == "" {
			return "", fmt.Errorf("model output fixture is required")
		}
		return payload.ModelOutput, nil
	})
	repeat := payload.Repeat
	if repeat <= 0 {
		repeat = 1
	}
	var result *app.AgentRouteResult
	var err error
	for range repeat {
		result, err = router.Decide(ctx, &app.AgentRouteInput{Query: evalCase.Input.Query, RouteMode: app.AgentRouteModeAuto, DiagnosisStrategy: app.AgentDiagnosisStrategyAuto, RoutingContext: payload.ContextSnapshot})
		if err != nil {
			break
		}
	}
	if err != nil {
		return evalharness.CaseResult{CaseID: evalCase.ID, Status: evalharness.StatusFailed, FailurePhase: "route", Reason: err.Error(), Latency: time.Since(start)}
	}
	expectedRoute := payload.ExpectedPublicRoute
	if expectedRoute == "" {
		expectedRoute = payload.ExpectedDecision
	}
	matched := string(result.Decision) == expectedRoute && (payload.ExpectedStrategy == "" || string(result.Strategy) == payload.ExpectedStrategy)
	if isV2 && len(payload.AcceptableIntents) > 0 && len(result.Candidates) > 0 {
		matched = matched && hasAcceptableCandidate(result.Candidates, payload.AcceptableIntents)
	}
	if isV2 && payload.NeedClarification != nil {
		matched = matched && (result.Clarification != nil) == *payload.NeedClarification
	}
	actualCandidates := result.Candidates
	top2Hit := hasAcceptableCandidate(actualCandidates, payload.AcceptableIntents)
	contextUsed := result.Trace != nil && result.Trace.ContextUsed
	actualClarification := result.Clarification != nil
	highRiskFalse := payload.RiskLevel == "high" && result.Decision != app.AgentRouteDecisionConfirm
	oodRejected := (payload.RiskLevel == "ood" || payload.RiskLevel == "out_of_scope") && result.Decision == app.AgentRouteDecisionConfirm
	contextMisuse := contextUsed && expectedRoute != "" && string(result.Decision) != expectedRoute
	status := evalharness.StatusSucceeded
	domain := routeCaseDomain{
		Matched:          matched,
		ExpectedDecision: expectedRoute, ActualDecision: string(result.Decision),
		ExpectedStrategy: payload.ExpectedStrategy, ActualStrategy: string(result.Strategy), Confidence: result.Confidence,
		Source: result.Source, RoutingInputHash: evalharness.QueryHash(evalCase.Input.Query), MemoryPresent: len(evalCase.Input.Memory) > 0,
		Degraded: result.Degraded, Schema: evalCase.PayloadSchemaVersion, AcceptableIntents: payload.AcceptableIntents,
		ExpectedPublicRoute: expectedRoute, NeedClarification: payload.NeedClarification != nil && *payload.NeedClarification,
		ActualCandidates: actualCandidates, Top2Hit: top2Hit, ActualClarification: actualClarification,
		ContextUsed: contextUsed, ContextMisuse: contextMisuse, RiskLevel: payload.RiskLevel,
		HighRiskFalseRoute: highRiskFalse, OODRejected: oodRejected,
		Ambiguous: len(payload.AcceptableIntents) > 1 || (payload.NeedClarification != nil && *payload.NeedClarification),
		Variant:   payload.Variant,
	}
	usage := evalharness.Usage{}
	if payload.ModelOutput != "" {
		usage.LLMCalls = 1
	}
	return evalharness.CaseResult{CaseID: evalCase.ID, Status: status, Matched: matched, Latency: time.Since(start), Usage: usage, TraceComplete: true, FailurePhase: "route", Domain: evalharness.MarshalDomain(domain)}
}
func (a *RouteAdapter) Aggregate(results []evalharness.CaseResult) (string, json.RawMessage, []evalharness.GateResult, error) {
	labels := []string{"chat", "incident", "confirm"}
	confusion := make(map[string]map[string]int)
	perClass := make(map[string]map[string]float64)
	lowConfidence, cacheHits, fallbacks := 0, 0, 0
	v2Cases := 0
	top2Hits, ambiguityTotal, ambiguityCorrect := 0, 0, 0
	contextTotal, contextMisuse := 0, 0
	unnecessaryClarifications, missedClarifications, clarifiedCorrect := 0, 0, 0
	highRiskFalse, oodTotal, oodRejected := 0, 0, 0
	confidenceError, brier := 0.0, 0.0
	latencies := make([]time.Duration, 0, len(results))
	for _, label := range labels {
		confusion[label] = make(map[string]int)
	}
	for _, result := range results {
		var domain routeCaseDomain
		if len(result.Domain) == 0 {
			continue
		}
		if err := json.Unmarshal(result.Domain, &domain); err != nil {
			return "", nil, nil, err
		}
		confusion[domain.ExpectedDecision][domain.ActualDecision]++
		latencies = append(latencies, result.Latency)
		if domain.Schema == RoutePayloadSchemaV2 {
			v2Cases++
		}
		if domain.Schema == RoutePayloadSchemaV2 {
			if domain.Top2Hit {
				top2Hits++
			}
			if domain.Ambiguous {
				ambiguityTotal++
				if domain.Matched {
					ambiguityCorrect++
				}
			}
			if domain.ContextUsed {
				contextTotal++
				if domain.ContextMisuse {
					contextMisuse++
				}
			}
			if domain.ActualClarification && !domain.NeedClarification {
				unnecessaryClarifications++
			}
			if !domain.ActualClarification && domain.NeedClarification {
				missedClarifications++
			}
			if domain.ActualClarification && domain.NeedClarification && domain.Matched {
				clarifiedCorrect++
			}
			if domain.HighRiskFalseRoute {
				highRiskFalse++
			}
			if domain.RiskLevel == "ood" || domain.RiskLevel == "out_of_scope" {
				oodTotal++
				if domain.OODRejected {
					oodRejected++
				}
			}
			confidence := domain.Confidence
			correct := 0.0
			if domain.Matched {
				correct = 1
			}
			confidenceError += math.Abs(confidence - correct)
			brier += (confidence - correct) * (confidence - correct)
		}
		if domain.Confidence > 0 && domain.Confidence < 0.75 {
			lowConfidence++
		}
		if domain.Source == "cache" {
			cacheHits++
		}
		if domain.Source == "fallback" {
			fallbacks++
		}
	}
	macroF1 := 0.0
	for _, label := range labels {
		tp := confusion[label][label]
		fp, fn := 0, 0
		for _, other := range labels {
			if other != label {
				fp += confusion[other][label]
				fn += confusion[label][other]
			}
		}
		precision, recall := ratio(tp, tp+fp), ratio(tp, tp+fn)
		f1 := 0.0
		if precision+recall > 0 {
			f1 = 2 * precision * recall / (precision + recall)
		}
		perClass[label] = map[string]float64{"precision": precision, "recall": recall, "f1": f1}
		macroF1 += f1
	}
	macroF1 /= float64(len(labels))
	count := float64(len(results))
	if count == 0 {
		count = 1
	}
	metrics := map[string]any{"macro_f1": macroF1, "low_confidence_rate": float64(lowConfidence) / count, "cache_hit_rate": float64(cacheHits) / count, "fallback_rate": float64(fallbacks) / count, "confusion_matrix": confusion, "per_class": perClass}
	if v2Cases > 0 {
		v2Count := float64(v2Cases)
		metrics["top2_recall"] = float64(top2Hits) / v2Count
		metrics["ece"] = confidenceError / v2Count
		metrics["brier"] = brier / v2Count
		metrics["ambiguity_resolution_accuracy"] = ratio(ambiguityCorrect, ambiguityTotal)
		metrics["context_misuse_rate"] = ratio(contextMisuse, contextTotal)
		metrics["unnecessary_clarification_rate"] = ratio(unnecessaryClarifications, v2Cases)
		metrics["missed_clarification_rate"] = ratio(missedClarifications, v2Cases)
		metrics["clarification_resolved_accuracy"] = ratio(clarifiedCorrect, ambiguityTotal)
		metrics["high_risk_false_routing"] = float64(highRiskFalse)
		metrics["ood_rejection_rate"] = ratio(oodRejected, oodTotal)
		metrics["p95_latency_ms"] = float64(percentileLatency(latencies, 0.95).Milliseconds())
		metrics["llm_calls"] = float64(v2Cases)
		metrics["token_cost"] = "unavailable"
		return "route-metrics/v2", evalharness.MarshalDomain(metrics), nil, nil
	}
	return "route-metrics/v1", evalharness.MarshalDomain(metrics), nil, nil
}

func hasAcceptableCandidate(candidates []app.AgentRouteCandidate, acceptable []string) bool {
	if len(acceptable) == 0 {
		return true
	}
	allowed := make(map[string]struct{}, len(acceptable))
	for _, item := range acceptable {
		allowed[strings.TrimSpace(item)] = struct{}{}
	}
	for _, candidate := range candidates {
		if _, ok := allowed[string(candidate.Intent)]; ok {
			return true
		}
	}
	return false
}

func percentileLatency(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]time.Duration(nil), values...)
	for i := 1; i < len(copyValues); i++ {
		for j := i; j > 0 && copyValues[j] < copyValues[j-1]; j-- {
			copyValues[j], copyValues[j-1] = copyValues[j-1], copyValues[j]
		}
	}
	index := int(float64(len(copyValues)-1) * percentile)
	return copyValues[index]
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
