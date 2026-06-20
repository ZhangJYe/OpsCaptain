package chat_pipeline

import (
	"SuperBizAgent/internal/ai/models"
	"context"

	"github.com/cloudwego/eino/components/model"
)

func newChatModel(ctx context.Context) (cm model.ToolCallingChatModel, err error) {
	cm, err = models.OpenAIForGLMFast(ctx)
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func newChatModelWithQuery(ctx context.Context, query string) (cm model.ToolCallingChatModel, err error) {
	router := models.NewModelRouterFromConfig(ctx)
	modelKey, _ := router.Route(ctx, query)
	cm, err = models.OpenAIChatModelFactory(modelKey)(ctx)
	if err != nil {
		return nil, err
	}
	return cm, nil
}
