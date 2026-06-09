package knowledge_index_pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/indexer"
)

// NewIndexerFunc is the factory function for creating an indexer.
// It must be set by the caller (e.g., main.go) to inject the infra-layer dependency.
var NewIndexerFunc func(ctx context.Context) (indexer.Indexer, error)

// newIndexer component initialization function of node 'RedisIndexer' in graph 'KnowledgeIndexing'
func newIndexer(ctx context.Context) (idr indexer.Indexer, err error) {
	if NewIndexerFunc == nil {
		return nil, fmt.Errorf("knowledge_index_pipeline.NewIndexerFunc is not set; inject from main")
	}
	return NewIndexerFunc(ctx)
}
