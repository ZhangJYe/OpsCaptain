package experts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	einoschema "github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/belief"
)

type fallbackQueryTestTool struct {
	args  string
	calls int
}

type namedExpertTestTool struct {
	name string
}

func (t *namedExpertTestTool) Info(context.Context) (*einoschema.ToolInfo, error) {
	return &einoschema.ToolInfo{Name: t.name}, nil
}

func (t *namedExpertTestTool) InvokableRun(context.Context, string, ...einotool.Option) (string, error) {
	return `{"success":true}`, nil
}

func (t *fallbackQueryTestTool) Info(context.Context) (*einoschema.ToolInfo, error) {
	return &einoschema.ToolInfo{Name: "query_logs"}, nil
}

func (t *fallbackQueryTestTool) InvokableRun(_ context.Context, args string, _ ...einotool.Option) (string, error) {
	t.args = args
	t.calls++
	return `{"success":true,"data":"pod_cpu_usage=95"}`, nil
}

type fakeExpertChatModel struct {
	content       string
	err           error
	delay         time.Duration
	maxTokensSeen int
}

func (f *fakeExpertChatModel) Generate(ctx context.Context, input []*einoschema.Message, opts ...einomodel.Option) (*einoschema.Message, error) {
	if common := einomodel.GetCommonOptions(nil, opts...); common.MaxTokens != nil {
		f.maxTokensSeen = *common.MaxTokens
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return einoschema.AssistantMessage(f.content, nil), nil
}

func (f *fakeExpertChatModel) Stream(ctx context.Context, input []*einoschema.Message, opts ...einomodel.Option) (*einoschema.StreamReader[*einoschema.Message], error) {
	return nil, fmt.Errorf("stream not implemented")
}

func (f *fakeExpertChatModel) WithTools(tools []*einoschema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	return f, nil
}

func TestGetArgBuilder(t *testing.T) {
	builder := GetArgBuilder("query_internal_docs")
	assert.IsType(t, &QueryArgBuilder{}, builder)

	builder = GetArgBuilder("query_logs")
	assert.IsType(t, &QueryArgBuilder{}, builder)

	builder = GetArgBuilder("get_current_time")
	assert.IsType(t, &RawArgBuilder{}, builder)

	builder = GetArgBuilder("query_prometheus_alerts")
	assert.IsType(t, &QueryArgBuilder{}, builder)

	builder = GetArgBuilder("unknown_tool")
	assert.IsType(t, &QueryArgBuilder{}, builder)
}

func TestQueryArgBuilder_Build(t *testing.T) {
	builder := &QueryArgBuilder{}
	result, err := builder.Build("test query")
	assert.NoError(t, err)
	assert.Equal(t, `{"query":"test query"}`, result)
}

func TestRawArgBuilder_Build(t *testing.T) {
	builder := &RawArgBuilder{}
	result, err := builder.Build("raw args")
	assert.NoError(t, err)
	assert.Equal(t, "raw args", result)
}

func TestNewToolRegistry(t *testing.T) {
	registry := NewToolRegistry()
	assert.NotNil(t, registry)
	assert.Empty(t, registry.tools)
}

func TestBaseExpert_Name(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:              "test_expert",
		Description:       "Test expert",
		ToolNames:         []string{},
		MaxRetrievalSteps: 3,
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)
	assert.Equal(t, "test_expert", expert.Name())
}

func TestNewLinuxSREExpert(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:              "linux_sre",
		Description:       "Linux SRE expert",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 3,
	}
	toolReg := NewToolRegistry()
	expert := NewLinuxSREExpert(cfg, toolReg)
	assert.NotNil(t, expert)
	assert.Equal(t, "linux_sre", expert.Name())
}

func TestNewNetworkSREExpert(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:              "network_sre",
		Description:       "Network SRE expert",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 3,
	}
	toolReg := NewToolRegistry()
	expert := NewNetworkSREExpert(cfg, toolReg)
	assert.NotNil(t, expert)
	assert.Equal(t, "network_sre", expert.Name())
}

func TestNewDatabaseSREExpert(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:              "database_sre",
		Description:       "Database SRE expert",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 3,
	}
	toolReg := NewToolRegistry()
	expert := NewDatabaseSREExpert(cfg, toolReg)
	assert.NotNil(t, expert)
	assert.Equal(t, "database_sre", expert.Name())
}

func TestParseToolOutput_Success(t *testing.T) {
	output := `{"success": true, "data": "test data"}`
	result := parseToolOutput(output)
	assert.True(t, result.Success)
	assert.True(t, result.HasExplicitFields)
	assert.True(t, result.HasSuccess)
	assert.False(t, result.Degraded)
	assert.Empty(t, result.Error)
}

func TestParseToolOutput_Degraded(t *testing.T) {
	output := `{"success": false, "degraded": true, "error": "partial data"}`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.True(t, result.HasExplicitFields)
	assert.True(t, result.HasSuccess)
	assert.True(t, result.Degraded)
	assert.Equal(t, "partial data", result.Error)
}

func TestParseToolOutput_Failed(t *testing.T) {
	output := `{"success": false, "error": "connection timeout"}`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.True(t, result.HasExplicitFields)
	assert.True(t, result.HasSuccess)
	assert.False(t, result.Degraded)
	assert.Equal(t, "connection timeout", result.Error)
}

func TestParseToolOutput_InvalidJSON(t *testing.T) {
	output := `not json`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.NotNil(t, result.Content)
}

func TestParseToolOutput_MCPCallToolResult(t *testing.T) {
	output := `{"content": [{"type": "text", "text": "log output"}], "isError": false}`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.NotNil(t, result.Content)
	assert.False(t, result.IsError)
	assert.True(t, result.HasExplicitFields)
	assert.False(t, result.HasSuccess)
}

func TestParseToolOutput_MCPCallToolResultError(t *testing.T) {
	output := `{"content": [{"type": "text", "text": "error message"}], "isError": true}`
	result := parseToolOutput(output)
	assert.True(t, result.IsError)
	assert.True(t, result.HasExplicitFields)
	assert.False(t, result.HasSuccess)
}

func TestParseToolOutput_UnknownJSON(t *testing.T) {
	output := `{"foo": "bar", "baz": 123}`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.NotNil(t, result.Content)
	assert.False(t, result.HasExplicitFields)
	assert.False(t, result.HasSuccess)
}

func TestParseToolOutput_SuccessFalse(t *testing.T) {
	output := `{"success": false, "error": "query failed"}`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.True(t, result.HasExplicitFields)
	assert.True(t, result.HasSuccess)
	assert.Equal(t, "query failed", result.Error)
}

func TestParseToolOutput_HasSuccess(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		hasSuccess bool
		success    bool
	}{
		{
			name:       "explicit success true",
			output:     `{"success": true}`,
			hasSuccess: true,
			success:    true,
		},
		{
			name:       "explicit success false",
			output:     `{"success": false}`,
			hasSuccess: true,
			success:    false,
		},
		{
			name:       "MCP isError false",
			output:     `{"content": [], "isError": false}`,
			hasSuccess: false,
			success:    false,
		},
		{
			name:       "unknown JSON",
			output:     `{"foo": "bar"}`,
			hasSuccess: false,
			success:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseToolOutput(tt.output)
			assert.Equal(t, tt.hasSuccess, result.HasSuccess)
			assert.Equal(t, tt.success, result.Success)
		})
	}
}

func TestIsEmptyRetrievalOutput(t *testing.T) {
	tests := []struct {
		name   string
		output string
		empty  bool
	}{
		{name: "empty array", output: "[]", empty: true},
		{name: "empty content array", output: `{"content":[],"isError":false}`, empty: true},
		{name: "empty data array", output: `{"success":true,"data":[]}`, empty: true},
		{name: "empty alerts array", output: `{"success":true,"alerts":[]}`, empty: true},
		{name: "non empty alerts array", output: `{"success":true,"alerts":[{"alert_name":"HighLatency"}]}`, empty: false},
		{name: "non empty array", output: `[{"content":"SOP"}]`, empty: false},
		{name: "plain text", output: "SOP-PAY-001", empty: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.empty, isEmptyRetrievalOutput(tt.output))
		})
	}
}

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "api key",
			input:    "api_key: abcdefghijklmnop",
			expected: "[REDACTED]",
		},
		{
			name:     "bearer token",
			input:    "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "Authorization: [REDACTED]",
		},
		{
			name:     "email",
			input:    "User email: user@example.com",
			expected: "User email: [REDACTED]",
		},
		{
			name:     "ip address",
			input:    "Server IP: 192.168.1.1",
			expected: "Server IP: [REDACTED]",
		},
		{
			name:     "no secrets",
			input:    "Normal log output",
			expected: "Normal log output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactSecrets(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseExpert_MakeDecision(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:              "test",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 3,
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)

	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "Test hypothesis",
		Why:    "Test reason",
	}
	allowedTools := map[string]struct{}{"query_logs": {}}

	decision, err := expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]bool{}, allowedTools, true, true, false, false)
	require.NoError(t, err)
	assert.Equal(t, "retrieve", decision["action"])

	decision, err = expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{{Query: "q1", Output: "o1"}}, map[string]bool{}, allowedTools, true, true, false, true)
	require.NoError(t, err)
	assert.Equal(t, "retrieve", decision["action"])

	decision, err = expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{
		{Query: "q1", Output: "o1"},
		{Query: "q2", Output: "o2"},
	}, map[string]bool{}, allowedTools, true, true, false, true)
	require.NoError(t, err)
	assert.Equal(t, "analyze", decision["action"])
}

func TestBaseExpert_MakeDecision_LastStepWithEvidence(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:              "test",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 3,
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)

	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "Test hypothesis",
		Why:    "Test reason",
	}
	allowedTools := map[string]struct{}{"query_logs": {}}

	decision, err := expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]bool{}, allowedTools, true, true, true, true)
	require.NoError(t, err)
	assert.Equal(t, "analyze", decision["action"])
	assert.Equal(t, "0.5", decision["confidence"])

	decision, err = expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]bool{}, allowedTools, true, true, true, false)
	require.NoError(t, err)
	assert.Equal(t, "retrieve", decision["action"])
}

func TestBaseExpert_MakeDecision_CollectsSecondToolBeforeLastStepAnalysis(t *testing.T) {
	toolReg := NewToolRegistry()
	toolReg.Register("query_logs", &namedExpertTestTool{name: "query_logs"})
	toolReg.Register("query_prometheus_alerts", &namedExpertTestTool{name: "query_prometheus_alerts"})
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:      "test",
		ToolNames: []string{"query_logs", "query_prometheus_alerts"},
	}, toolReg)
	frontier := &belief.Frontier{NodeID: "test", Label: "Test hypothesis", Why: "Test reason"}
	allowedTools := map[string]struct{}{
		"query_logs":              {},
		"query_prometheus_alerts": {},
	}

	decision, err := expert.makeDecision(context.Background(), frontier, belief.NewBeliefGraph(), nil, map[string]bool{}, allowedTools, true, false, false, false)
	require.NoError(t, err)
	assert.Equal(t, "tool_call", decision["action"])
	assert.Equal(t, "query_logs", decision["tool"])

	decision, err = expert.makeDecision(context.Background(), frontier, belief.NewBeliefGraph(), []RetrievalRecord{{Tool: "query_logs", Output: "log evidence"}}, map[string]bool{"query_logs": true}, allowedTools, true, false, true, true)
	require.NoError(t, err)
	assert.Equal(t, "tool_call", decision["action"])
	assert.Equal(t, "query_prometheus_alerts", decision["tool"])

	decision, err = expert.makeDecision(context.Background(), frontier, belief.NewBeliefGraph(), []RetrievalRecord{{Tool: "query_logs", Output: "log evidence"}}, map[string]bool{"query_logs": true, "query_prometheus_alerts": true}, allowedTools, true, false, true, true)
	require.NoError(t, err)
	assert.Equal(t, "analyze", decision["action"])
}

func TestBaseExpertRunWithTwoToolsCompletesForcedAnalysisWithoutDegrading(t *testing.T) {
	toolReg := NewToolRegistry()
	toolReg.Register("query_logs", &namedExpertTestTool{name: "query_logs"})
	toolReg.Register("query_prometheus_alerts", &namedExpertTestTool{name: "query_prometheus_alerts"})
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:      "network_sre",
		ToolNames: []string{"query_logs", "query_prometheus_alerts"},
		ExecutionBudget: ExecutionBudget{
			LLMCalls: 3, ToolCalls: 2, RAGCalls: 1, Timeout: time.Second, MaxRetrievalSteps: 2, MaxOutputTokens: 256,
		},
		GenerateContentFunc: func(_ context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error) {
			if decision["action"] == "tool_call" {
				return "eval-retry-inventory", nil
			}
			require.Len(t, history, 2)
			return `{"analysis":"日志和告警共同支持重试风暴","confidence":0.88,"evidence":[{"index":0,"relation":"support","strength":0.9},{"index":1,"relation":"support","strength":0.7}]}`, nil
		},
	}, toolReg)
	frontier := &belief.Frontier{NodeID: "hypothesis", Label: "重试风暴", Why: "检查失败与重试证据"}

	result := expert.RunPlanned(context.Background(), ExpertTask{
		Frontier:     frontier,
		Graph:        belief.NewBeliefGraph(),
		AllowedTools: []string{"query_logs", "query_prometheus_alerts"},
		Budget: ExecutionBudget{
			LLMCalls: 3, ToolCalls: 2, RAGCalls: 1, Timeout: time.Second, MaxRetrievalSteps: 2, MaxOutputTokens: 256,
		},
	})

	require.Equal(t, "succeeded", result.Status)
	assert.Empty(t, result.DegradationReason)
	assert.Equal(t, 3, result.LLMCalls)
	assert.Equal(t, 2, result.ToolCalls)
	require.Len(t, result.Evidence, 2)
	assert.Equal(t, EvidenceRelationSupport, result.Evidence[0].Relation)
	assert.Equal(t, EvidenceRelationSupport, result.Evidence[1].Relation)
}

func TestEvidenceSourceIDIsStableForSameObservation(t *testing.T) {
	first := evidenceSourceID("query_logs", "cpu=95")
	second := evidenceSourceID("query_logs", "cpu=95")
	different := evidenceSourceID("query_logs", "cpu=96")

	assert.Equal(t, first, second)
	assert.NotEqual(t, first, different)
}

func TestBaseExpert_GenerateContent(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:      "test",
		ToolNames: []string{"query_logs"},
		ChatModelFactory: func(ctx context.Context) (einomodel.ToolCallingChatModel, error) {
			return &fakeExpertChatModel{content: "CPU 高负载分析结果"}, nil
		},
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)
	execution := expert.normalizeExecution(ExpertTask{})

	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "CPU 高负载",
		Why:    "CPU 使用率超过 90%",
	}

	content, err := expert.generateContent(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]string{
		"action": "tool_call",
	}, execution)
	require.NoError(t, err)
	assert.Contains(t, content, "CPU 高负载")

	content, err = expert.generateContent(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]string{
		"action": "retrieve",
	}, execution)
	require.NoError(t, err)
	assert.Contains(t, content, "CPU 高负载")

	content, err = expert.generateContent(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]string{
		"action":     "analyze",
		"confidence": "0.8",
	}, execution)
	require.NoError(t, err)
	assert.Contains(t, content, "CPU 高负载")
}

func TestBaseExpert_GenerateContentLLMTimeoutDegrades(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:        "test",
		CallTimeout: 20 * time.Millisecond,
		ChatModelFactory: func(ctx context.Context) (einomodel.ToolCallingChatModel, error) {
			return &fakeExpertChatModel{content: "late result", delay: 200 * time.Millisecond}, nil
		},
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)
	execution := expert.normalizeExecution(ExpertTask{})

	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "cpu high",
		Why:    "usage over 90%",
	}

	start := time.Now()
	content, err := expert.generateContent(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]string{
		"action": "retrieve",
	}, execution)

	assert.Less(t, time.Since(start), 150*time.Millisecond)
	assert.Error(t, err)
	assert.Empty(t, content)
}

func TestBaseExpert_GenerateContentKeepsLongUTF8Report(t *testing.T) {
	report := strings.Repeat("检查是否有异常进程（包括残留任务、重复推理进程和驱动异常）。", 20)
	cfg := ExpertRuntimeConfig{
		Name:        "test",
		CallTimeout: time.Second,
		ChatModelFactory: func(ctx context.Context) (einomodel.ToolCallingChatModel, error) {
			return &fakeExpertChatModel{content: report}, nil
		},
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)
	execution := expert.normalizeExecution(ExpertTask{})
	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "gpu high",
		Why:    "gpu usage over 90%",
	}

	content, err := expert.generateContent(context.Background(), frontier, graph, nil, map[string]string{
		"action": "analyze",
	}, execution)

	require.NoError(t, err)
	assert.Equal(t, report, content)
	assert.NotContains(t, content, "�")
}

func TestTruncateStringKeepsUTF8Valid(t *testing.T) {
	got := truncateString("检查是否有异常进程（包括残留任务）", 8)

	assert.Equal(t, "检查是否有异常进...", got)
	assert.NotContains(t, got, "�")
}

func TestBaseExpertEvidenceBudgetIsConfigurable(t *testing.T) {
	configured := NewBaseExpert(ExpertRuntimeConfig{EvidenceMaxChars: 8192}, NewToolRegistry())
	legacyFallback := NewBaseExpert(ExpertRuntimeConfig{}, NewToolRegistry())

	assert.Equal(t, 8192, configured.evidenceMaxChars())
	assert.Equal(t, 500, legacyFallback.evidenceMaxChars())
	assert.Contains(t, truncateString(strings.Repeat("x", 600)+"tail", configured.evidenceMaxChars()), "tail")
	assert.NotContains(t, truncateString(strings.Repeat("x", 600)+"tail", legacyFallback.evidenceMaxChars()), "tail")
}

func TestBaseExpertUsesDeterministicEvidenceQueryWhenLLMQueryFails(t *testing.T) {
	testTool := &fallbackQueryTestTool{}
	registry := NewToolRegistry()
	registry.Register("query_logs", testTool)
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:              "linux_sre",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 2,
		CallTimeout:       time.Second,
		GenerateContentFunc: func(_ context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, _ []RetrievalRecord, decision map[string]string) (string, error) {
			if decision["action"] == "tool_call" {
				return "", errors.New("llm query unavailable")
			}
			return "CPU evidence collected", nil
		},
	}, registry)
	graph := belief.NewBeliefGraph()
	graph.StartSignalID = "signal"
	graph.Nodes["signal"] = &belief.Node{ID: "signal", Label: "checkout latency increased"}
	frontier := &belief.Frontier{NodeID: "hypothesis", Label: "CPU saturation", Why: "verify CPU telemetry"}

	result := expert.Run(context.Background(), frontier, graph)

	require.Equal(t, "degraded", result.Status)
	require.Equal(t, 1, testTool.calls)
	require.Contains(t, testTool.args, "checkout latency increased")
	require.Contains(t, testTool.args, "CPU saturation")
	require.Len(t, result.Evidence, 1)
	require.Contains(t, result.Evidence[0].Snippet, "pod_cpu_usage")
	require.NotEmpty(t, result.ToolErrors)
}

func TestBaseExpertRunAppliesStructuredEvidenceRelations(t *testing.T) {
	testTool := &fallbackQueryTestTool{}
	registry := NewToolRegistry()
	registry.Register("query_logs", testTool)
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:              "linux_sre",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 2,
		CallTimeout:       time.Second,
		GenerateContentFunc: func(_ context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, _ []RetrievalRecord, decision map[string]string) (string, error) {
			if decision["action"] == "tool_call" {
				return "检查 CPU 指标", nil
			}
			return `{"analysis":"CPU 指标支持当前假设","confidence":0.82,"evidence":[{"index":0,"relation":"support","strength":0.9}]}`, nil
		},
	}, registry)
	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{NodeID: "hypothesis", Label: "CPU saturation", Why: "verify CPU telemetry"}

	result := expert.Run(context.Background(), frontier, graph)

	require.Equal(t, "succeeded", result.Status)
	require.Equal(t, "CPU 指标支持当前假设", result.Analysis)
	require.InDelta(t, 0.82, result.Confidence, 0.001)
	require.Len(t, result.Evidence, 1)
	require.Equal(t, EvidenceRelationSupport, result.Evidence[0].Relation)
	require.InDelta(t, 0.9, result.Evidence[0].Strength, 0.001)
	require.Equal(t, frontier.NodeID, result.Evidence[0].TargetHypothesisID)
}

func TestApplyAnalysisProposalRejectsInvalidEvidenceSchemaWithoutPartialMutation(t *testing.T) {
	result := &ExpertAnalysis{Evidence: []EvidenceItem{
		{Relation: EvidenceRelationNeutral},
		{Relation: EvidenceRelationNeutral},
	}}

	err := applyAnalysisProposal(result, `{"analysis":"invalid","confidence":0.8,"evidence":[{"index":0,"relation":"support","strength":0.9},{"index":0,"relation":"refute","strength":0.7}]}`)

	require.ErrorContains(t, err, "duplicated")
	require.Empty(t, result.Analysis)
	require.Zero(t, result.Confidence)
	require.Equal(t, EvidenceRelationNeutral, result.Evidence[0].Relation)
	require.Equal(t, EvidenceRelationNeutral, result.Evidence[1].Relation)
}

func TestApplyAnalysisProposalAcceptsStructuredRefinements(t *testing.T) {
	result := &ExpertAnalysis{Evidence: []EvidenceItem{{Relation: EvidenceRelationNeutral}}}

	err := applyAnalysisProposal(result, `{"analysis":"CPU 使用率持续高位","confidence":0.86,"evidence":[{"index":0,"relation":"support","strength":0.9}],"refinements":[{"label":"CPU 饱和","score":0.84,"why":"核验负载和节流状态","actionable":true}],"current_hypothesis_actionable":"true"}`)

	require.NoError(t, err)
	require.Len(t, result.Refinements, 1)
	assert.Equal(t, "CPU 饱和", result.Refinements[0].Label)
	assert.InDelta(t, 0.84, result.Refinements[0].Score, 0.001)
	assert.True(t, result.Refinements[0].Actionable)
	assert.Equal(t, EvidenceRelationSupport, result.Evidence[0].Relation)
	require.NotNil(t, result.CurrentHypothesisActionable)
	assert.True(t, *result.CurrentHypothesisActionable)
}

func TestApplyAnalysisProposalNormalizesQuotedRefinementActionability(t *testing.T) {
	result := &ExpertAnalysis{Evidence: []EvidenceItem{{Relation: EvidenceRelationNeutral}}}

	err := applyAnalysisProposal(result, `{"analysis":"CPU 使用率持续高位","confidence":0.86,"evidence":[{"index":0,"relation":"support","strength":0.9}],"refinements":[{"label":"CPU 饱和","score":0.84,"why":"核验负载和节流状态","actionable":"true"}]}`)

	require.NoError(t, err)
	require.Len(t, result.Refinements, 1)
	assert.True(t, result.Refinements[0].Actionable)
	assert.Equal(t, EvidenceRelationSupport, result.Evidence[0].Relation)
}

func TestApplyAnalysisProposalRejectsInvalidRefinementSchemaWithoutPartialMutation(t *testing.T) {
	result := &ExpertAnalysis{Evidence: []EvidenceItem{{Relation: EvidenceRelationNeutral}}}

	err := applyAnalysisProposal(result, `{"analysis":"CPU 使用率持续高位","confidence":0.86,"evidence":[{"index":0,"relation":"support","strength":0.9}],"refinements":[{"label":"CPU 饱和","score":0.84,"why":"核验负载和节流状态","actionable":true,"unknown":"invalid"}]}`)

	require.ErrorContains(t, err, "unknown field")
	assert.Empty(t, result.Analysis)
	assert.Zero(t, result.Confidence)
	assert.Empty(t, result.Refinements)
	assert.Equal(t, EvidenceRelationNeutral, result.Evidence[0].Relation)
}

func TestApplyAnalysisProposalRejectsRefinementWithoutDirectionalEvidence(t *testing.T) {
	result := &ExpertAnalysis{Evidence: []EvidenceItem{{Relation: EvidenceRelationNeutral}}}

	err := applyAnalysisProposal(result, `{"analysis":"告警仅说明服务异常，不能区分根因","confidence":0.5,"evidence":[{"index":0,"relation":"neutral","strength":0.3}],"refinements":[{"label":"CPU 饱和","score":0.7,"why":"需要进一步核验","actionable":true}]}`)

	require.ErrorContains(t, err, "require at least one support or refute")
	assert.Empty(t, result.Analysis)
	assert.Zero(t, result.Confidence)
	assert.Empty(t, result.Refinements)
	assert.Equal(t, EvidenceRelationNeutral, result.Evidence[0].Relation)
	assert.Zero(t, result.Evidence[0].Strength)
}

func TestApplyAnalysisProposalRejectsActionablePromotionWithoutSupport(t *testing.T) {
	result := &ExpertAnalysis{Evidence: []EvidenceItem{{Relation: EvidenceRelationNeutral}}}

	err := applyAnalysisProposal(result, `{"analysis":"告警不能区分根因","confidence":0.5,"evidence":[{"index":0,"relation":"neutral","strength":0.3}],"current_hypothesis_actionable":true}`)

	require.ErrorContains(t, err, "requires at least one support")
	assert.Nil(t, result.CurrentHypothesisActionable)
	assert.Empty(t, result.Analysis)
}

func TestBaseExpertRunDegradesWhenStructuredEvidenceAssessmentIsInvalid(t *testing.T) {
	testTool := &fallbackQueryTestTool{}
	registry := NewToolRegistry()
	registry.Register("query_logs", testTool)
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:              "linux_sre",
		ToolNames:         []string{"query_logs"},
		MaxRetrievalSteps: 2,
		CallTimeout:       time.Second,
		GenerateContentFunc: func(_ context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, _ []RetrievalRecord, decision map[string]string) (string, error) {
			if decision["action"] == "tool_call" {
				return "检查 CPU 指标", nil
			}
			return `{"analysis":"CPU 指标支持当前假设","confidence":0.82,"evidence":[]}`, nil
		},
	}, registry)
	frontier := &belief.Frontier{NodeID: "hypothesis", Label: "CPU saturation", Why: "verify CPU telemetry"}

	result := expert.Run(context.Background(), frontier, belief.NewBeliefGraph())

	require.Equal(t, "degraded", result.Status)
	require.Equal(t, "structured_assessment_invalid", result.DegradationReason)
	require.Len(t, result.Evidence, 1)
	require.Equal(t, EvidenceRelationNeutral, result.Evidence[0].Relation)
	require.Zero(t, result.Evidence[0].Strength)
	require.NotEmpty(t, result.ToolErrors)
	require.Equal(t, "evidence_assessment", result.ToolErrors[0].Action)
}

func TestBaseExpertRunPlannedEnforcesAuthorizationAndBudgets(t *testing.T) {
	authorizedTool := &fallbackQueryTestTool{}
	unauthorizedTool := &fallbackQueryTestTool{}
	registry := NewToolRegistry()
	registry.Register("query_internal_docs", unauthorizedTool)
	registry.Register("query_logs", authorizedTool)
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:              "linux_sre",
		ToolNames:         []string{"query_internal_docs", "query_logs"},
		MaxRetrievalSteps: 4,
		ExecutionBudget: ExecutionBudget{
			LLMCalls:          2,
			ToolCalls:         1,
			RAGCalls:          1,
			Timeout:           time.Second,
			MaxRetrievalSteps: 2,
			MaxOutputTokens:   128,
		},
		GenerateContentFunc: func(_ context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, _ []RetrievalRecord, decision map[string]string) (string, error) {
			if decision["action"] == "analyze" {
				return `{"analysis":"CPU 指标支持当前假设","confidence":0.8,"evidence":[{"index":0,"relation":"support","strength":0.9}]}`, nil
			}
			return "查询 CPU 使用率", nil
		},
	}, registry)
	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{NodeID: "hypothesis", Label: "CPU 饱和", Why: "验证 CPU 指标"}

	result := expert.RunPlanned(context.Background(), ExpertTask{
		Frontier:         frontier,
		Graph:            graph,
		ExpectedEvidence: []string{"CPU 使用率与节流状态"},
		AllowedTools:     []string{"query_logs"},
		StopConditions:   []string{"获得 source-backed evidence"},
		Budget: ExecutionBudget{
			LLMCalls:          10,
			ToolCalls:         10,
			RAGCalls:          10,
			Timeout:           5 * time.Second,
			MaxRetrievalSteps: 10,
			MaxOutputTokens:   1024,
		},
	})

	require.Equal(t, "succeeded", result.Status)
	assert.Equal(t, 2, result.LLMCalls)
	assert.Equal(t, 1, result.ToolCalls)
	assert.Zero(t, result.RAGCalls)
	assert.Equal(t, 1, authorizedTool.calls)
	assert.Zero(t, unauthorizedTool.calls)
	assert.Equal(t, []string{"query_logs"}, result.Metadata["allowed_tools"])
	assert.Equal(t, []string{"CPU 使用率与节流状态"}, result.Metadata["expected_evidence"])
}

func TestBaseExpertRunPlannedStopsAtOverallTimeoutWithinBudget(t *testing.T) {
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:              "linux_sre",
		MaxRetrievalSteps: 5,
		GenerateContentFunc: func(ctx context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, _ []RetrievalRecord, _ map[string]string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		RAGQueryFunc: func(ctx context.Context, _ string) ([]*einoschema.Document, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}, NewToolRegistry())
	startedAt := time.Now()

	result := expert.RunPlanned(context.Background(), ExpertTask{
		Frontier: &belief.Frontier{NodeID: "hypothesis", Label: "CPU 饱和"},
		Graph:    belief.NewBeliefGraph(),
		Budget: ExecutionBudget{
			LLMCalls:          3,
			ToolCalls:         1,
			RAGCalls:          1,
			Timeout:           20 * time.Millisecond,
			MaxRetrievalSteps: 3,
			MaxOutputTokens:   64,
		},
	})

	assert.Less(t, time.Since(startedAt), 150*time.Millisecond)
	assert.Equal(t, "degraded", result.Status)
	assert.Equal(t, "expert_timeout", result.DegradationReason)
	assert.LessOrEqual(t, result.LLMCalls, 3)
	assert.LessOrEqual(t, result.ToolCalls, 1)
	assert.LessOrEqual(t, result.RAGCalls, 1)
}

func TestBaseExpertRunPlannedCancellationDoesNotExceedBudget(t *testing.T) {
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:              "linux_sre",
		MaxRetrievalSteps: 5,
		GenerateContentFunc: func(ctx context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, _ []RetrievalRecord, _ map[string]string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
		RAGQueryFunc: func(ctx context.Context, _ string) ([]*einoschema.Document, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}, NewToolRegistry())
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	result := expert.RunPlanned(ctx, ExpertTask{
		Frontier: &belief.Frontier{NodeID: "hypothesis", Label: "CPU 饱和"},
		Graph:    belief.NewBeliefGraph(),
		Budget: ExecutionBudget{
			LLMCalls: 2, ToolCalls: 1, RAGCalls: 1, Timeout: time.Second, MaxRetrievalSteps: 2, MaxOutputTokens: 64,
		},
	})

	assert.Equal(t, "degraded", result.Status)
	assert.Equal(t, "context_cancelled", result.DegradationReason)
	assert.LessOrEqual(t, result.LLMCalls, 2)
	assert.LessOrEqual(t, result.ToolCalls, 1)
	assert.LessOrEqual(t, result.RAGCalls, 1)
}

func TestBaseExpertGenerateContentPassesPlanOutputTokenLimit(t *testing.T) {
	chatModel := &fakeExpertChatModel{content: "bounded output"}
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:      "linux_sre",
		MaxTokens: 4096,
		ChatModelFactory: func(context.Context) (einomodel.ToolCallingChatModel, error) {
			return chatModel, nil
		},
	}, NewToolRegistry())
	execution := expert.normalizeExecution(ExpertTask{Budget: ExecutionBudget{
		LLMCalls: 1, ToolCalls: 1, RAGCalls: 1, Timeout: time.Second, MaxRetrievalSteps: 1, MaxOutputTokens: 64,
	}})

	_, err := expert.generateContent(
		context.Background(),
		&belief.Frontier{NodeID: "hypothesis", Label: "CPU 饱和"},
		belief.NewBeliefGraph(),
		nil,
		map[string]string{"action": "analyze"},
		execution,
	)

	require.NoError(t, err)
	assert.Equal(t, 64, chatModel.maxTokensSeen)
}

func TestBaseExpert_Run_RAGTimeoutDegrades(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:              "test",
		MaxRetrievalSteps: 1,
		CallTimeout:       20 * time.Millisecond,
		GenerateContentFunc: func(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error) {
			return "CPU 高负载", nil
		},
		RAGQueryFunc: func(ctx context.Context, query string) ([]*einoschema.Document, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return []*einoschema.Document{{Content: "late result"}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)
	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "CPU 高负载",
		Why:    "CPU 使用率超过 90%",
	}

	start := time.Now()
	result := expert.Run(context.Background(), frontier, graph)

	assert.Less(t, time.Since(start), 150*time.Millisecond)
	assert.Equal(t, "degraded", result.Status)
	assert.Equal(t, "rag_retrieve_failed", result.DegradationReason)
	assert.NotEmpty(t, result.ToolErrors)
}
