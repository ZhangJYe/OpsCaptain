package models

import (
	"reflect"
	"testing"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIChatModelFactory(t *testing.T) {
	assert.Equal(t, reflect.ValueOf(OpenAIForGLM).Pointer(), reflect.ValueOf(OpenAIChatModelFactory("chat_model")).Pointer())
	assert.Equal(t, reflect.ValueOf(OpenAIForGLMFast).Pointer(), reflect.ValueOf(OpenAIChatModelFactory("chat_model_fast")).Pointer())
	assert.Equal(t, reflect.ValueOf(OpenAIForGLMFast).Pointer(), reflect.ValueOf(OpenAIChatModelFactory("")).Pointer())
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
