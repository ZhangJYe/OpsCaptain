package contextengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseIntent_JSON(t *testing.T) {
	intent, ok := parseIntent(`{"type": "fault_diagnosis"}`)
	assert.True(t, ok)
	assert.Equal(t, IntentFaultDiagnosis, intent)
}

func TestParseIntent_JSONKnowledge(t *testing.T) {
	intent, ok := parseIntent(`{"type": "knowledge_query"}`)
	assert.True(t, ok)
	assert.Equal(t, IntentKnowledgeQuery, intent)
}

func TestParseIntent_JSONChat(t *testing.T) {
	intent, ok := parseIntent(`{"type": "chat"}`)
	assert.True(t, ok)
	assert.Equal(t, IntentChat, intent)
}

func TestParseIntent_JSONInvalid(t *testing.T) {
	_, ok := parseIntent(`{"type": "unknown_type"}`)
	assert.False(t, ok)
}

func TestParseIntent_FallbackChinese(t *testing.T) {
	tests := []struct {
		resp     string
		expected IntentType
	}{
		{"这是一个故障排查问题", IntentFaultDiagnosis},
		{"这是知识查询", IntentKnowledgeQuery},
		{"闲聊一下", IntentChat},
	}
	for _, tt := range tests {
		intent, ok := parseIntent(tt.resp)
		assert.True(t, ok, "resp: %s", tt.resp)
		assert.Equal(t, tt.expected, intent, "resp: %s", tt.resp)
	}
}

func TestParseIntent_FallbackEnglish(t *testing.T) {
	tests := []struct {
		resp     string
		expected IntentType
	}{
		{"fault_diagnosis", IntentFaultDiagnosis},
		{"knowledge_query", IntentKnowledgeQuery},
		{"chat", IntentChat},
	}
	for _, tt := range tests {
		intent, ok := parseIntent(tt.resp)
		assert.True(t, ok, "resp: %s", tt.resp)
		assert.Equal(t, tt.expected, intent, "resp: %s", tt.resp)
	}
}

func TestParseIntent_FallbackMixed(t *testing.T) {
	intent, ok := parseIntent("我认为这是 fault_diagnosis 类型")
	assert.True(t, ok)
	assert.Equal(t, IntentFaultDiagnosis, intent)
}

func TestParseIntent_Invalid(t *testing.T) {
	_, ok := parseIntent("I don't know")
	assert.False(t, ok)
}

func TestParseIntent_Empty(t *testing.T) {
	_, ok := parseIntent("")
	assert.False(t, ok)
}

func TestProfileForIntent(t *testing.T) {
	assert.Equal(t, "aiops_diagnosis", ProfileForIntent(IntentFaultDiagnosis))
	assert.Equal(t, "chat", ProfileForIntent(IntentKnowledgeQuery))
	assert.Equal(t, "chat", ProfileForIntent(IntentChat))
	assert.Equal(t, "chat", ProfileForIntent(IntentUnknown))
}

func TestNewIntentRecognizer(t *testing.T) {
	r := NewIntentRecognizer(nil)
	assert.NotNil(t, r)
	assert.Equal(t, defaultIntentTimeout, r.timeout)
}
