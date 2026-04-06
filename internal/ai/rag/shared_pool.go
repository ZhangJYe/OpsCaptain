package rag

import (
	ragretriever "SuperBizAgent/internal/ai/retriever"
	"context"
	"sync"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
)

var NewRetrieverFunc RetrieverFactory = ragretriever.NewMilvusRetriever

var (
	sharedPoolOnce sync.Once
	sharedPool     *RetrieverPool
)

func SharedPool() *RetrieverPool {
	sharedPoolOnce.Do(func() {
		sharedPool = NewRetrieverPool(
			func(ctx context.Context) (retrieverapi.Retriever, error) {
				return NewRetrieverFunc(ctx)
			},
			DefaultRetrieverCacheKey,
			sharedInitFailureTTL,
		)
	})
	return sharedPool
}

func ResetSharedPool() {
	if sharedPool != nil {
		sharedPool.Reset()
	}
}

const defaultSharedInitFailureTTL = 15 * time.Second

func sharedInitFailureTTL(ctx context.Context) time.Duration {
	return DurationFromConfig(
		ctx,
		defaultSharedInitFailureTTL,
		"chat.rag_init_failure_ttl_ms",
		"context.docs_init_failure_ttl_ms",
		"multi_agent.knowledge_init_failure_ttl_ms",
	)
}
