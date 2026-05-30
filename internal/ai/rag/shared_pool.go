package rag

import (
	"context"
	"fmt"
	"sync"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
)

var NewRetrieverFunc RetrieverFactory

var (
	sharedPoolOnce sync.Once
	sharedPool     *RetrieverPool
)

func SharedPool() *RetrieverPool {
	sharedPoolOnce.Do(func() {
		sharedPool = NewRetrieverPool(
			func(ctx context.Context) (retrieverapi.Retriever, error) {
				if NewRetrieverFunc == nil {
					return nil, fmt.Errorf("rag.NewRetrieverFunc is not set; call rag.NewRetrieverFunc = ... before using SharedPool")
				}
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
