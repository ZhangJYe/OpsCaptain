package rag

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type AgentTrace struct {
	Rounds          int
	FinalConfidence float64
	TotalLatencyMs  int64
	RoundTraces     []RoundTrace
}

type RoundTrace struct {
	Round         int
	SubQueryCount int
	DocCount      int
	NewDocCount   int
	Confidence    float64
	Strategy      string
	LatencyMs     int64
}

type AgentRAG struct {
	evaluator *Evaluator
	cfg       AgentConfig
}

func NewAgentRAG(cfg AgentConfig) *AgentRAG {
	return &AgentRAG{
		evaluator: NewEvaluator(cfg.Model),
		cfg:       cfg,
	}
}

func (a *AgentRAG) Query(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, AgentTrace, error) {
	start := time.Now()
	trace := AgentTrace{}

	if !a.cfg.Enabled {
		docs, _, err := QueryWithPlanner(ctx, pool, query, LoadPlannerConfig(ctx))
		return docs, trace, err
	}

	totalTimeout := time.Duration(a.cfg.TotalTimeoutMs) * time.Millisecond
	agentCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	allDocs := make([]*schema.Document, 0)
	seenDocIDs := make(map[string]struct{})
	currentQuery := query
	plannerCfg := LoadPlannerConfig(ctx)

	for round := 0; round < a.cfg.MaxRounds; round++ {
		if agentCtx.Err() != nil {
			g.Log().Debugf(ctx, "agent rag: total timeout at round %d", round+1)
			break
		}

		roundTrace := RoundTrace{Round: round + 1}
		roundStart := time.Now()

		docs, merged, err := QueryWithPlanner(agentCtx, pool, currentQuery, plannerCfg)
		if err != nil {
			g.Log().Debugf(ctx, "agent rag: retrieval failed at round %d: %v", round+1, err)
			break
		}

		newDocs := filterNewDocs(docs, seenDocIDs)
		roundTrace.DocCount = len(docs)
		roundTrace.NewDocCount = len(newDocs)
		roundTrace.SubQueryCount = merged.Trace.SubQueryCount

		if len(newDocs) == 0 && round > 0 {
			g.Log().Debugf(ctx, "agent rag: no new docs at round %d, stopping", round+1)
			break
		}

		allDocs = append(allDocs, newDocs...)
		candidateDocs := mergeAndDedup(allDocs)

		evalResult := a.evaluator.Evaluate(agentCtx, query, candidateDocs, round+1, a.cfg.MaxRounds)
		roundTrace.Confidence = evalResult.Confidence
		roundTrace.Strategy = evalResult.NextStrategy
		roundTrace.LatencyMs = time.Since(roundStart).Milliseconds()

		trace.RoundTraces = append(trace.RoundTraces, roundTrace)
		trace.Rounds = round + 1
		trace.FinalConfidence = evalResult.Confidence

		g.Log().Debugf(ctx, "agent rag round %d: docs=%d new=%d confidence=%.2f sufficient=%v",
			round+1, len(docs), len(newDocs), evalResult.Confidence, evalResult.Sufficient)

		if evalResult.Sufficient || evalResult.Confidence >= a.cfg.ConfidenceThreshold {
			break
		}

		if round < a.cfg.MaxRounds-1 {
			nextQuery := buildNextQuery(query, evalResult)
			if nextQuery == "" {
				break
			}
			currentQuery = nextQuery
		}
	}

	finalDocs := mergeAndDedup(allDocs)
	trace.TotalLatencyMs = time.Since(start).Milliseconds()
	return finalDocs, trace, nil
}

func buildNextQuery(originalQuery string, evalResult EvalResult) string {
	if len(evalResult.MissingInfo) == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, originalQuery)
	for _, info := range evalResult.MissingInfo {
		info = strings.TrimSpace(info)
		if info != "" {
			parts = append(parts, info)
		}
	}
	return strings.Join(parts, " ")
}

func filterNewDocs(docs []*schema.Document, seen map[string]struct{}) []*schema.Document {
	var out []*schema.Document
	for _, doc := range docs {
		id := canonicalDocID(doc)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, doc)
	}
	return out
}

func mergeAndDedup(docs []*schema.Document) []*schema.Document {
	seen := make(map[string]*schema.Document, len(docs))
	order := make([]string, 0, len(docs))

	for _, doc := range docs {
		id := canonicalDocID(doc)
		if id == "" {
			id = doc.ID
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = doc
		order = append(order, id)
	}

	out := make([]*schema.Document, 0, len(order))
	for _, id := range order {
		out = append(out, seen[id])
	}
	return out
}
