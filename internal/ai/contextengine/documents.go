package contextengine

import (
	"SuperBizAgent/internal/ai/contextcompression"
	"SuperBizAgent/internal/ai/rag"
	"context"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/memory"

	"github.com/cloudwego/eino/schema"
)

const defaultContextDocsQueryTimeout = 5 * time.Second

type documentSelectionResult struct {
	selected           []ContextItem
	dropped            []ContextItem
	used               int
	notes              []string
	metrics            *RetrievalStageMetrics
	compressionReports []contextcompression.Report
}

func contextDocsQueryTimeout(ctx context.Context) time.Duration {
	return rag.DurationFromConfig(ctx, defaultContextDocsQueryTimeout, "context.docs_query_timeout_ms", "multi_agent.knowledge_query_timeout_ms")
}

func selectDocuments(ctx context.Context, query string, profile ContextProfile) documentSelectionResult {
	if strings.TrimSpace(query) == "" || !profile.AllowDocs || profile.Budget.DocumentTokens == 0 {
		return documentSelectionResult{notes: []string{"documents disabled"}}
	}

	queryCtx, cancel := context.WithTimeout(ctx, contextDocsQueryTimeout(ctx))
	defer cancel()

	docs, trace, err := rag.Query(queryCtx, rag.SharedPool(), query)
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

	// 加载压缩配置
	compCfg := contextcompression.LoadConfig(ctx)
	var compressionReports []contextcompression.Report

	for idx, doc := range docs {
		item := newDocumentItem(doc, idx)
		if compCfg.Enabled && compCfg.Mode != contextcompression.ModeOff {
			compResult := contextcompression.Compress(ctx, contextcompression.Request{
				SourceType: contextcompression.SourceRAG,
				SourceID:   item.SourceID,
				Query:      query,
				Content:    item.Content,
			}, compCfg)
			if shouldRecordCompressionReport(compResult.Report) {
				compressionReports = append(compressionReports, compResult.Report)
			}
			if compCfg.Mode == contextcompression.ModeOptimize &&
				!compResult.Report.Degraded &&
				compResult.Report.CompressionRatio < 1.0 {
				item.Content = compResult.Content
				item.TokenEstimate = memory.EstimateTokens(item.Content)
				item.CompressionLevel = "compressed"
			} else if compCfg.Mode == contextcompression.ModeAudit && shouldRecordCompressionReport(compResult.Report) {
				item.CompressionLevel = "audit"
			}
		}
		if item.TokenEstimate > remaining {
			trimmed := memory.TrimToTokenBudget(item.Content, remaining)
			if strings.TrimSpace(trimmed) == "" {
				item.DroppedReason = "document_budget"
				dropped = append(dropped, item)
				continue
			}
			item.Content = trimmed
			item.TokenEstimate = memory.EstimateTokens(trimmed)
			if item.CompressionLevel == "" || item.CompressionLevel == "audit" {
				item.CompressionLevel = "trimmed"
			}
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
		metrics:            metrics,
		compressionReports: compressionReports,
	}
}

func DocumentsContent(pkg *ContextPackage) string {
	if pkg == nil || len(pkg.DocumentItems) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pkg.DocumentItems))
	for idx, item := range pkg.DocumentItems {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("[ctx-doc-%d] title: %s", idx+1, item.Title))
		if item.SourceID != "" && item.SourceID != item.Title {
			sb.WriteString(fmt.Sprintf("\nsource: %s", item.SourceID))
		}
		if item.Score > 0 {
			sb.WriteString(fmt.Sprintf("\nscore: %.2f", item.Score))
		}
		sb.WriteString(fmt.Sprintf("\ncontent:\n%s", item.Content))
		parts = append(parts, sb.String())
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
		TokenEstimate: memory.EstimateTokens(content),
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
		RewriteLatencyMs:  trace.RewriteLatencyMs,
		RerankLatencyMs:   trace.RerankLatencyMs,
		OriginalQuery:     trace.OriginalQuery,
		RewrittenQuery:    trace.RewrittenQuery,
		RawResultCount:    trace.RawResultCount,
		ResultCount:       trace.ResultCount,
		RerankEnabled:     trace.RerankEnabled,
	}
}

func formatRetrievalTraceNote(metrics *RetrievalStageMetrics) string {
	if metrics == nil {
		return "retrieval_trace unavailable"
	}
	return fmt.Sprintf(
		"retrieval cache_hit=%t init_ms=%d rewrite_ms=%d retrieve_ms=%d rerank_ms=%d raw=%d final=%d rerank=%t",
		metrics.CacheHit,
		metrics.InitLatencyMs,
		metrics.RewriteLatencyMs,
		metrics.RetrieveLatencyMs,
		metrics.RerankLatencyMs,
		metrics.RawResultCount,
		metrics.ResultCount,
		metrics.RerankEnabled,
	)
}
