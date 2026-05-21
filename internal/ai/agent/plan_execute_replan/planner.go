package plan_execute_replan

import (
	"SuperBizAgent/internal/ai/models"
	"context"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/gogf/gf/v2/frame/g"
)

func NewPlanner(ctx context.Context) (adk.Agent, error) {
	planModel, err := newPlanChatModel(ctx)
	if err != nil {
		return nil, err
	}
	return planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
		ToolCallingChatModel: planModel,
	})
}

func newPlanChatModel(ctx context.Context) (einomodel.ToolCallingChatModel, error) {
	modelPath := "chat_model_fast"
	if v, err := g.Cfg().Get(ctx, "aiops.plan.model_path"); err == nil && strings.TrimSpace(v.String()) != "" {
		modelPath = strings.TrimSpace(v.String())
	}
	return models.OpenAIChatModelFactory(modelPath)(ctx)
}
