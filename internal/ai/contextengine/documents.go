package contextengine

import (
	"SuperBizAgent/internal/ai/rag"
	"context"
	"fmt"
	"strings"
	"time"

	ragretriever "SuperBizAgent/internal/ai/retriever"
	"SuperBizAgent/utility/mem"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

const defaultContextDocsQueryTimeout = 5 * time.Second
const defaultContextDocsInitFailureTTL = 15 * time.Second

type documentSelectionResult struct {
	selected []ContextItem
	dropped  []ContextItem
	used     int
	notes    []string
	metrics  *RetrievalStageMetrics
}

var (
	newContextRetriever  = ragretriever.NewMilvusRetriever
	contextRetrieverPool = rag.NewRetrieverPool(
		func(ctx context.Context) (retrieverapi.Retriever, error) {
			return newContextRetriever(ctx)
		},
		rag.DefaultRetrieverCacheKey,
		contextDocsInitFailureTTL,
	)
)

func getOrCreateContextRetriever(ctx context.Context) (retrieverapi.Retriever, error) {
	rr, _, err := contextRetrieverPool.GetOrCreate(ctx)
	return rr, err
}

func resetContextRetrieverCache() {
	contextRetrieverPool.Reset()
}

func contextDocsQueryTimeout(ctx context.Context) time.Duration {
	return rag.DurationFromConfig(ctx, defaultContextDocsQueryTimeout, "context.docs_query_timeout_ms", "multi_agent.knowledge_query_timeout_ms")
}

func contextDocsInitFailureTTL(ctx context.Context) time.Duration {
	return rag.DurationFromConfig(ctx, defaultContextDocsInitFailureTTL, "context.docs_init_failure_ttl_ms", "multi_agent.knowledge_init_failure_ttl_ms")
}

func selectDocuments(ctx context.Context, query string, profile ContextProfile) documentSelectionResult {
	if strings.TrimSpace(query) == "" || !profile.AllowDocs || profile.Budget.DocumentTokens == 0 {
		return documentSelectionResult{notes: []string{"documents disabled"}}
	}

	queryCtx, cancel := context.WithTimeout(ctx, contextDocsQueryTimeout(ctx))
	defer cancel()

	docs, trace, err := rag.Query(queryCtx, contextRetrieverPool, query)
	metrics := retrievalMetricsFromQueryTrace(trace)
	if err != nil {
		return documentSelectionResult{
			notes:   []string{fmt.Sprintf("documents unavailable: %v", err), formatRetrievalTraceNote(metrics)},
			metrics: metrics,
		}
	}
	if len(docs) == 0 {
		return documentSelectionResult{
			notes:   []string{"documents empty", formatRetrievalTraceNote(metrics)},
			metrics: metrics,
		}
	}

	remaining := profile.Budget.DocumentTokens
	selected := make([]ContextItem, 0, len(docs))
	dropped := make([]ContextItem, 0)
	used := 0
	for idx, doc := range docs {
		item := newDocumentItem(doc, idx)
		if item.TokenEstimate > remaining {
			trimmed := mem.TrimToTokenBudget(item.Content, remaining)
			if strings.TrimSpace(trimmed) == "" {
				item.DroppedReason = "document_budget"
				dropped = append(dropped, item)
				continue
			}
			item.Content = trimmed
			item.TokenEstimate = mem.EstimateTokens(trimmed)
			item.CompressionLevel = "trimmed"
		}
		item.Selected = true
		selected = append(selected, item)
		remaining -= item.TokenEstimate
		used += item.TokenEstimate
		if remaining <= 0 {
			for j := idx + 1; j < len(docs); j++ {
				dropped = append(dropped, newDroppedDocumentItem(docs[j], j, "document_budget"))
			}
			break
		}
	}

	return documentSelectionResult{
		selected: selected,
		dropped:  dropped,
		used:     used,
		notes: []string{
			fmt.Sprintf("tokens=%d/%d", used, profile.Budget.DocumentTokens),
			formatRetrievalTraceNote(metrics),
		},
		metrics: metrics,
	}
}

func DocumentsContent(pkg *ContextPackage) string {
	if pkg == nil || len(pkg.DocumentItems) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pkg.DocumentItems))
	for idx, item := range pkg.DocumentItems {
		parts = append(parts, fmt.Sprintf("[%d] %s\n%s", idx+1, item.Title, item.Content))
	}
	return strings.Join(parts, "\n\n")
}

func newDocumentItem(doc *schema.Document, idx int) ContextItem {
	title := fmt.Sprintf("document-%d", idx+1)
	sourceID := title
	content := ""
	score := 0.0
	if doc != nil {
		if doc.ID != "" {
			sourceID = doc.ID
		}
		content = doc.Content
		score = doc.Score()
		if doc.MetaData != nil {
			for _, key := range []string{"title", "file_name", "filename", "source"} {
				if value, ok := doc.MetaData[key].(string); ok && value != "" {
					title = value
					break
				}
			}
		}
	}
	return ContextItem{
		ID:            sourceID,
		SourceType:    "doc",
		SourceID:      sourceID,
		Title:         title,
		Content:       content,
		Score:         score,
		TrustLevel:    "retrieved",
		TokenEstimate: mem.EstimateTokens(content),
		SafetyLabel:   "retrieved_doc",
		UpdatePolicy:  "refresh_on_retrieval",
	}
}

func newDroppedDocumentItem(doc *schema.Document, idx int, reason string) ContextItem {
	item := newDocumentItem(doc, idx)
	item.DroppedReason = reason
	return item
}

func retrievalMetricsFromQueryTrace(trace rag.QueryTrace) *RetrievalStageMetrics {
	if trace.CacheKey == "" &&
		!trace.CacheHit &&
		!trace.InitFailureCached &&
		trace.InitLatencyMs == 0 &&
		trace.RetrieveLatencyMs == 0 &&
		trace.ResultCount == 0 {
		return nil
	}
	return &RetrievalStageMetrics{
		CacheKey:          trace.CacheKey,
		CacheHit:          trace.CacheHit,
		InitFailureCached: trace.InitFailureCached,
		InitLatencyMs:     trace.InitLatencyMs,
		RetrieveLatencyMs: trace.RetrieveLatencyMs,
		ResultCount:       trace.ResultCount,
	}
}

func formatRetrievalTraceNote(metrics *RetrievalStageMetrics) string {
	if metrics == nil {
		return "retrieval_trace unavailable"
	}
	return fmt.Sprintf(
		"retrieval cache_hit=%t init_cached_error=%t init_ms=%d retrieve_ms=%d hits=%d",
		metrics.CacheHit,
		metrics.InitFailureCached,
		metrics.InitLatencyMs,
		metrics.RetrieveLatencyMs,
		metrics.ResultCount,
	)
}
