package knowledge_index_pipeline

import (
	loader2 "SuperBizAgent/internal/ai/loader"
	"context"

	"github.com/cloudwego/eino/components/document"
)

// newLoader component initialization function of node 'FileLoader' in graph 'KnowledgeIndexing'
func newLoader(ctx context.Context) (ldr document.Loader, err error) {
	return loader2.NewFileLoader(ctx)
}
