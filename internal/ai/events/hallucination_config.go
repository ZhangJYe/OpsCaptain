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
	SchemaGateEnabled          bool
	OutputValidationEnabled    bool
	NoToolCallDetectionEnabled bool
	OpsKeywords                []string
	HedgeWords                 []string
	ProblemWords               []string
	ActionWords                []string
	Contradictions             [][2]string
	MetricPattern              *regexp.Regexp
}

var (
	hallucinationCfg    *HallucinationConfig
	hallucinationCfgMu  sync.Once
	hallucinationCfgErr error
)

// HallucinationConfigFromYAML 从 config.yaml 读取防幻觉配置
func HallucinationConfigFromYAML(ctx context.Context) (*HallucinationConfig, error) {
	hallucinationCfgMu.Do(func() {
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
		hc.MetricPattern = regexp.MustCompile(pattern)

		hallucinationCfg = hc
	})

	return hallucinationCfg, hallucinationCfgErr
}

// ResetHallucinationConfig 重置全局配置（仅用于测试）
func ResetHallucinationConfig() {
	hallucinationCfgMu = sync.Once{}
	hallucinationCfg = nil
	hallucinationCfgErr = nil
}
