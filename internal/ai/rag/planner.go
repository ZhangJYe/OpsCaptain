package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/models"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type PlanResult struct {
	Decomposed bool
	SubQueries []string
	Reason     string
	LatencyMs  int64
}

type PlannerTrace struct {
	Analyzed       bool
	SubQueryCount  int
	PlanLatencyMs  int64
	ExecLatencyMs  int64
	MergeLatencyMs int64
	FallbackReason string
}

type MergedResult struct {
	Docs  []*schema.Document
	Trace PlannerTrace
}

func needsDecomposition(query string, cfg PlannerConfig) bool {
	if len([]rune(query)) < cfg.MinQueryLength {
		return false
	}
	for _, kw := range cfg.DecompositionKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}
	return false
}

const plannerDecomposePrompt = `你是运维查询分析器。将用户问题拆解为独立的检索子查询。

规则：
1. 每个子查询聚焦一个具体信息点
2. 最多 %d 个子查询
3. 保留原始 query 的语义关键词
4. 如果问题已经是单一主题，返回空数组 []
5. 只输出 JSON 数组，不要其他文字

示例：
用户: "payment 服务最近为什么延迟升高，跟 Redis 有没有关系？"
输出: ["payment 服务延迟升高指标", "payment 服务错误日志", "Redis 性能异常", "payment Redis 连接"]

用户: "Prometheus 告警先看什么"
输出: []

用户: %s
输出:`

func decomposeQuery(ctx context.Context, query string, cfg PlannerConfig) ([]string, error) {
	planCtx, cancel := context.WithTimeout(ctx, plannerTimeout(ctx, cfg))
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(planCtx)
	if err != nil {
		return nil, fmt.Errorf("planner model init failed: %w", err)
	}

	prompt := fmt.Sprintf(plannerDecomposePrompt, cfg.MaxSubQueries, query)
	resp, err := chatModel.Generate(planCtx, []*schema.Message{
		{Role: schema.User, Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("planner LLM call failed: %w", err)
	}

	var subQueries []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(resp.Content)), &subQueries); err != nil {
		return nil, fmt.Errorf("planner parse failed: %w", err)
	}

	seen := make(map[string]struct{})
	unique := make([]string, 0, len(subQueries))
	for _, q := range subQueries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if _, ok := seen[q]; ok {
			continue
		}
		seen[q] = struct{}{}
		unique = append(unique, q)
	}

	if len(unique) < 2 {
		return nil, nil
	}
	if len(unique) > cfg.MaxSubQueries {
		unique = unique[:cfg.MaxSubQueries]
	}
	return unique, nil
}

func Analyze(ctx context.Context, query string, cfg PlannerConfig) PlanResult {
	start := time.Now()
	result := PlanResult{}

	if !cfg.Enabled {
		result.Reason = "planner_disabled"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	if !needsDecomposition(query, cfg) {
		result.Reason = "no_decomposition_needed"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	subQueries, err := decomposeQuery(ctx, query, cfg)
	if err != nil {
		g.Log().Debugf(ctx, "query planner decompose failed: %v", err)
		result.Reason = fmt.Sprintf("decompose_error: %v", err)
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	if len(subQueries) == 0 {
		result.Reason = "decompose_returned_empty"
		result.LatencyMs = time.Since(start).Milliseconds()
		return result
	}

	result.Decomposed = true
	result.SubQueries = subQueries
	result.Reason = "decomposed"
	result.LatencyMs = time.Since(start).Milliseconds()
	return result
}

func Execute(ctx context.Context, subQueries []string, cfg PlannerConfig) ([][]*schema.Document, int64) {
	start := time.Now()
	results := make([][]*schema.Document, len(subQueries))
	var wg sync.WaitGroup

	for i, q := range subQueries {
		wg.Add(1)
		go func(idx int, query string) {
			defer wg.Done()
			queryCtx, cancel := context.WithTimeout(ctx, plannerExecTimeout(ctx, cfg))
			defer cancel()
			docs, _, err := Query(queryCtx, SharedPool(), query)
			if err != nil {
				g.Log().Debugf(ctx, "planner sub-query %d failed: %v", idx, err)
				return
			}
			results[idx] = docs
		}(i, q)
	}
	wg.Wait()

	return results, time.Since(start).Milliseconds()
}

func MergeResults(subQueryResults [][]*schema.Document, subQueries []string, finalTopK int) ([]*schema.Document, map[string][]string) {
	const k = 60.0

	type entry struct {
		doc        *schema.Document
		score      float64
		subQueries []string
	}
	byID := make(map[string]*entry)

	for i, docs := range subQueryResults {
		if i >= len(subQueries) {
			break
		}
		for rank, doc := range docs {
			if doc == nil {
				continue
			}
			id := docFusionKey(doc)
			if id == "" {
				id = doc.ID
			}
			if id == "" {
				continue
			}
			e, ok := byID[id]
			if !ok {
				e = &entry{doc: doc}
				byID[id] = e
			}
			e.score += 1.0 / (k + float64(rank+1))
			found := false
			for _, sq := range e.subQueries {
				if sq == subQueries[i] {
					found = true
					break
				}
			}
			if !found {
				e.subQueries = append(e.subQueries, subQueries[i])
			}
		}
	}

	entries := make([]*entry, 0, len(byID))
	for _, e := range byID {
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	limit := finalTopK
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	docs := make([]*schema.Document, 0, limit)
	subQueryMap := make(map[string][]string)
	for _, e := range entries[:limit] {
		docs = append(docs, e.doc)
		id := docFusionKey(e.doc)
		if id == "" {
			id = e.doc.ID
		}
		subQueryMap[id] = e.subQueries
	}

	return docs, subQueryMap
}

func QueryWithPlanner(ctx context.Context, pool *RetrieverPool, query string, cfg PlannerConfig) ([]*schema.Document, MergedResult, error) {
	trace := PlannerTrace{Analyzed: true}

	if strings.TrimSpace(query) == "" {
		return nil, MergedResult{Trace: trace}, nil
	}

	plan := Analyze(ctx, query, cfg)
	trace.PlanLatencyMs = plan.LatencyMs

	if !plan.Decomposed {
		docs, _, err := Query(ctx, pool, query)
		trace.FallbackReason = plan.Reason
		return docs, MergedResult{Trace: trace}, err
	}

	trace.SubQueryCount = len(plan.SubQueries)

	subResults, execLatency := Execute(ctx, plan.SubQueries, cfg)
	trace.ExecLatencyMs = execLatency

	mergeStart := time.Now()
	docs, subQueryMap := MergeResults(subResults, plan.SubQueries, RetrieverTopK(ctx))
	trace.MergeLatencyMs = time.Since(mergeStart).Milliseconds()

	for _, doc := range docs {
		id := docFusionKey(doc)
		if id == "" {
			id = doc.ID
		}
		if sqs, ok := subQueryMap[id]; ok {
			if doc.MetaData == nil {
				doc.MetaData = make(map[string]any)
			}
			doc.MetaData["_planner_sub_queries"] = sqs
		}
	}

	return docs, MergedResult{Trace: trace}, nil
}
