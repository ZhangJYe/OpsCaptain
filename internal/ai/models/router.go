package models

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gogf/gf/v2/frame/g"
)

type QueryComplexity string

const (
	ComplexitySimple  QueryComplexity = "simple"
	ComplexityMedium  QueryComplexity = "medium"
	ComplexityComplex QueryComplexity = "complex"
)

type ModelRouter struct {
	fastModel string
	proModel  string
	threshold float64
}

func NewModelRouter(fastModel, proModel string, threshold float64) *ModelRouter {
	if threshold <= 0 || threshold > 1 {
		threshold = 0.6
	}
	return &ModelRouter{
		fastModel: fastModel,
		proModel:  proModel,
		threshold: threshold,
	}
}

func (r *ModelRouter) Route(_ context.Context, query string) (model string, complexity QueryComplexity) {
	score := EstimateComplexity(query)
	if score >= r.threshold {
		return r.proModel, ComplexityComplex
	}
	if score >= r.threshold*0.5 {
		return r.fastModel, ComplexityMedium
	}
	return r.fastModel, ComplexitySimple
}

var (
	complexKeywords   = regexp.MustCompile(`(?i)(分析|排查|诊断|计划|analyze|diagnose|plan|root.?cause|incident|故障|告警|alert|为什么|怎么回事|如何|怎么解决|优化|建议|评估|review)`)
	simpleKeywords    = regexp.MustCompile(`(?i)^(hi|hello|hey|你好|谢谢|再见|bye|ok|好的|是|不是|嗯|哦|啊)\s*[!.！？\s?]*$`)
	opsServicePattern = regexp.MustCompile(`(?i)(服务|service|pod|容器|container|deploy|部署|rollout|node|节点|server)`)
	opsMetricPattern  = regexp.MustCompile(`(?i)(指标|metric|cpu|memory|内存|磁盘|disk|延迟|latency|错误率|error.?rate|超时|timeout|qps|tps|流量|带宽|bandwidth)`)
)

func EstimateComplexity(query string) float64 {
	query = strings.TrimSpace(query)
	length := utf8.RuneCountInString(query)

	if length == 0 {
		return 0
	}

	if simpleKeywords.MatchString(query) {
		return 0.1
	}

	score := 0.0

	if length < 20 {
		score += 0.1
	} else if length < 60 {
		score += 0.3
	} else if length < 100 {
		score += 0.5
	} else {
		score += 0.7
	}

	if complexKeywords.MatchString(query) {
		score += 0.3
	}

	serviceMatches := opsServicePattern.FindAllString(query, -1)
	if len(serviceMatches) >= 2 {
		score += 0.2
	} else if len(serviceMatches) >= 1 {
		score += 0.1
	}

	metricMatches := opsMetricPattern.FindAllString(query, -1)
	if len(metricMatches) >= 2 {
		score += 0.15
	} else if len(metricMatches) >= 1 {
		score += 0.05
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

func NewModelRouterFromConfig(ctx context.Context) *ModelRouter {
	fastKey, err := g.Cfg().Get(ctx, "model_routing.fast_model")
	if err != nil || fastKey.String() == "" {
		fastKey = g.NewVar("chat_model_fast")
	}
	proKey, err := g.Cfg().Get(ctx, "model_routing.pro_model")
	if err != nil || proKey.String() == "" {
		proKey = g.NewVar("chat_model")
	}
	thresholdVal, err := g.Cfg().Get(ctx, "model_routing.complexity_threshold")
	if err != nil || thresholdVal.Float64() == 0 {
		thresholdVal = g.NewVar(0.6)
	}
	enabled, err := g.Cfg().Get(ctx, "model_routing.enabled")
	if err != nil || !enabled.Bool() {
		return NewModelRouter("chat_model_fast", "chat_model", 0.6)
	}
	return NewModelRouter(fastKey.String(), proKey.String(), thresholdVal.Float64())
}
