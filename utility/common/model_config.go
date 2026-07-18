package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	ChatModelPrimary = "chat_model"
	ChatModelFast    = "chat_model_fast"
	EmbeddingModel   = "embedding_model"
)

type AIModelConfig struct {
	Provider  string
	Model     string
	APIKey    string
	BaseURL   string
	Dimension int
}

func LoadChatModelConfig(ctx context.Context, configName string) (AIModelConfig, error) {
	switch configName {
	case ChatModelPrimary, ChatModelFast:
		return loadAIModelConfig(ctx, configName, false)
	default:
		return AIModelConfig{}, fmt.Errorf("unsupported chat model config %q", configName)
	}
}

func LoadEmbeddingModelConfig(ctx context.Context) (AIModelConfig, error) {
	return loadAIModelConfig(ctx, EmbeddingModel, true)
}

func loadAIModelConfig(ctx context.Context, configName string, requireDimension bool) (AIModelConfig, error) {
	var config AIModelConfig
	provider, err := readResolvedModelValue(ctx, configName+".provider", false)
	if err != nil {
		return config, err
	}
	config.Provider = strings.ToLower(provider)
	modelName, err := readResolvedModelValue(ctx, configName+".model", false)
	if err != nil {
		return config, err
	}
	config.Model = modelName
	apiKey, err := readResolvedModelValue(ctx, configName+".api_key", true)
	if err != nil {
		return config, err
	}
	config.APIKey = apiKey
	baseURL, err := readResolvedModelValue(ctx, configName+".base_url", false)
	if err != nil {
		return config, err
	}
	config.BaseURL = strings.TrimRight(baseURL, "/")
	if requireDimension {
		dimension, err := g.Cfg().Get(ctx, configName+".dimension")
		if err != nil || dimension.Int() <= 0 {
			return config, fmt.Errorf("%s.dimension must be a positive integer", configName)
		}
		config.Dimension = dimension.Int()
	}
	return config, nil
}

func readResolvedModelValue(ctx context.Context, path string, secret bool) (string, error) {
	value, err := g.Cfg().Get(ctx, path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	resolved, ok := ResolveOptionalEnv(value.String())
	if !ok {
		return "", fmt.Errorf("%s is empty or unresolved", path)
	}
	if secret && LooksLikePlaceholderSecret(resolved) {
		return "", fmt.Errorf("%s is empty or uses a placeholder", path)
	}
	return resolved, nil
}
