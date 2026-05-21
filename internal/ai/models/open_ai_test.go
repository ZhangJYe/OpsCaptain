package models

import (
	"reflect"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIChatConfig_UnresolvedAPIKey(t *testing.T) {
	cfg, err := buildOpenAIChatConfig("deepseek-v4-flash", "${DEEPSEEK_API_KEY}", "https://api.deepseek.com")

	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "api_key")
}

func TestOpenAIChatModelFactory(t *testing.T) {
	assert.Equal(t, reflect.ValueOf(OpenAIForGLM).Pointer(), reflect.ValueOf(OpenAIChatModelFactory("chat_model")).Pointer())
	assert.Equal(t, reflect.ValueOf(OpenAIForGLM).Pointer(), reflect.ValueOf(OpenAIChatModelFactory("glm_chat_model")).Pointer())
	assert.Equal(t, reflect.ValueOf(OpenAIForGLMFast).Pointer(), reflect.ValueOf(OpenAIChatModelFactory("chat_model_fast")).Pointer())
	assert.Equal(t, reflect.ValueOf(OpenAIForGLMFast).Pointer(), reflect.ValueOf(OpenAIChatModelFactory("")).Pointer())
}

func TestBuildOpenAIChatConfig_ResolvedEnv(t *testing.T) {
	t.Setenv("TEST_MODEL_KEY", "test-key")

	cfg, err := buildOpenAIChatConfig("deepseek-v4-flash", "${TEST_MODEL_KEY}", "https://api.deepseek.com")

	require.NoError(t, err)
	assert.Equal(t, "deepseek-v4-flash", cfg.Model)
	assert.Equal(t, "test-key", cfg.APIKey)
	assert.Equal(t, "https://api.deepseek.com", cfg.BaseURL)
}

func TestDeepSeekForcedToolChoiceDowngraded(t *testing.T) {
	m := &instrumentedChatModel{modelName: "deepseek-v4-flash"}
	opts := m.compatibleOptions([]einomodel.Option{
		einomodel.WithToolChoice(schema.ToolChoiceForced, "plan"),
	})
	common := einomodel.GetCommonOptions(nil, opts...)

	assert.Nil(t, common.ToolChoice)
	assert.Empty(t, common.AllowedToolNames)
}

func TestNonDeepSeekKeepsForcedToolChoice(t *testing.T) {
	m := &instrumentedChatModel{modelName: "gpt-4.1"}
	opts := m.compatibleOptions([]einomodel.Option{
		einomodel.WithToolChoice(schema.ToolChoiceForced, "plan"),
	})
	common := einomodel.GetCommonOptions(nil, opts...)

	require.NotNil(t, common.ToolChoice)
	assert.Equal(t, schema.ToolChoiceForced, *common.ToolChoice)
	assert.Equal(t, []string{"plan"}, common.AllowedToolNames)
}

func TestInstrumentedChatModelImplementsInterface(t *testing.T) {
	var _ einomodel.ToolCallingChatModel = (*instrumentedChatModel)(nil)
}
