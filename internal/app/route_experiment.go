package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

type RouteExperimentVariant string

const (
	RouteExperimentA RouteExperimentVariant = "A"
	RouteExperimentB RouteExperimentVariant = "B"
)

type RouteExperimentStage string

const (
	RouteExperimentOff      RouteExperimentStage = "off"
	RouteExperimentShadow   RouteExperimentStage = "shadow"
	RouteExperimentCanary5  RouteExperimentStage = "canary_5"
	RouteExperimentCanary25 RouteExperimentStage = "canary_25"
	RouteExperimentHalf     RouteExperimentStage = "half"
)

type RouteExperimentConfig struct {
	Enabled               bool
	ExperimentID          string
	Version               string
	Salt                  string
	RolloutStage          RouteExperimentStage
	RolloutPercent        int
	ShadowEnabled         bool
	ShadowTimeout         time.Duration
	ShadowMaxConcurrency  int
	ShadowTokenBudget     int
	HighRiskExcluded      bool
	OODExcluded           bool
	MaxErrorRate          float64
	MaxTimeoutRate        float64
	MaxP95Latency         time.Duration
	MaxHighRiskFalseRoute int
}

func (c RouteExperimentConfig) Normalize() RouteExperimentConfig {
	if c.ExperimentID == "" {
		c.ExperimentID = "agent-router-funnel"
	}
	if c.Version == "" {
		c.Version = "v1"
	}
	if c.RolloutPercent < 0 {
		c.RolloutPercent = 0
	}
	if c.RolloutPercent > 100 {
		c.RolloutPercent = 100
	}
	if c.ShadowMaxConcurrency <= 0 {
		c.ShadowMaxConcurrency = 1
	}
	if c.ShadowTimeout <= 0 {
		c.ShadowTimeout = 300 * time.Millisecond
	}
	if c.MaxP95Latency <= 0 {
		c.MaxP95Latency = time.Second
	}
	if c.MaxErrorRate < 0 {
		c.MaxErrorRate = 0.05
	}
	if c.MaxTimeoutRate < 0 {
		c.MaxTimeoutRate = 0.02
	}
	return c
}

type RouteExperimentAssignment struct {
	ExperimentID    string                 `json:"experiment_id"`
	Version         string                 `json:"version"`
	AssignmentHash  string                 `json:"assignment_key_hash"`
	Variant         RouteExperimentVariant `json:"variant"`
	ServedVariant   RouteExperimentVariant `json:"served_variant"`
	Shadow          bool                   `json:"shadow"`
	Excluded        bool                   `json:"excluded"`
	ExclusionReason string                 `json:"exclusion_reason,omitempty"`
}

// Assign uses only a server-provided anonymous key. Client-provided variants
// are intentionally not accepted by this API.
func AssignRouteExperiment(cfg RouteExperimentConfig, assignmentKey string, risk AgentRouteRisk, ood bool) RouteExperimentAssignment {
	cfg = cfg.Normalize()
	keyHash := hashAssignmentKey(assignmentKey)
	assignment := RouteExperimentAssignment{ExperimentID: cfg.ExperimentID, Version: cfg.Version, AssignmentHash: keyHash, Variant: RouteExperimentA, ServedVariant: RouteExperimentA}
	if !cfg.Enabled || cfg.RolloutStage == RouteExperimentOff || strings.TrimSpace(assignmentKey) == "" {
		assignment.Excluded = true
		assignment.ExclusionReason = "experiment_disabled_or_missing_key"
		return assignment
	}
	if (cfg.HighRiskExcluded && risk == AgentRouteRiskHigh) || (cfg.OODExcluded && ood) {
		assignment.Excluded = true
		assignment.ExclusionReason = "risk_or_ood_excluded"
		return assignment
	}
	if cfg.RolloutStage == RouteExperimentShadow || cfg.ShadowEnabled {
		assignment.Shadow = true
		assignment.Variant = RouteExperimentB
		return assignment
	}
	bucket := stableBucket(cfg.Salt, assignmentKey)
	if bucket < cfg.RolloutPercent {
		assignment.Variant = RouteExperimentB
		assignment.ServedVariant = RouteExperimentB
	}
	return assignment
}

func stableBucket(salt, assignmentKey string) int {
	mac := hmac.New(sha256.New, []byte(salt))
	_, _ = mac.Write([]byte(assignmentKey))
	digest := mac.Sum(nil)
	return int((uint16(digest[0])<<8 | uint16(digest[1])) % 100)
}

func hashAssignmentKey(key string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(digest[:8])
}

type RouteShadowResult struct {
	Assignment RouteExperimentAssignment `json:"assignment"`
	BResult    *AgentRouteResult         `json:"b_result,omitempty"`
	TimedOut   bool                      `json:"timed_out"`
	Error      string                    `json:"error,omitempty"`
	LatencyMS  int64                     `json:"latency_ms"`
}

// RunRouteShadow never returns B as the service decision. The callback must be
// classification-only; callers must not attach tools or incident creation.
func RunRouteShadow(ctx context.Context, cfg RouteExperimentConfig, assignment RouteExperimentAssignment, classify func(context.Context) (*AgentRouteResult, error)) RouteShadowResult {
	started := time.Now()
	result := RouteShadowResult{Assignment: assignment}
	if !assignment.Shadow || classify == nil {
		result.LatencyMS = time.Since(started).Milliseconds()
		return result
	}
	shadowCtx, cancel := context.WithTimeout(ctx, cfg.Normalize().ShadowTimeout)
	defer cancel()
	type response struct {
		result *AgentRouteResult
		err    error
	}
	ch := make(chan response, 1)
	go func() { value, err := classify(shadowCtx); ch <- response{value, err} }()
	select {
	case value := <-ch:
		result.BResult, result.Error = value.result, errorString(value.err)
	case <-shadowCtx.Done():
		result.TimedOut = true
		result.Error = shadowCtx.Err().Error()
	}
	result.LatencyMS = time.Since(started).Milliseconds()
	return result
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type RouteGuardrailObservation struct {
	ErrorRate           float64
	TimeoutRate         float64
	P95Latency          time.Duration
	HighRiskFalseRoutes int
}

type RouteGuardrail struct {
	mu      sync.RWMutex
	config  RouteExperimentConfig
	stopped bool
	reason  string
}

func NewRouteGuardrail(config RouteExperimentConfig) *RouteGuardrail {
	return &RouteGuardrail{config: config.Normalize()}
}

func (g *RouteGuardrail) Observe(observation RouteGuardrailObservation) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return true
	}
	cfg := g.config.Normalize()
	switch {
	case observation.HighRiskFalseRoutes > cfg.MaxHighRiskFalseRoute:
		g.stopped, g.reason = true, "high_risk_false_route"
	case observation.ErrorRate > cfg.MaxErrorRate:
		g.stopped, g.reason = true, "error_rate"
	case observation.TimeoutRate > cfg.MaxTimeoutRate:
		g.stopped, g.reason = true, "timeout_rate"
	case observation.P95Latency > cfg.MaxP95Latency:
		g.stopped, g.reason = true, "p95_latency"
	}
	return g.stopped
}

func (g *RouteGuardrail) Stopped() (bool, string) {
	if g == nil {
		return false, ""
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.stopped, g.reason
}

func (g *RouteGuardrail) Reset() {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.stopped, g.reason = false, ""
	g.mu.Unlock()
}

type RouteFeedbackEvent struct {
	RequestID    string    `json:"request_id"`
	SessionID    string    `json:"session_id,omitempty"`
	ExperimentID string    `json:"experiment_id,omitempty"`
	Kind         string    `json:"kind"`
	Value        string    `json:"value,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type RouteFeedbackDeduper struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func NewRouteFeedbackDeduper() *RouteFeedbackDeduper {
	return &RouteFeedbackDeduper{seen: make(map[string]struct{})}
}

func (d *RouteFeedbackDeduper) Accept(event RouteFeedbackEvent) bool {
	if d == nil || strings.TrimSpace(event.RequestID) == "" || strings.TrimSpace(event.Kind) == "" {
		return false
	}
	key := event.RequestID + "\x00" + event.Kind
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.seen[key]; exists {
		return false
	}
	d.seen[key] = struct{}{}
	return true
}
