package contextengine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	ragretriever "SuperBizAgent/internal/ai/retriever"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/mem"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const defaultContextDocsQueryTimeout = 5 * time.Second
const defaultContextDocsInitFailureTTL = 15 * time.Second

var (
	newContextRetriever   = ragretriever.NewMilvusRetriever
	contextRetrieverMu    sync.Mutex
	contextRetrieverCache cachedContextRetriever
)

type cachedContextRetriever struct {
	key      string
	rr       retrieverapi.Retriever
	lastErr  error
	failedAt time.Time
}

func getOrCreateContextRetriever(ctx context.Context) (retrieverapi.Retriever, error) {
	cacheKey := contextRetrieverCacheKey(ctx)

	contextRetrieverMu.Lock()
	defer contextRetrieverMu.Unlock()

	if contextRetrieverCache.rr != nil && contextRetrieverCache.key == cacheKey {
		return contextRetrieverCache.rr, nil
	}
	if contextRetrieverCache.key == cacheKey &&
		contextRetrieverCache.lastErr != nil &&
		time.Since(contextRetrieverCache.failedAt) < contextDocsInitFailureTTL(ctx) {
		return nil, contextRetrieverCache.lastErr
	}

	rr, err := newContextRetriever(ctx)
	if err != nil {
		contextRetrieverCache.key = cacheKey
		contextRetrieverCache.rr = nil
		contextRetrieverCache.lastErr = err
		contextRetrieverCache.failedAt = time.Now()
		return nil, err
	}
	contextRetrieverCache.key = cacheKey
	contextRetrieverCache.rr = rr
	contextRetrieverCache.lastErr = nil
	contextRetrieverCache.failedAt = time.Time{}
	return rr, nil
}

func resetContextRetrieverCache() {
	contextRetrieverMu.Lock()
	defer contextRetrieverMu.Unlock()
	contextRetrieverCache = cachedContextRetriever{}
}

func contextRetrieverCacheKey(ctx context.Context) string {
	topK := 3
	if v, err := g.Cfg().Get(ctx, "retriever.top_k"); err == nil && v.Int() > 0 {
		topK = v.Int()
	}
	return fmt.Sprintf("%s|%d", common.GetMilvusAddr(ctx), topK)
}

func contextDocsQueryTimeout(ctx context.Context) time.Duration {
	if v, err := g.Cfg().Get(ctx, "context.docs_query_timeout_ms"); err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "multi_agent.knowledge_query_timeout_ms"); err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultContextDocsQueryTimeout
}

func contextDocsInitFailureTTL(ctx context.Context) time.Duration {
	if v, err := g.Cfg().Get(ctx, "context.docs_init_failure_ttl_ms"); err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "multi_agent.knowledge_init_failure_ttl_ms"); err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultContextDocsInitFailureTTL
}

func selectDocuments(ctx context.Context, query string, profile ContextProfile) ([]ContextItem, []ContextItem, int, []string) {
	if strings.TrimSpace(query) == "" || !profile.AllowDocs || profile.Budget.DocumentTokens == 0 {
		return nil, nil, 0, []string{"documents disabled"}
	}

	queryCtx, cancel := context.WithTimeout(ctx, contextDocsQueryTimeout(ctx))
	defer cancel()

	rr, err := getOrCreateContextRetriever(queryCtx)
	if err != nil {
		return nil, nil, 0, []string{fmt.Sprintf("documents unavailable: %v", err)}
	}
	docs, err := rr.Retrieve(queryCtx, query)
	if err != nil {
		return nil, nil, 0, []string{fmt.Sprintf("documents retrieve failed: %v", err)}
	}
	if len(docs) == 0 {
		return nil, nil, 0, []string{"documents empty"}
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

	return selected, dropped, used, []string{fmt.Sprintf("tokens=%d/%d", used, profile.Budget.DocumentTokens)}
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
