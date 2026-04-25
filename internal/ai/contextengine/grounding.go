package contextengine

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

type RetrievalConfidence int

const (
	RetrievalConfidenceNone RetrievalConfidence = iota
	RetrievalConfidenceLow
	RetrievalConfidenceMedium
	RetrievalConfidenceHigh
)

func (c RetrievalConfidence) String() string {
	switch c {
	case RetrievalConfidenceHigh:
		return "high"
	case RetrievalConfidenceMedium:
		return "medium"
	case RetrievalConfidenceLow:
		return "low"
	default:
		return "none"
	}
}

type GroundingSignal struct {
	Confidence    RetrievalConfidence
	TopScore      float64
	AvgScore      float64
	DocCount      int
	HasExactMatch bool
}

func EvaluateGrounding(items []ContextItem) GroundingSignal {
	sig := GroundingSignal{}
	if len(items) == 0 {
		return sig
	}
	sig.DocCount = len(items)
	total := 0.0
	for _, item := range items {
		if item.Score > sig.TopScore {
			sig.TopScore = item.Score
		}
		total += item.Score
	}
	sig.AvgScore = total / float64(len(items))

	highThreshold := groundingThreshold("rag.grounding_high_threshold", 0.75)
	lowThreshold := groundingThreshold("rag.grounding_low_threshold", 0.45)

	switch {
	case sig.TopScore >= highThreshold && sig.DocCount >= 2:
		sig.Confidence = RetrievalConfidenceHigh
	case sig.TopScore >= lowThreshold:
		sig.Confidence = RetrievalConfidenceMedium
	case sig.DocCount > 0:
		sig.Confidence = RetrievalConfidenceLow
	default:
		sig.Confidence = RetrievalConfidenceNone
	}
	return sig
}

func GroundingPreamble(sig GroundingSignal) string {
	switch sig.Confidence {
	case RetrievalConfidenceHigh:
		return ""
	case RetrievalConfidenceMedium:
		return "⚠️ 以下回答基于有限的知识库匹配结果，部分内容可能不完整，请结合实际情况判断。\n\n"
	case RetrievalConfidenceLow:
		return "⚠️ 知识库中相关信息较少且匹配度较低，以下回答仅供参考，建议结合实际日志和监控数据进一步确认。\n\n"
	default:
		return "⚠️ 当前知识库中未找到与该问题相关的信息，无法给出基于文档的回答。以下仅为基于通用知识的推测，请谨慎参考。\n\n"
	}
}

func GroundingPromptDirective(sig GroundingSignal) string {
	switch sig.Confidence {
	case RetrievalConfidenceHigh:
		return `
## 回答约束（文档置信度：高）
- 你的回答必须严格基于上方提供的文档内容
- 对于每个事实性陈述，必须使用 [编号] 标注来源文档，例如"根据 [1]，该服务的 RRT 超过阈值"
- 如果文档中没有直接支持某个结论，必须明确标注"基于通用知识"或"文档未涉及此部分"
- 不要编造文档中不存在的数据、指标值、服务名、配置项或日志内容
- 如果文档内容相互矛盾，请指出矛盾而不是选择性引用
`
	case RetrievalConfidenceMedium:
		return `
## 回答约束（文档置信度：中）
- 优先使用上方提供的文档内容回答，使用 [编号] 标注来源
- 文档匹配度有限，允许结合通用运维知识补充，但必须明确区分哪些是"文档所述"，哪些是"通用知识补充"
- 不要编造不存在的数据、指标值、服务名或日志内容
- 对于不确定的部分，使用"可能"、"建议进一步确认"等措辞
`
	case RetrievalConfidenceLow:
		return `
## 回答约束（文档置信度：低）
- 检索到的文档与问题相关度较低，不要强行引用不相关的文档内容
- 可以基于通用运维知识回答，但必须在开头说明"当前知识库中未找到高度相关的信息"
- 不要编造任何具体的数据、指标值、服务名、pod名、配置项或日志片段
- 建议用户提供更多上下文信息，或指出可以查看哪些监控/日志来获取更准确的判断
`
	default:
		return `
## 回答约束（无文档支持）
- 当前没有检索到相关文档
- 你可以基于通用运维知识给出方向性建议，但必须在开头明确说明"知识库中未收录相关内容"
- 严禁编造任何具体的数据、指标值、服务名称、pod名称、配置项、日志内容或历史案例
- 如果问题涉及具体的业务系统状态，必须明确告知用户需要查看实际监控和日志
- 优先给出排查思路和方向，而不是具体的结论
`
	}
}

func DocumentsContentWithCitation(pkg *ContextPackage) string {
	if pkg == nil || len(pkg.DocumentItems) == 0 {
		return "（当前未检索到相关文档）"
	}
	parts := make([]string, 0, len(pkg.DocumentItems))
	for idx, item := range pkg.DocumentItems {
		header := fmt.Sprintf("[%d] 来源: %s (相关度: %.2f)", idx+1, item.Title, item.Score)
		parts = append(parts, header+"\n"+item.Content)
	}
	return strings.Join(parts, "\n\n")
}

func groundingThreshold(key string, fallback float64) float64 {
	v, err := g.Cfg().Get(context.Background(), key)
	if err == nil && v.Float64() > 0 {
		return v.Float64()
	}
	return fallback
}
