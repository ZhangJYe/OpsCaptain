package models

import (
	"SuperBizAgent/utility/common"
	"context"
	"fmt"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/gogf/gf/v2/frame/g"
)

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
	config := &openai.ChatModelConfig{
		Model:   common.ResolveEnv(modelName),
		APIKey:  common.ResolveEnv(apiKey),
		BaseURL: common.ResolveEnv(baseURL),
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
	config := &openai.ChatModelConfig{
		Model:   common.ResolveEnv(modelName),
		APIKey:  common.ResolveEnv(apiKey),
		BaseURL: common.ResolveEnv(baseURL),
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(cm, config.Model), nil
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
