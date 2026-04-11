package retriever

import (
	"SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"context"
	"strings"

	"github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type safeRetriever struct {
	inner retriever.Retriever
}

func (s *safeRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	docs, err := s.inner.Retrieve(ctx, query, opts...)
	if err != nil && strings.Contains(err.Error(), "extra output fields") && strings.Contains(err.Error(), "does not dynamic field") {
		g.Log().Debugf(ctx, "milvus retriever returned empty-collection error, treating as empty result: %v", err)
		return nil, nil
	}
	return docs, err
}

func NewMilvusRetriever(ctx context.Context) (rtr retriever.Retriever, err error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}
	eb, err := embedder.DoubaoEmbedding(ctx)
	if err != nil {
		return nil, err
	}
	topK := common.GetRetrieverTopK(ctx)
	r, err := milvus.NewRetriever(ctx, &milvus.RetrieverConfig{
		Client:      cli,
		Collection:  common.GetMilvusCollectionName(ctx),
		VectorField: "vector",
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		TopK:      topK,
		Embedding: eb,
	})
	if err != nil {
		return nil, err
	}
	return &safeRetriever{inner: r}, nil
}
