package events

import (
	"context"
	"regexp"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

// HallucinationConfig 防幻觉配置
type HallucinationConfig struct {
	Enabled                    bool
	ContractEnabled            bool
	ContractBlockOnViolation   bool
	SchemaGateEnabled          bool
	OutputValidationEnabled    bool
	NoToolCallDetectionEnabled bool
	OpsKeywords                []string
	HedgeWords                 []string
	ProblemWords               []string
	ActionWords                []string
	Contradictions             [][2]string
	MetricPattern              *regexp.Regexp
	LLMValidation              *LLMValidationConfig
}

// LLMValidationConfig LLM 校验配置
type LLMValidationConfig struct {
	Enabled           bool
	Model             string
	TimeoutMs         int
	MaxTokens         int
	OmissionDetection bool
	AccuracyCheck     bool
}

var (
	hallucinationCfg    *HallucinationConfig
	hallucinationCfgMu  sync.RWMutex
	hallucinationLoaded bool
)

// HallucinationConfigFromYAML 从 config.yaml 读取防幻觉配置
func HallucinationConfigFromYAML(ctx context.Context) (*HallucinationConfig, error) {
	hallucinationCfgMu.RLock()
	if hallucinationLoaded {
		cfg := hallucinationCfg
		hallucinationCfgMu.RUnlock()
		return cfg, nil
	}
	hallucinationCfgMu.RUnlock()

	hallucinationCfgMu.Lock()
	defer hallucinationCfgMu.Unlock()

	// Double-check after acquiring write lock
	if hallucinationLoaded {
		return hallucinationCfg, nil
	}

	hc := &HallucinationConfig{
		Enabled:                    true,
		ContractEnabled:            true,
		SchemaGateEnabled:          true,
		OutputValidationEnabled:    true,
		NoToolCallDetectionEnabled: true,
	}

	cfg := g.Cfg()

	if v, err := cfg.Get(ctx, "hallucination.enabled"); err == nil && !v.IsNil() {
		hc.Enabled = v.Bool()
	}
	if v, err := cfg.Get(ctx, "hallucination.contract_enabled"); err == nil && !v.IsNil() {
		hc.ContractEnabled = v.Bool()
	}
	if v, err := cfg.Get(ctx, "hallucination.contract_block_on_violation"); err == nil && !v.IsNil() {
		hc.ContractBlockOnViolation = v.Bool()
	}
	if v, err := cfg.Get(ctx, "hallucination.schema_gate_enabled"); err == nil && !v.IsNil() {
		hc.SchemaGateEnabled = v.Bool()
	}
	if v, err := cfg.Get(ctx, "hallucination.output_validation_enabled"); err == nil && !v.IsNil() {
		hc.OutputValidationEnabled = v.Bool()
	}
	if v, err := cfg.Get(ctx, "hallucination.no_tool_call_detection_enabled"); err == nil && !v.IsNil() {
		hc.NoToolCallDetectionEnabled = v.Bool()
	}

	if v, err := cfg.Get(ctx, "hallucination.ops_keywords"); err == nil && !v.IsNil() {
		hc.OpsKeywords = v.Strings()
	}
	if v, err := cfg.Get(ctx, "hallucination.hedge_words"); err == nil && !v.IsNil() {
		hc.HedgeWords = v.Strings()
	}
	if v, err := cfg.Get(ctx, "hallucination.problem_words"); err == nil && !v.IsNil() {
		hc.ProblemWords = v.Strings()
	}
	if v, err := cfg.Get(ctx, "hallucination.action_words"); err == nil && !v.IsNil() {
		hc.ActionWords = v.Strings()
	}
	if v, err := cfg.Get(ctx, "hallucination.contradictions"); err == nil && !v.IsNil() {
		raw := v.Interfaces()
		for _, pair := range raw {
			if arr, ok := pair.([]interface{}); ok && len(arr) == 2 {
				a, _ := arr[0].(string)
				b, _ := arr[1].(string)
				if a != "" && b != "" {
					hc.Contradictions = append(hc.Contradictions, [2]string{a, b})
				}
			}
		}
	}

	pattern := `(?i)(\d+(?:\.\d+)?)\s*(ms|秒|s|%)|P[0-9]+\s*[:：]\s*(\d+(?:\.\d+)?)`
	if v, err := cfg.Get(ctx, "hallucination.metric_pattern"); err == nil && !v.IsNil() && v.String() != "" {
		pattern = v.String()
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		// 不 panic，用默认正则
		compiled = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(ms|秒|s|%)|P[0-9]+\s*[:：]\s*(\d+(?:\.\d+)?)`)
	}
	hc.MetricPattern = compiled

	// LLM 校验配置
	llmVal := &LLMValidationConfig{
		Enabled:           false,
		Model:             "glm_chat_model_fast",
		TimeoutMs:         5000,
		MaxTokens:         512,
		OmissionDetection: true,
		AccuracyCheck:     true,
	}
	if v, err := cfg.Get(ctx, "hallucination.llm_validation.enabled"); err == nil && !v.IsNil() {
		llmVal.Enabled = v.Bool()
	}
	if v, err := cfg.Get(ctx, "hallucination.llm_validation.model"); err == nil && !v.IsNil() && v.String() != "" {
		llmVal.Model = v.String()
	}
	if v, err := cfg.Get(ctx, "hallucination.llm_validation.timeout_ms"); err == nil && !v.IsNil() {
		llmVal.TimeoutMs = v.Int()
	}
	if v, err := cfg.Get(ctx, "hallucination.llm_validation.max_tokens"); err == nil && !v.IsNil() {
		llmVal.MaxTokens = v.Int()
	}
	if v, err := cfg.Get(ctx, "hallucination.llm_validation.omission_detection"); err == nil && !v.IsNil() {
		llmVal.OmissionDetection = v.Bool()
	}
	if v, err := cfg.Get(ctx, "hallucination.llm_validation.accuracy_check"); err == nil && !v.IsNil() {
		llmVal.AccuracyCheck = v.Bool()
	}
	hc.LLMValidation = llmVal

	hallucinationCfg = hc
	hallucinationLoaded = true
	return hallucinationCfg, nil
}

// ResetHallucinationConfig 重置全局配置（仅用于测试）
func ResetHallucinationConfig() {
	hallucinationCfgMu.Lock()
	defer hallucinationCfgMu.Unlock()
	hallucinationCfg = nil
	hallucinationLoaded = false
}
