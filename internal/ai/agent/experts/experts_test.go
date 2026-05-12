package experts

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
