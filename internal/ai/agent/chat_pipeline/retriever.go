package chat_pipeline

import (
	retriever2 "SuperBizAgent/internal/ai/retriever"
	"context"
	"time"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
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
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	inner, err := retriever2.NewMilvusRetriever(initCtx)
	if err != nil {
		g.Log().Warningf(ctx, "Milvus unavailable, RAG disabled for this request: %v", err)
		return &fallbackRetriever{}, nil
	}
	return &fallbackRetriever{inner: inner}, nil
}
