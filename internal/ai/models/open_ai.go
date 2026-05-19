package models

import (
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/gogf/gf/v2/frame/g"
)

func OpenAIChatModelFactory(configName string) func(context.Context) (model.ToolCallingChatModel, error) {
	switch strings.ToLower(strings.TrimSpace(configName)) {
	case "chat_model", "glm_chat_model":
		return OpenAIForGLM
	default:
		return OpenAIForGLMFast
	}
}

func OpenAIForGLM(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	modelName, err := readModelConfig(ctx, "chat_model.model", "glm_chat_model.model")
	if err != nil {
		return nil, err
	}
	apiKey, err := readModelConfig(ctx, "chat_model.api_key", "glm_chat_model.api_key")
	if err != nil {
		return nil, err
	}
	baseURL, err := readModelConfig(ctx, "chat_model.base_url", "glm_chat_model.base_url")
	if err != nil {
		return nil, err
	}
	config, err := buildOpenAIChatConfig(modelName, apiKey, baseURL)
	if err != nil {
		return nil, err
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(cm, config.Model), nil
}

func OpenAIForGLMFast(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	modelName, err := readModelConfig(ctx, "chat_model_fast.model", "glm_chat_model_fast.model")
	if err != nil {
		return nil, err
	}
	apiKey, err := readModelConfig(ctx, "chat_model_fast.api_key", "glm_chat_model_fast.api_key")
	if err != nil {
		return nil, err
	}
	baseURL, err := readModelConfig(ctx, "chat_model_fast.base_url", "glm_chat_model_fast.base_url")
	if err != nil {
		return nil, err
	}
	config, err := buildOpenAIChatConfig(modelName, apiKey, baseURL)
	if err != nil {
		return nil, err
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(cm, config.Model), nil
}

func buildOpenAIChatConfig(modelName, apiKey, baseURL string) (*openai.ChatModelConfig, error) {
	resolvedModel, ok := common.ResolveOptionalEnv(modelName)
	if !ok {
		return nil, fmt.Errorf("chat model name is empty or unresolved")
	}
	resolvedAPIKey, ok := common.ResolveOptionalEnv(apiKey)
	if !ok {
		return nil, fmt.Errorf("chat model api_key is empty or unresolved")
	}
	resolvedBaseURL, ok := common.ResolveOptionalEnv(baseURL)
	if !ok {
		return nil, fmt.Errorf("chat model base_url is empty or unresolved")
	}
	return &openai.ChatModelConfig{
		Model:   resolvedModel,
		APIKey:  resolvedAPIKey,
		BaseURL: resolvedBaseURL,
	}, nil
}

func readModelConfig(ctx context.Context, paths ...string) (string, error) {
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
	return "", fmt.Errorf("model config not found")
}
