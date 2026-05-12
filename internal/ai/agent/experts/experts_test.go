package experts

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/belief"
)

func TestGetArgBuilder(t *testing.T) {
	builder := GetArgBuilder("query_internal_docs")
	assert.IsType(t, &QueryArgBuilder{}, builder)

	builder = GetArgBuilder("query_logs")
	assert.IsType(t, &QueryArgBuilder{}, builder)

	builder = GetArgBuilder("get_current_time")
	assert.IsType(t, &RawArgBuilder{}, builder)

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
	assert.False(t, result.Degraded)
	assert.Empty(t, result.Error)
}

func TestParseToolOutput_Degraded(t *testing.T) {
	output := `{"success": false, "degraded": true, "error": "partial data"}`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.True(t, result.Degraded)
	assert.Equal(t, "partial data", result.Error)
}

func TestParseToolOutput_Failed(t *testing.T) {
	output := `{"success": false, "error": "connection timeout"}`
	result := parseToolOutput(output)
	assert.False(t, result.Success)
	assert.False(t, result.Degraded)
	assert.Equal(t, "connection timeout", result.Error)
}

func TestParseToolOutput_InvalidJSON(t *testing.T) {
	output := `not json`
	result := parseToolOutput(output)
	assert.True(t, result.Success)
	assert.Equal(t, output, result.Data)
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

	decision, err := expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{})
	require.NoError(t, err)
	assert.Equal(t, "retrieve", decision["action"])

	decision, err = expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{{Query: "q1", Output: "o1"}})
	require.NoError(t, err)
	assert.Equal(t, "retrieve", decision["action"])

	decision, err = expert.makeDecision(context.Background(), frontier, graph, []RetrievalRecord{
		{Query: "q1", Output: "o1"},
		{Query: "q2", Output: "o2"},
	})
	require.NoError(t, err)
	assert.Equal(t, "analyze", decision["action"])
}

func TestBaseExpert_GenerateContent(t *testing.T) {
	cfg := ExpertRuntimeConfig{
		Name:      "test",
		ToolNames: []string{"query_logs"},
	}
	toolReg := NewToolRegistry()
	expert := NewBaseExpert(cfg, toolReg)

	graph := belief.NewBeliefGraph()
	frontier := &belief.Frontier{
		NodeID: "test",
		Label:  "CPU 高负载",
		Why:    "CPU 使用率超过 90%",
	}

	content, err := expert.generateContent(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]string{
		"action": "tool_call",
	})
	require.NoError(t, err)
	assert.Contains(t, content, "CPU 高负载")

	content, err = expert.generateContent(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]string{
		"action": "retrieve",
	})
	require.NoError(t, err)
	assert.Contains(t, content, "CPU 高负载")

	content, err = expert.generateContent(context.Background(), frontier, graph, []RetrievalRecord{}, map[string]string{
		"action":     "analyze",
		"confidence": "0.8",
	})
	require.NoError(t, err)
	assert.Contains(t, content, "CPU 高负载")
}
