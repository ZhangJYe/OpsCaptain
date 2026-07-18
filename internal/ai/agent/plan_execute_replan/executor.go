package plan_execute_replan

import (
	aitools "SuperBizAgent/internal/ai/tools"
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
)

type executorToolsContextKey struct{}

func WithExecutorTools(ctx context.Context, toolList []tool.BaseTool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, executorToolsContextKey{}, append([]tool.BaseTool(nil), toolList...))
}

func executorToolsFromContext(ctx context.Context) ([]tool.BaseTool, bool) {
	if ctx == nil {
		return nil, false
	}
	toolList, ok := ctx.Value(executorToolsContextKey{}).([]tool.BaseTool)
	if !ok {
		return nil, false
	}
	return append([]tool.BaseTool(nil), toolList...), true
}

func NewExecutor(ctx context.Context) (adk.Agent, error) {
	toolList, overridden := executorToolsFromContext(ctx)
	if !overridden {
		mcpTool, err := aitools.GetLogMcpTool()
		if err != nil {
			return nil, err
		}
		toolList = mcpTool
		if t := aitools.NewPrometheusAlertsQueryTool(); t != nil {
			toolList = append(toolList, t)
		}
		if t := aitools.NewPrometheusMetricsDiscoveryTool(); t != nil {
			toolList = append(toolList, t)
		}
		if t := aitools.NewPrometheusRangeQueryTool(); t != nil {
			toolList = append(toolList, t)
		}
		if t := aitools.NewPrometheusInstantQueryTool(); t != nil {
			toolList = append(toolList, t)
		}
		if t := aitools.NewQueryInternalDocsTool(); t != nil {
			toolList = append(toolList, t)
		}
		if t := aitools.NewGetCurrentTimeTool(); t != nil {
			toolList = append(toolList, t)
		}
	}
	execModel, err := newPlanChatModel(ctx)
	if err != nil {
		return nil, err
	}
	return planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		Model: execModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: toolList,
			},
		},
		MaxIterations: 10,
	})
}
