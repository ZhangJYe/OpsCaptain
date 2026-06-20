package tools

import (
	"SuperBizAgent/internal/ai/actionexecutor"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type ExecuteActionInput struct {
	ActionID string            `json:"action" jsonschema:"description=动作 ID，例如 restart_service、query_service_status、scale_deployment"`
	Params   map[string]string `json:"params,omitempty" jsonschema:"description=动作参数，例如 {\"service\": \"paymentservice\", \"namespace\": \"default\"}"`
}

type ExecuteActionOutput struct {
	Success  bool                       `json:"success"`
	RequiresApproval bool               `json:"requires_approval,omitempty"`
	Result   *actionexecutor.ActionResult `json:"result,omitempty"`
	Error    string                     `json:"error,omitempty"`
	Message  string                     `json:"message,omitempty"`
}

func NewExecuteActionTool(registry *actionexecutor.Registry) tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"execute_action",
		"执行预定义的运维动作（需要人工确认）。可执行的动作包括：重启服务、查询服务状态、扩缩容等。所有执行操作都会返回结果，高风险操作需要审批。使用前请先确认用户同意执行。",
		func(ctx context.Context, input *ExecuteActionInput, opts ...tool.Option) (output string, err error) {
			if registry == nil {
				return marshalExecuteOutput(ExecuteActionOutput{
					Success: false,
					Error:   "Action executor not available",
					Message: "执行引擎未配置。",
				})
			}

			action, ok := registry.Get(input.ActionID)
			if !ok {
				return marshalExecuteOutput(ExecuteActionOutput{
					Success: false,
					Error:   fmt.Sprintf("action %q not found", input.ActionID),
					Message: fmt.Sprintf("未找到动作 %q。使用 list_actions 查看可用动作。", input.ActionID),
				})
			}

			// High-risk actions need approval
			if action.RiskLevel == "high" || action.RiskLevel == "medium" {
				return marshalExecuteOutput(ExecuteActionOutput{
					Success:          false,
					RequiresApproval: true,
					Error:            fmt.Sprintf("action %q requires approval (risk_level=%s)", input.ActionID, action.RiskLevel),
					Message:          fmt.Sprintf("动作 %q 风险等级为 %s，需要人工审批后执行。", action.Name, action.RiskLevel),
				})
			}

			// Low-risk actions can execute directly
			result, execErr := registry.Execute(ctx, input.ActionID, input.Params)
			if execErr != nil {
				return marshalExecuteOutput(ExecuteActionOutput{
					Success: false,
					Error:   execErr.Error(),
					Message: fmt.Sprintf("执行动作 %q 失败。", action.Name),
				})
			}

			return marshalExecuteOutput(ExecuteActionOutput{
				Success: true,
				Result:  result,
				Message: fmt.Sprintf("动作 %q 执行完成。", action.Name),
			})
		})
	if err != nil {
		return nil
	}
	return t
}

func marshalExecuteOutput(out ExecuteActionOutput) (string, error) {
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"success":false,"error":"marshal failed: %s"}`, err.Error()), nil
	}
	return string(b), nil
}
