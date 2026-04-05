package chat_pipeline

import (
	"SuperBizAgent/internal/ai/rag"
	retriever2 "SuperBizAgent/internal/ai/retriever"
	"context"
	"time"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultChatRetrieverInitTimeout = 5 * time.Second
	defaultChatRetrieverFailureTTL  = 15 * time.Second
)

var (
	newChatPipelineRetriever = retriever2.NewMilvusRetriever
	chatRetrieverPool        = rag.NewRetrieverPool(
		func(ctx context.Context) (retriever.Retriever, error) {
			return newChatPipelineRetriever(ctx)
		},
		rag.DefaultRetrieverCacheKey,
		chatRetrieverInitFailureTTL,
	)
)

type fallbackRetriever struct {
	inner retriever.Retriever
}

func (f *fallbackRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	if f.inner == nil {
		return nil, nil
	}
	docs, err := f.inner.Retrieve(ctx, query, opts...)
	if err != nil {
		g.Log().Warningf(ctx, "retriever error, returning empty docs: %v", err)
		return nil, nil
	}
	return docs, nil
}

func (f *fallbackRetriever) GetType() string {
	return "fallback_retriever"
}

func newRetriever(ctx context.Context) (rtr retriever.Retriever, err error) {
	initCtx, cancel := context.WithTimeout(ctx, chatRetrieverInitTimeout(ctx))
	defer cancel()

	inner, _, err := chatRetrieverPool.GetOrCreate(initCtx)
	if err != nil {
		g.Log().Warningf(ctx, "Milvus unavailable, RAG disabled for this request: %v", err)
		return &fallbackRetriever{}, nil
	}
	return &fallbackRetriever{inner: inner}, nil
}

func chatRetrieverInitTimeout(ctx context.Context) time.Duration {
	return rag.DurationFromConfig(ctx, defaultChatRetrieverInitTimeout, "chat.rag_init_timeout_ms", "context.docs_query_timeout_ms")
}

func chatRetrieverInitFailureTTL(ctx context.Context) time.Duration {
	return rag.DurationFromConfig(ctx, defaultChatRetrieverFailureTTL, "chat.rag_init_failure_ttl_ms", "context.docs_init_failure_ttl_ms", "multi_agent.knowledge_init_failure_ttl_ms")
}
