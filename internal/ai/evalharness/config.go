package evalharness

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type RuntimeConfig struct {
	Enabled                  bool
	DefaultSuites            []SuiteName
	Budget                   BudgetLimits
	Redaction                RedactionConfig
	GateMaxFailureRate       float64
	GateMaxDegradationRate   float64
	GateMinTraceCompleteness float64
}

func DefaultRuntimeConfig() RuntimeConfig {
	return RuntimeConfig{
		Enabled:       true,
		DefaultSuites: []SuiteName{SuiteRoute, SuiteRAG, SuitePlan, SuiteGoS, SuiteTool, SuiteEvidence},
		Budget: BudgetLimits{
			MaxCases: 100, Concurrency: 1,
			CaseTimeout: 15 * time.Second, TotalTimeout: 2 * time.Minute,
			MaxLLMCalls: 200, MaxToolCalls: 300, MaxRAGCalls: 200, MaxTokens: 500000,
		},
		Redaction: RedactionConfig{
			MaxTextChars:  512,
			SensitiveKeys: []string{"authorization", "token", "api_key", "password", "secret", "dsn"},
		},
		GateMaxFailureRate:       0,
		GateMaxDegradationRate:   0,
		GateMinTraceCompleteness: 1,
	}
}

func LoadRuntimeConfig(ctx context.Context) RuntimeConfig {
	cfg := DefaultRuntimeConfig()
	if v, err := g.Cfg().Get(ctx, "evaluation_harness.enabled"); err == nil && !v.IsNil() {
		cfg.Enabled = v.Bool()
	}
	if v, err := g.Cfg().Get(ctx, "evaluation_harness.suites"); err == nil && len(v.Strings()) > 0 {
		cfg.DefaultSuites = cfg.DefaultSuites[:0]
		for _, name := range v.Strings() {
			cfg.DefaultSuites = append(cfg.DefaultSuites, SuiteName(name))
		}
	}
	readPositiveInt(ctx, "evaluation_harness.budget.max_cases", &cfg.Budget.MaxCases)
	readPositiveInt(ctx, "evaluation_harness.budget.concurrency", &cfg.Budget.Concurrency)
	readPositiveInt(ctx, "evaluation_harness.budget.max_llm_calls", &cfg.Budget.MaxLLMCalls)
	readPositiveInt(ctx, "evaluation_harness.budget.max_tool_calls", &cfg.Budget.MaxToolCalls)
	readPositiveInt(ctx, "evaluation_harness.budget.max_rag_calls", &cfg.Budget.MaxRAGCalls)
	readPositiveInt(ctx, "evaluation_harness.budget.max_tokens", &cfg.Budget.MaxTokens)
	if v, err := g.Cfg().Get(ctx, "evaluation_harness.budget.case_timeout_ms"); err == nil && v.Int64() > 0 {
		cfg.Budget.CaseTimeout = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "evaluation_harness.budget.total_timeout_ms"); err == nil && v.Int64() > 0 {
		cfg.Budget.TotalTimeout = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "evaluation_harness.budget.max_cost"); err == nil && v.Float64() > 0 {
		cfg.Budget.MaxCost = v.Float64()
	}
	if v, err := g.Cfg().Get(ctx, "evaluation_harness.redaction.max_text_chars"); err == nil && v.Int() > 0 {
		cfg.Redaction.MaxTextChars = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "evaluation_harness.redaction.sensitive_keys"); err == nil && len(v.Strings()) > 0 {
		cfg.Redaction.SensitiveKeys = append([]string(nil), v.Strings()...)
	}
	readRate(ctx, "evaluation_harness.gates.max_failure_rate", &cfg.GateMaxFailureRate)
	readRate(ctx, "evaluation_harness.gates.max_degradation_rate", &cfg.GateMaxDegradationRate)
	readRate(ctx, "evaluation_harness.gates.min_trace_completeness", &cfg.GateMinTraceCompleteness)
	return cfg
}

func readPositiveInt(ctx context.Context, key string, target *int) {
	if v, err := g.Cfg().Get(ctx, key); err == nil && v.Int() > 0 {
		*target = v.Int()
	}
}

func readRate(ctx context.Context, key string, target *float64) {
	if v, err := g.Cfg().Get(ctx, key); err == nil && v.Float64() >= 0 && v.Float64() <= 1 {
		*target = v.Float64()
	}
}
