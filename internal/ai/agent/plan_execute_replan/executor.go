package plan_execute_replan

import (
	"SuperBizAgent/internal/ai/tools"
	"context"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/compose"
)

func NewExecutor(ctx context.Context) (adk.Agent, error) {
	// log
	mcpTool, err := tools.GetLogMcpTool()
	if err != nil {
		return nil, err
	}
	toolList := mcpTool
	// alerts
	if t := tools.NewPrometheusAlertsQueryTool(); t != nil {
		toolList = append(toolList, t)
	}
	if t := tools.NewPrometheusMetricsDiscoveryTool(); t != nil {
		toolList = append(toolList, t)
	}
	if t := tools.NewPrometheusRangeQueryTool(); t != nil {
		toolList = append(toolList, t)
	}
	if t := tools.NewPrometheusInstantQueryTool(); t != nil {
		toolList = append(toolList, t)
	}
	// file
	if t := tools.NewQueryInternalDocsTool(); t != nil {
		toolList = append(toolList, t)
	}
	// time
	if t := tools.NewGetCurrentTimeTool(); t != nil {
		toolList = append(toolList, t)
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
