package embedder

import (
	"SuperBizAgent/utility/common"
	"context"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/gogf/gf/v2/frame/g"
)

func DoubaoEmbedding(ctx context.Context) (eb embedding.Embedder, err error) {
	model, err := g.Cfg().Get(ctx, "doubao_embedding_model.model")
	if err != nil {
		return nil, err
	}
	api_key, err := g.Cfg().Get(ctx, "doubao_embedding_model.api_key")
	if err != nil {
		return nil, err
	}
	base_url, err := g.Cfg().Get(ctx, "doubao_embedding_model.base_url")
	if err != nil {
		return nil, err
	}
	dim := common.GetVectorDimension(ctx)
	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		Model:      common.ResolveEnv(model.String()),
		APIKey:     common.ResolveEnv(api_key.String()),
		BaseURL:    common.ResolveEnv(base_url.String()),
		Dimensions: &dim,
	})
	if err != nil {
		g.Log().Errorf(ctx, "new embedder error: %v", err)
		return nil, err
	}
	return embedder, nil
}
