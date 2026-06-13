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

type RetrievalPlan struct {
	SubQueries []string
	Strategy   string
	Reason     string
}

type RetrievalPlanner struct {
	model string
}

func NewRetrievalPlanner(model string) *RetrievalPlanner {
	return &RetrievalPlanner{model: model}
}

const retrievalPlannerPrompt = `你是运维检索策略规划器。根据评估结果和已检索文档，规划下一轮检索。

用户问题: %s
评估结果:
- 置信度: %.2f
- 是否充分: %v
- 缺失信息: %s
- 建议策略: %s

已检索文档 ID: %s

规则:
1. 针对 missing_info 制定检索子查询
2. 最多 3 个子查询
3. 不要重复已检索的文档 ID（程序会自动过滤，但请尽量避免）

输出 JSON:
{
  "sub_queries": ["子查询1", "子查询2"],
  "strategy": "expand_scope|refine_query|add_angle",
  "reason": "规划理由"
}`

func (rp *RetrievalPlanner) Plan(ctx context.Context, query string, evalResult EvalResult, candidateDocs []*schema.Document) RetrievalPlan {
	planCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(planCtx)
	if err != nil {
		g.Log().Debugf(ctx, "retrieval planner model init failed: %v", err)
		return RetrievalPlan{Strategy: "none", Reason: "model init failed"}
	}

	missingInfoStr := "无"
	if len(evalResult.MissingInfo) > 0 {
		missingInfoStr = strings.Join(evalResult.MissingInfo, ", ")
	}

	docIDs := extractDocIDs(candidateDocs)
	prompt := fmt.Sprintf(retrievalPlannerPrompt,
		query, evalResult.Confidence, evalResult.Sufficient,
		missingInfoStr, evalResult.NextStrategy, docIDs)

	resp, err := chatModel.Generate(planCtx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		g.Log().Debugf(ctx, "retrieval planner LLM call failed: %v", err)
		return RetrievalPlan{Strategy: "none", Reason: "LLM call failed"}
	}

	return parseRetrievalPlan(resp.Content)
}

func extractDocIDs(docs []*schema.Document) string {
	ids := make([]string, 0, len(docs))
	seen := make(map[string]struct{})
	for _, doc := range docs {
		id := canonicalDocID(doc)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return strings.Join(ids, ", ")
}

func parseRetrievalPlan(raw string) RetrievalPlan {
	type planResponse struct {
		SubQueries []string `json:"sub_queries"`
		Strategy   string   `json:"strategy"`
		Reason     string   `json:"reason"`
	}

	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var r planResponse
	if err := json.Unmarshal([]byte(cleaned), &r); err != nil {
		g.Log().Debugf(context.Background(), "retrieval planner parse failed: %v, raw=%q", err, raw)
		return RetrievalPlan{Strategy: "none", Reason: "parse failed"}
	}

	if r.Strategy == "" {
		r.Strategy = "none"
	}

	return RetrievalPlan{
		SubQueries: r.SubQueries,
		Strategy:   r.Strategy,
		Reason:     r.Reason,
	}
}
