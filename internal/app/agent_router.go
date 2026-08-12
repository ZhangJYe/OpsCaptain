package app

import (
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/promptreg"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type AgentRouteMode string

const (
	AgentRouteModeAuto      AgentRouteMode = "auto"
	AgentRouteModeReact     AgentRouteMode = "react"
	AgentRouteModeDiagnosis AgentRouteMode = "diagnosis"

	AgentDiagnosisStrategyAuto AgentDiagnosisStrategy = "auto"
	AgentDiagnosisStrategyPlan AgentDiagnosisStrategy = "plan_execute_replan"
	AgentDiagnosisStrategyGoS  AgentDiagnosisStrategy = "gos_engine"
)

type AgentDiagnosisStrategy string

type AgentRouteDecision string

const (
	AgentRouteDecisionChat     AgentRouteDecision = "chat"
	AgentRouteDecisionIncident AgentRouteDecision = "incident"
	AgentRouteDecisionConfirm  AgentRouteDecision = "confirm"
)

var ErrDiagnosisStrategyUnavailable = errors.New("诊断策略不可用")

type AgentRouterConfig struct {
	Enabled                    bool
	Timeout                    time.Duration
	ConfidenceThreshold        float64
	DefaultDiagnosisStrategy   AgentDiagnosisStrategy
	AllowedDiagnosisStrategies map[AgentDiagnosisStrategy]struct{}
	RouteCacheTTL              time.Duration
	RouteCacheMaxEntries       int
	HighConfidenceKeywords     []string
}

type AgentRouteInput struct {
	Query             string
	RouteMode         AgentRouteMode
	DiagnosisStrategy AgentDiagnosisStrategy
}

type AgentRouteResult struct {
	Decision   AgentRouteDecision     `json:"decision"`
	Strategy   AgentDiagnosisStrategy `json:"strategy,omitempty"`
	Confidence float64                `json:"confidence,omitempty"`
	Reason     string                 `json:"reason"`
	Degraded   bool                   `json:"degraded,omitempty"`
}

type flashRouteOutput struct {
	Intent              string  `json:"intent"`
	RecommendedStrategy string  `json:"recommended_strategy"`
	Confidence          float64 `json:"confidence"`
	Reason              string  `json:"reason"`
}

// AgentRouterApp only classifies a request. It intentionally has no tool, AIOps,
// or persistence dependency so a route decision cannot execute an operation.
type AgentRouterApp struct {
	loadConfig func(context.Context) AgentRouterConfig
	generate   func(context.Context, string) (string, error)
	now        func() time.Time
	cacheMu    sync.Mutex
	cache      map[string]agentRouteCacheEntry
}

type agentRouteCacheEntry struct {
	result    AgentRouteResult
	expiresAt time.Time
}

func NewAgentRouterApp() *AgentRouterApp {
	a := &AgentRouterApp{loadConfig: LoadAgentRouterConfig, now: time.Now, cache: make(map[string]agentRouteCacheEntry)}
	a.generate = a.generateWithFlash
	return a
}

func (a *AgentRouterApp) SetGenerate(fn func(context.Context, string) (string, error)) {
	if fn != nil {
		a.generate = fn
	}
}

func (a *AgentRouterApp) SetConfigLoader(fn func(context.Context) AgentRouterConfig) {
	if fn != nil {
		a.loadConfig = fn
	}
}

func (a *AgentRouterApp) Decide(ctx context.Context, input *AgentRouteInput) (*AgentRouteResult, error) {
	if input == nil || strings.TrimSpace(input.Query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	mode, err := normalizeAgentRouteMode(input.RouteMode)
	if err != nil {
		return nil, err
	}
	strategy, err := normalizeAgentDiagnosisStrategy(input.DiagnosisStrategy)
	if err != nil {
		return nil, err
	}
	cfg := a.loadConfig(ctx)

	switch mode {
	case AgentRouteModeReact:
		return &AgentRouteResult{Decision: AgentRouteDecisionChat, Reason: "已按 ReAct 问答方式处理"}, nil
	case AgentRouteModeDiagnosis:
		if strategy != AgentDiagnosisStrategyAuto {
			if !cfg.isAllowed(strategy) {
				return nil, fmt.Errorf("%w：%s", ErrDiagnosisStrategyUnavailable, strategy)
			}
			return &AgentRouteResult{Decision: AgentRouteDecisionIncident, Strategy: strategy, Reason: "已按指定诊断策略处理"}, nil
		}
	}

	if !cfg.Enabled {
		return confirmRoute("智能路由暂不可用，请选择继续问答或启动排障"), nil
	}
	if mode == AgentRouteModeAuto {
		if cached, ok := a.loadCachedRoute(cfg, input); ok {
			return cached, nil
		}
		if cfg.matchesHighConfidenceKeyword(input.Query) {
			selectedStrategy := strategy
			if selectedStrategy == AgentDiagnosisStrategyAuto {
				selectedStrategy = cfg.DefaultDiagnosisStrategy
			}
			if !cfg.isAllowed(selectedStrategy) {
				return confirmRoute("默认诊断策略当前不可用，请选择 Plan 或 GoS"), nil
			}
			result := &AgentRouteResult{
				Decision:   AgentRouteDecisionIncident,
				Strategy:   selectedStrategy,
				Confidence: 1,
				Reason:     "命中高置信故障规则，已直接启动诊断",
			}
			a.cacheRoute(cfg, input, result)
			return result, nil
		}
	}

	routeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	raw, err := a.generate(routeCtx, strings.TrimSpace(input.Query))
	if err != nil {
		return confirmRoute("智能路由暂不可用，请手动选择执行方式"), nil
	}
	output, err := parseFlashRouteOutput(raw)
	if err != nil {
		return confirmRoute("路由结果无法确认，请手动选择执行方式"), nil
	}
	if output.Confidence < cfg.ConfidenceThreshold {
		return confirmRoute("路由置信度不足，请确认本次请求的执行方式"), nil
	}

	if mode == AgentRouteModeDiagnosis {
		strategy := AgentDiagnosisStrategy(output.RecommendedStrategy)
		if !cfg.isAllowed(strategy) {
			return confirmRoute("推荐策略当前不可用，请选择 Plan 或 GoS"), nil
		}
		return &AgentRouteResult{Decision: AgentRouteDecisionIncident, Strategy: strategy, Confidence: output.Confidence, Reason: output.Reason}, nil
	}

	switch output.Intent {
	case string(AgentRouteDecisionChat):
		result := &AgentRouteResult{Decision: AgentRouteDecisionChat, Confidence: output.Confidence, Reason: output.Reason}
		a.cacheRoute(cfg, input, result)
		return result, nil
	case string(AgentRouteDecisionIncident):
		selectedStrategy := strategy
		if selectedStrategy == AgentDiagnosisStrategyAuto {
			selectedStrategy = AgentDiagnosisStrategy(output.RecommendedStrategy)
		}
		if !cfg.isAllowed(selectedStrategy) {
			return confirmRoute("推荐策略当前不可用，请选择 Plan 或 GoS"), nil
		}
		result := &AgentRouteResult{Decision: AgentRouteDecisionIncident, Strategy: selectedStrategy, Confidence: output.Confidence, Reason: output.Reason}
		a.cacheRoute(cfg, input, result)
		return result, nil
	default:
		return confirmRoute("路由意图无法确认，请手动选择执行方式"), nil
	}
}

func (a *AgentRouterApp) generateWithFlash(ctx context.Context, query string) (string, error) {
	chatModel, err := models.OpenAIForGLMFast(ctx)
	if err != nil {
		return "", err
	}
	response, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: promptreg.AgentRoute},
		{Role: schema.User, Content: query},
	})
	if err != nil || response == nil {
		return "", err
	}
	return response.Content, nil
}

func LoadAgentRouterConfig(ctx context.Context) AgentRouterConfig {
	cfg := AgentRouterConfig{
		Enabled:                  true,
		Timeout:                  5 * time.Second,
		ConfidenceThreshold:      0.75,
		DefaultDiagnosisStrategy: AgentDiagnosisStrategyPlan,
		AllowedDiagnosisStrategies: map[AgentDiagnosisStrategy]struct{}{
			AgentDiagnosisStrategyPlan: {},
			AgentDiagnosisStrategyGoS:  {},
		},
		RouteCacheTTL:        5 * time.Minute,
		RouteCacheMaxEntries: 256,
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.enabled"); err == nil && !v.IsNil() {
		cfg.Enabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.timeout_ms"); err == nil && v.Int() > 0 {
		cfg.Timeout = time.Duration(v.Int()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.confidence_threshold"); err == nil && v.Float64() > 0 && v.Float64() <= 1 {
		cfg.ConfidenceThreshold = v.Float64()
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.default_diagnosis_strategy"); err == nil {
		if strategy, normalizeErr := normalizeAgentDiagnosisStrategy(AgentDiagnosisStrategy(v.String())); normalizeErr == nil && strategy != AgentDiagnosisStrategyAuto {
			cfg.DefaultDiagnosisStrategy = strategy
		}
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.allowed_diagnosis_strategies"); err == nil && len(v.Strings()) > 0 {
		allowed := make(map[AgentDiagnosisStrategy]struct{}, len(v.Strings()))
		for _, raw := range v.Strings() {
			if strategy, normalizeErr := normalizeAgentDiagnosisStrategy(AgentDiagnosisStrategy(raw)); normalizeErr == nil && strategy != AgentDiagnosisStrategyAuto {
				allowed[strategy] = struct{}{}
			}
		}
		if len(allowed) > 0 {
			cfg.AllowedDiagnosisStrategies = allowed
		}
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.route_cache_ttl_seconds"); err == nil && v.Int() >= 0 {
		cfg.RouteCacheTTL = time.Duration(v.Int()) * time.Second
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.route_cache_max_entries"); err == nil && v.Int() >= 0 {
		cfg.RouteCacheMaxEntries = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "agent_router.high_confidence_incident_keywords"); err == nil {
		for _, keyword := range v.Strings() {
			if keyword = strings.TrimSpace(keyword); keyword != "" {
				cfg.HighConfidenceKeywords = append(cfg.HighConfidenceKeywords, keyword)
			}
		}
	}
	if !cfg.isAllowed(cfg.DefaultDiagnosisStrategy) {
		for strategy := range cfg.AllowedDiagnosisStrategies {
			cfg.DefaultDiagnosisStrategy = strategy
			break
		}
	}
	return cfg
}

func (c AgentRouterConfig) isAllowed(strategy AgentDiagnosisStrategy) bool {
	_, ok := c.AllowedDiagnosisStrategies[strategy]
	return ok
}

func (c AgentRouterConfig) matchesHighConfidenceKeyword(query string) bool {
	normalized := strings.ToLower(strings.TrimSpace(query))
	for _, keyword := range c.HighConfidenceKeywords {
		if strings.Contains(normalized, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func (a *AgentRouterApp) loadCachedRoute(cfg AgentRouterConfig, input *AgentRouteInput) (*AgentRouteResult, bool) {
	if cfg.RouteCacheTTL <= 0 || cfg.RouteCacheMaxEntries <= 0 {
		return nil, false
	}
	key := routeCacheKey(input)
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	entry, ok := a.cache[key]
	if !ok {
		return nil, false
	}
	if !entry.expiresAt.After(a.now()) {
		delete(a.cache, key)
		return nil, false
	}
	result := entry.result
	return &result, true
}

func (a *AgentRouterApp) cacheRoute(cfg AgentRouterConfig, input *AgentRouteInput, result *AgentRouteResult) {
	if cfg.RouteCacheTTL <= 0 || cfg.RouteCacheMaxEntries <= 0 || result == nil || result.Decision == AgentRouteDecisionConfirm {
		return
	}
	key := routeCacheKey(input)
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	now := a.now()
	for cachedKey, entry := range a.cache {
		if !entry.expiresAt.After(now) {
			delete(a.cache, cachedKey)
		}
	}
	if _, exists := a.cache[key]; !exists && len(a.cache) >= cfg.RouteCacheMaxEntries {
		for cachedKey := range a.cache {
			delete(a.cache, cachedKey)
			break
		}
	}
	a.cache[key] = agentRouteCacheEntry{result: *result, expiresAt: now.Add(cfg.RouteCacheTTL)}
}

func routeCacheKey(input *AgentRouteInput) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(input.Query), " "))
	digest := sha256.Sum256([]byte(normalized))
	mode := input.RouteMode
	if mode == "" {
		mode = AgentRouteModeAuto
	}
	strategy := input.DiagnosisStrategy
	if strategy == "" {
		strategy = AgentDiagnosisStrategyAuto
	}
	return fmt.Sprintf("%s:%s:%x", mode, strategy, digest[:])
}

func parseFlashRouteOutput(raw string) (*flashRouteOutput, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	var output flashRouteOutput
	if err := json.Unmarshal([]byte(strings.TrimSpace(trimmed)), &output); err != nil {
		return nil, err
	}
	output.Intent = strings.TrimSpace(output.Intent)
	output.RecommendedStrategy = strings.TrimSpace(output.RecommendedStrategy)
	output.Reason = strings.TrimSpace(output.Reason)
	if output.Intent != string(AgentRouteDecisionChat) && output.Intent != string(AgentRouteDecisionIncident) {
		return nil, fmt.Errorf("unsupported route intent %q", output.Intent)
	}
	if _, err := normalizeAgentDiagnosisStrategy(AgentDiagnosisStrategy(output.RecommendedStrategy)); err != nil || output.RecommendedStrategy == string(AgentDiagnosisStrategyAuto) {
		return nil, fmt.Errorf("unsupported recommended strategy %q", output.RecommendedStrategy)
	}
	if output.Confidence < 0 || output.Confidence > 1 || output.Reason == "" {
		return nil, errors.New("invalid Flash route output")
	}
	return &output, nil
}

func confirmRoute(reason string) *AgentRouteResult {
	return &AgentRouteResult{Decision: AgentRouteDecisionConfirm, Reason: reason, Degraded: true}
}

func normalizeAgentRouteMode(mode AgentRouteMode) (AgentRouteMode, error) {
	if mode == "" {
		return AgentRouteModeAuto, nil
	}
	switch mode {
	case AgentRouteModeAuto, AgentRouteModeReact, AgentRouteModeDiagnosis:
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported route mode %q", mode)
	}
}

func normalizeAgentDiagnosisStrategy(strategy AgentDiagnosisStrategy) (AgentDiagnosisStrategy, error) {
	if strategy == "" {
		return AgentDiagnosisStrategyAuto, nil
	}
	if strategy == "gos" {
		strategy = AgentDiagnosisStrategyGoS
	}
	switch strategy {
	case AgentDiagnosisStrategyAuto, AgentDiagnosisStrategyPlan, AgentDiagnosisStrategyGoS:
		return strategy, nil
	default:
		return "", fmt.Errorf("%w：%s", ErrDiagnosisStrategyUnavailable, strategy)
	}
}
