package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type EvalResult struct {
	Confidence   float64
	Sufficient   bool
	MissingInfo  []string
	NextStrategy string
	Reason       string
}

type Evaluator struct {
	model string
}

func NewEvaluator(model string) *Evaluator {
	return &Evaluator{model: model}
}

const evaluatorPrompt = `你是运维知识检索质量评估器。给定用户问题和检索结果，判断是否足够回答。

用户问题: %s
已检索文档:
%s
已尝试轮数: %d/%d

评估维度:
1. 是否覆盖了问题的所有方面？
2. 结果与问题的相关性如何？
3. 是否有明确的证据支持结论？

输出 JSON:
{
  "confidence": 0.0-1.0,
  "sufficient": true/false,
  "missing_info": ["缺失点1"],
  "next_strategy": "expand_scope|refine_query|add_angle|none",
  "reason": "评估理由"
}`

func (e *Evaluator) Evaluate(ctx context.Context, query string, docs []*schema.Document, round, maxRounds int) EvalResult {
	evalCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(evalCtx)
	if err != nil {
		g.Log().Debugf(ctx, "evaluator model init failed: %v", err)
		return EvalResult{Confidence: 0.5, Sufficient: false, NextStrategy: "none", Reason: "model init failed"}
	}

	docSummaries := formatDocsForEval(docs)
	prompt := fmt.Sprintf(evaluatorPrompt, query, docSummaries, round, maxRounds)

	resp, err := chatModel.Generate(evalCtx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		g.Log().Debugf(ctx, "evaluator LLM call failed: %v", err)
		return EvalResult{Confidence: 0.5, Sufficient: false, NextStrategy: "none", Reason: "LLM call failed"}
	}

	result := parseEvalResult(resp.Content)
	return result
}

func formatDocsForEval(docs []*schema.Document) string {
	var sb strings.Builder
	for i, doc := range docs {
		if doc == nil {
			continue
		}
		title := ""
		if doc.MetaData != nil {
			for _, key := range []string{"title", "file_name", "source"} {
				if v, ok := doc.MetaData[key].(string); ok && v != "" {
					title = v
					break
				}
			}
		}
		if title == "" {
			title = doc.ID
		}
		content := doc.Content
		runes := []rune(content)
		if len(runes) > 200 {
			content = string(runes[:200]) + "..."
		}
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i+1, title, content)
	}
	return sb.String()
}

func canonicalDocID(doc *schema.Document) string {
	if doc == nil {
		return ""
	}
	if doc.MetaData != nil {
		for _, key := range []string{"case_id", "caseid", "doc_id"} {
			if v, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return strings.TrimSpace(doc.ID)
}

func parseEvalResult(raw string) EvalResult {
	type evalResponse struct {
		Confidence   float64  `json:"confidence"`
		Sufficient   bool     `json:"sufficient"`
		MissingInfo  []string `json:"missing_info"`
		NextStrategy string   `json:"next_strategy"`
		Reason       string   `json:"reason"`
	}

	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var r evalResponse
	if err := json.Unmarshal([]byte(cleaned), &r); err != nil {
		g.Log().Debugf(context.Background(), "evaluator parse failed: %v, raw=%q", err, raw)
		return EvalResult{Confidence: 0.5, Sufficient: false, NextStrategy: "none", Reason: "parse failed"}
	}

	if r.Confidence < 0 || r.Confidence > 1 {
		r.Confidence = 0.5
	}
	if r.NextStrategy == "" {
		r.NextStrategy = "none"
	}

	return EvalResult{
		Confidence:   r.Confidence,
		Sufficient:   r.Sufficient,
		MissingInfo:  r.MissingInfo,
		NextStrategy: r.NextStrategy,
		Reason:       r.Reason,
	}
}
