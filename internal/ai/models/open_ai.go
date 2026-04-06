package models

import (
	"SuperBizAgent/utility/common"
	"context"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/gogf/gf/v2/frame/g"
)

func OpenAIForDeepSeekV31Think(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	model, err := g.Cfg().Get(ctx, "ds_think_chat_model.model")
	if err != nil {
		return nil, err
	}
	api_key, err := g.Cfg().Get(ctx, "ds_think_chat_model.api_key")
	if err != nil {
		return nil, err
	}
	base_url, err := g.Cfg().Get(ctx, "ds_think_chat_model.base_url")
	if err != nil {
		return nil, err
	}
	config := &openai.ChatModelConfig{
		Model:   common.ResolveEnv(model.String()),
		APIKey:  common.ResolveEnv(api_key.String()),
		BaseURL: common.ResolveEnv(base_url.String()),
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(cm, config.Model), nil
}

func OpenAIForDeepSeekV3Quick(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	model, err := g.Cfg().Get(ctx, "ds_quick_chat_model.model")
	if err != nil {
		return nil, err
	}
	api_key, err := g.Cfg().Get(ctx, "ds_quick_chat_model.api_key")
	if err != nil {
		return nil, err
	}
	base_url, err := g.Cfg().Get(ctx, "ds_quick_chat_model.base_url")
	if err != nil {
		return nil, err
	}
	config := &openai.ChatModelConfig{
		Model:   common.ResolveEnv(model.String()),
		APIKey:  common.ResolveEnv(api_key.String()),
		BaseURL: common.ResolveEnv(base_url.String()),
	}
	cm, err = openai.NewChatModel(ctx, config)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(cm, config.Model), nil
}
