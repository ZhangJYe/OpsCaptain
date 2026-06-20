package tools

import (
	"SuperBizAgent/internal/ai/actionexecutor"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteActionTool_NilRegistry(t *testing.T) {
	tool := NewExecuteActionTool(nil)
	input := `{"action":"test"}`
	result, err := tool.InvokableRun(context.Background(), input)
	require.NoError(t, err)

	var output ExecuteActionOutput
	require.NoError(t, json.Unmarshal([]byte(result), &output))
	assert.False(t, output.Success)
	assert.Contains(t, output.Error, "not available")
}

func TestExecuteActionTool_LowRisk(t *testing.T) {
	registry := actionexecutor.NewRegistry()
	registry.Register(&actionexecutor.ActionDefinition{
		ID:       "query_status",
		Name:     "查询状态",
		Category: "query",
		RiskLevel: "low",
		Executor: "http",
		Config:   map[string]string{"method": "GET", "url": "http://localhost:9999/health"},
	})

	tool := NewExecuteActionTool(registry)
	input := `{"action":"query_status","params":{}}`
	result, err := tool.InvokableRun(context.Background(), input)
	require.NoError(t, err)

	var output ExecuteActionOutput
	require.NoError(t, json.Unmarshal([]byte(result), &output))
	// Will fail to connect but should not panic
	assert.False(t, output.Success)
}

func TestExecuteActionTool_HighRiskRequiresApproval(t *testing.T) {
	registry := actionexecutor.NewRegistry()
	registry.Register(&actionexecutor.ActionDefinition{
		ID:       "restart_service",
		Name:     "重启服务",
		Category: "restart",
		RiskLevel: "high",
		Executor: "http",
	})

	tool := NewExecuteActionTool(registry)
	input := `{"action":"restart_service","params":{"service":"paymentservice"}}`
	result, err := tool.InvokableRun(context.Background(), input)
	require.NoError(t, err)

	var output ExecuteActionOutput
	require.NoError(t, json.Unmarshal([]byte(result), &output))
	assert.False(t, output.Success)
	assert.True(t, output.RequiresApproval)
	assert.Contains(t, output.Message, "需要人工审批")
}

func TestExecuteActionTool_MediumRiskRequiresApproval(t *testing.T) {
	registry := actionexecutor.NewRegistry()
	registry.Register(&actionexecutor.ActionDefinition{
		ID:       "scale_deployment",
		Name:     "扩缩容",
		Category: "scale",
		RiskLevel: "medium",
		Executor: "http",
	})

	tool := NewExecuteActionTool(registry)
	input := `{"action":"scale_deployment","params":{"replicas":"3"}}`
	result, err := tool.InvokableRun(context.Background(), input)
	require.NoError(t, err)

	var output ExecuteActionOutput
	require.NoError(t, json.Unmarshal([]byte(result), &output))
	assert.False(t, output.Success)
	assert.True(t, output.RequiresApproval)
}

func TestExecuteActionTool_NotFound(t *testing.T) {
	registry := actionexecutor.NewRegistry()
	tool := NewExecuteActionTool(registry)
	input := `{"action":"nonexistent"}`
	result, err := tool.InvokableRun(context.Background(), input)
	require.NoError(t, err)

	var output ExecuteActionOutput
	require.NoError(t, json.Unmarshal([]byte(result), &output))
	assert.False(t, output.Success)
	assert.Contains(t, output.Error, "not found")
}
