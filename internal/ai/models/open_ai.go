package models

import (
	"SuperBizAgent/utility/common"
	"context"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

func OpenAIChatModelFactory(configName string) func(context.Context) (model.ToolCallingChatModel, error) {
	switch strings.ToLower(strings.TrimSpace(configName)) {
	case common.ChatModelPrimary:
		return OpenAIForGLM
	default:
		return OpenAIForGLMFast
	}
}

func OpenAIForGLM(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	settings, err := common.LoadChatModelConfig(ctx, common.ChatModelPrimary)
	if err != nil {
		return nil, err
	}
	config := &openai.ChatModelConfig{
		Model:   settings.Model,
		APIKey:  settings.APIKey,
		BaseURL: settings.BaseURL,
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(cm, config.Model), nil
}

func OpenAIForGLMFast(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	settings, err := common.LoadChatModelConfig(ctx, common.ChatModelFast)
	if err != nil {
		return nil, err
	}
	config := &openai.ChatModelConfig{
		Model:   settings.Model,
		APIKey:  settings.APIKey,
		BaseURL: settings.BaseURL,
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(cm, config.Model), nil
}
