package embedder

import (
	"SuperBizAgent/utility/common"
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/embedding/openai"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/gogf/gf/v2/frame/g"
)

func DoubaoEmbedding(ctx context.Context) (eb embedding.Embedder, err error) {
	modelName, err := readEmbeddingConfig(ctx, "embedding_model.model", "doubao_embedding_model.model")
	if err != nil {
		return nil, err
	}
	apiKey, err := readEmbeddingConfig(ctx, "embedding_model.api_key", "doubao_embedding_model.api_key")
	if err != nil {
		return nil, err
	}
	baseURL, err := readEmbeddingConfig(ctx, "embedding_model.base_url", "doubao_embedding_model.base_url")
	if err != nil {
		return nil, err
	}
	dim := common.GetVectorDimension(ctx)
	embedder, err := openai.NewEmbedder(ctx, &openai.EmbeddingConfig{
		Model:      common.ResolveEnv(modelName),
		APIKey:     common.ResolveEnv(apiKey),
		BaseURL:    common.ResolveEnv(baseURL),
		Dimensions: &dim,
	})
	if err != nil {
		g.Log().Errorf(ctx, "new embedder error: %v", err)
		return nil, err
	}
	return embedder, nil
}

func readEmbeddingConfig(ctx context.Context, paths ...string) (string, error) {
	var lastErr error
	var foundEmpty bool
	for _, path := range paths {
		v, err := g.Cfg().Get(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		s := v.String()
		if s != "" {
			return s, nil
		}
		foundEmpty = true
	}
	if foundEmpty {
		return "", nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("embedding config not found")
}
