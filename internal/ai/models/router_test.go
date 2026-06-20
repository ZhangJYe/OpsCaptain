package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEstimateComplexity_SimpleQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"greeting_en", "hello"},
		{"greeting_cn", "你好"},
		{"thanks", "谢谢"},
		{"short_word", "ok"},
		{"single_char", "好"},
		{"short_question", "hi"},
		{"bye", "再见"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := EstimateComplexity(tt.query)
			assert.Less(t, score, 0.4, "simple query should score < 0.4, query=%q score=%f", tt.query, score)
		})
	}
}

func TestEstimateComplexity_ComplexQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"analysis", "请分析一下订单服务和支付服务最近的CPU使用率和错误率趋势"},
		{"diagnosis", "排查为什么 order-service 和 payment-service 出现了超时告警，需要诊断根因"},
		{"plan", "请制定一个关于订单服务和支付服务的优化计划，包括指标分析和故障排查"},
		{"incident", "线上告警：订单服务 P99 延迟从 200ms 飙升到 2s，错误率从 0.1% 升到 5%，请分析原因并给出修复建议"},
		{"multi_service", "比较 order-service、payment-service、user-service 三个服务的 QPS 和错误率，看看哪个有问题"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := EstimateComplexity(tt.query)
			assert.Greater(t, score, 0.6, "complex query should score > 0.6, query=%q score=%f", tt.query, score)
		})
	}
}

func TestRoute_SimpleQuery(t *testing.T) {
	router := NewModelRouter("chat_model_fast", "chat_model", 0.6)
	ctx := context.Background()

	model, complexity := router.Route(ctx, "你好")
	assert.Equal(t, "chat_model_fast", model)
	assert.Equal(t, ComplexitySimple, complexity)
}

func TestRoute_ComplexQuery(t *testing.T) {
	router := NewModelRouter("chat_model_fast", "chat_model", 0.6)
	ctx := context.Background()

	model, complexity := router.Route(ctx, "请分析一下订单服务和支付服务最近的CPU使用率和错误率趋势，并排查原因")
	assert.Equal(t, "chat_model", model)
	assert.Equal(t, ComplexityComplex, complexity)
}

func TestRoute_MediumQuery(t *testing.T) {
	router := NewModelRouter("chat_model_fast", "chat_model", 0.6)
	ctx := context.Background()

	model, complexity := router.Route(ctx, "看看 order-service 的部署状态")
	assert.Equal(t, "chat_model_fast", model)
	assert.Equal(t, ComplexityMedium, complexity)
}

func TestEstimateComplexity_EmptyQuery(t *testing.T) {
	score := EstimateComplexity("")
	assert.Equal(t, 0.0, score)
}

func TestEstimateComplexity_SingleWord(t *testing.T) {
	score := EstimateComplexity("order-service")
	assert.Less(t, score, 0.6, "single service name should not be complex")
}

func TestNewModelRouter_DefaultThreshold(t *testing.T) {
	r := NewModelRouter("fast", "pro", -1)
	assert.Equal(t, 0.6, r.threshold)

	r2 := NewModelRouter("fast", "pro", 1.5)
	assert.Equal(t, 0.6, r2.threshold)
}

func TestEstimateComplexity_KeywordBoost(t *testing.T) {
	basic := EstimateComplexity("帮我看看这个东西")
	withKeyword := EstimateComplexity("帮我分析一下这个东西")
	assert.Greater(t, withKeyword, basic, "complex keyword should boost score")
}

func TestEstimateComplexity_MultiServiceBoost(t *testing.T) {
	single := EstimateComplexity("看看 order-service 的指标")
	multi := EstimateComplexity("看看 order-service 和 payment-service 的指标")
	assert.Greater(t, multi, single, "multiple service mentions should boost score")
}
