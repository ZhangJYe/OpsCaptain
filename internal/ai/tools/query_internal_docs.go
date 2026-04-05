package tools

import (
	ragretriever "SuperBizAgent/internal/ai/retriever"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

type QueryInternalDocsInput struct {
	Query string `json:"query" jsonschema:"description=The query string to search in internal documentation for relevant information and processing steps"`
}

const defaultInternalDocsQueryTimeout = 5 * time.Second
const defaultInternalDocsInitFailureTTL = 15 * time.Second

var (
	newMilvusRetriever         = ragretriever.NewMilvusRetriever
	internalDocsRetrieverMu    sync.Mutex
	internalDocsRetrieverCache struct {
		key      string
		rr       retrieverapi.Retriever
		lastErr  error
		failedAt time.Time
	}
)

func NewQueryInternalDocsTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_internal_docs",
		"Use this tool to search internal documentation and knowledge base for relevant information. It performs RAG (Retrieval-Augmented Generation) to find similar documents and extract processing steps. This is useful when you need to understand internal procedures, best practices, or step-by-step guides stored in the company's documentation.",
		func(ctx context.Context, input *QueryInternalDocsInput, opts ...tool.Option) (output string, err error) {
			queryCtx, cancel := context.WithTimeout(ctx, internalDocsQueryTimeout(ctx))
			defer cancel()

			rr, err := getOrCreateInternalDocsRetriever(queryCtx)
			if err != nil {
				return "", fmt.Errorf("failed to create retriever: %w", err)
			}
			resp, err := rr.Retrieve(queryCtx, input.Query)
			if err != nil {
				return "", fmt.Errorf("failed to retrieve documents: %w", err)
			}
			respBytes, err := json.Marshal(resp)
			if err != nil {
				return "", fmt.Errorf("failed to marshal response: %w", err)
			}
			return string(respBytes), nil
		})
	if err != nil {
		panic(fmt.Sprintf("failed to create query_internal_docs tool: %v", err))
	}
	return t
}

func internalDocsQueryTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "multi_agent.knowledge_query_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultInternalDocsQueryTimeout
}

func getOrCreateInternalDocsRetriever(ctx context.Context) (retrieverapi.Retriever, error) {
	cacheKey := internalDocsRetrieverCacheKey(ctx)

	internalDocsRetrieverMu.Lock()
	defer internalDocsRetrieverMu.Unlock()

	if internalDocsRetrieverCache.rr != nil && internalDocsRetrieverCache.key == cacheKey {
		return internalDocsRetrieverCache.rr, nil
	}
	if internalDocsRetrieverCache.key == cacheKey &&
		internalDocsRetrieverCache.lastErr != nil &&
		time.Since(internalDocsRetrieverCache.failedAt) < internalDocsInitFailureTTL(ctx) {
		return nil, internalDocsRetrieverCache.lastErr
	}

	rr, err := newMilvusRetriever(ctx)
	if err != nil {
		internalDocsRetrieverCache.key = cacheKey
		internalDocsRetrieverCache.rr = nil
		internalDocsRetrieverCache.lastErr = err
		internalDocsRetrieverCache.failedAt = time.Now()
		return nil, err
	}
	internalDocsRetrieverCache.key = cacheKey
	internalDocsRetrieverCache.rr = rr
	internalDocsRetrieverCache.lastErr = nil
	internalDocsRetrieverCache.failedAt = time.Time{}
	return rr, nil
}

func resetInternalDocsRetrieverCache() {
	internalDocsRetrieverMu.Lock()
	defer internalDocsRetrieverMu.Unlock()
	internalDocsRetrieverCache.key = ""
	internalDocsRetrieverCache.rr = nil
	internalDocsRetrieverCache.lastErr = nil
	internalDocsRetrieverCache.failedAt = time.Time{}
}

func internalDocsRetrieverCacheKey(ctx context.Context) string {
	topK := 3
	if v, err := g.Cfg().Get(ctx, "retriever.top_k"); err == nil && v.Int() > 0 {
		topK = v.Int()
	}
	return fmt.Sprintf("%s|%d", common.GetMilvusAddr(ctx), topK)
}

func internalDocsInitFailureTTL(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "multi_agent.knowledge_init_failure_ttl_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultInternalDocsInitFailureTTL
}
