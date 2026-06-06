package contextengine

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestParseRerankScores_JSON(t *testing.T) {
	resp := `{"scores": [{"id": 1, "score": 9}, {"id": 2, "score": 2}, {"id": 3, "score": 10}]}`
	scores, ok := parseRerankScores(resp, 3)
	assert.True(t, ok)
	assert.Len(t, scores, 3)
	assert.InDelta(t, 9.0, scores[0], 0.01)
	assert.InDelta(t, 2.0, scores[1], 0.01)
	assert.InDelta(t, 10.0, scores[2], 0.01)
}

func TestParseRerankScores_JSONOutOfBounds(t *testing.T) {
	resp := `{"scores": [{"id": 1, "score": 9}, {"id": 5, "score": 7}]}`
	scores, ok := parseRerankScores(resp, 3)
	assert.True(t, ok)
	assert.Len(t, scores, 3)
	assert.InDelta(t, 9.0, scores[0], 0.01)
	assert.InDelta(t, 0.0, scores[1], 0.01)
	assert.InDelta(t, 0.0, scores[2], 0.01)
}

func TestParseRerankScores_JSONClamp(t *testing.T) {
	resp := `{"scores": [{"id": 1, "score": 15}, {"id": 2, "score": -3}]}`
	scores, ok := parseRerankScores(resp, 2)
	assert.True(t, ok)
	assert.InDelta(t, 10.0, scores[0], 0.01)
	assert.InDelta(t, 0.0, scores[1], 0.01)
}

func TestParseRerankScores_FallbackRegex(t *testing.T) {
	resp := "[1] 9\n[2] 2\n[3] 10"
	scores, ok := parseRerankScores(resp, 3)
	assert.True(t, ok)
	assert.Len(t, scores, 3)
	assert.InDelta(t, 9.0, scores[0], 0.01)
	assert.InDelta(t, 2.0, scores[1], 0.01)
	assert.InDelta(t, 10.0, scores[2], 0.01)
}

func TestParseRerankScores_FallbackRegexDecimal(t *testing.T) {
	resp := "[1] 8.5\n[2] 3.2"
	scores, ok := parseRerankScores(resp, 2)
	assert.True(t, ok)
	assert.InDelta(t, 8.5, scores[0], 0.01)
	assert.InDelta(t, 3.2, scores[1], 0.01)
}

func TestParseRerankScores_Invalid(t *testing.T) {
	resp := "I don't understand the question"
	_, ok := parseRerankScores(resp, 3)
	assert.False(t, ok)
}

func TestSanitizeSnippets_IP(t *testing.T) {
	items := []ContextItem{
		{Content: "connection to 10.0.1.5:6379 failed"},
	}
	result := sanitizeSnippets(items)
	assert.Contains(t, result[0].Content, "[private-ip]")
	assert.NotContains(t, result[0].Content, "10.0.1.5")
}

func TestSanitizeSnippets_Token(t *testing.T) {
	items := []ContextItem{
		{Content: "token=abc123secret value"},
	}
	result := sanitizeSnippets(items)
	assert.Contains(t, result[0].Content, "[redacted]")
}

func TestSanitizeSnippets_UUID(t *testing.T) {
	items := []ContextItem{
		{Content: "pod uid: 550e8400-e29b-41d4-a716-446655440000"},
	}
	result := sanitizeSnippets(items)
	assert.Contains(t, result[0].Content, "[uuid]")
	assert.NotContains(t, result[0].Content, "550e8400")
}

func TestSanitizeSnippets_Truncate(t *testing.T) {
	longContent := make([]byte, 300)
	for i := range longContent {
		longContent[i] = 'a'
	}
	items := []ContextItem{
		{Content: string(longContent)},
	}
	result := sanitizeSnippets(items)
	assert.LessOrEqual(t, len(result[0].Content), snippetTruncateLen+3)
}

func TestSanitizeSnippets_TruncateKeepsUTF8(t *testing.T) {
	content := strings.Repeat("知识库证据", 80)
	items := []ContextItem{
		{Content: content},
	}

	result := sanitizeSnippets(items)
	assert.True(t, utf8.ValidString(result[0].Content))
	assert.LessOrEqual(t, utf8.RuneCountInString(result[0].Content), snippetTruncateLen+3)
	assert.Contains(t, result[0].Content, "...")
}

func TestSanitizeSnippets_NoChange(t *testing.T) {
	items := []ContextItem{
		{Content: "normal log entry without sensitive data"},
	}
	result := sanitizeSnippets(items)
	assert.Equal(t, items[0].Content, result[0].Content)
}

func TestTakeTopK(t *testing.T) {
	items := []ContextItem{
		{Content: "a"}, {Content: "b"}, {Content: "c"}, {Content: "d"},
	}
	result := takeTopK(items, 2)
	assert.Len(t, result, 2)
	assert.Equal(t, "a", result[0].Content)
	assert.Equal(t, "b", result[1].Content)

	result = takeTopK(items, 10)
	assert.Len(t, result, 4)
}

func TestBuildRerankPrompt(t *testing.T) {
	items := []ContextItem{
		{Title: "log-1", SourceType: "log", Content: "connection timeout"},
	}
	prompt := buildRerankPrompt("Redis 超时", items)
	assert.Contains(t, prompt, "Redis 超时")
	assert.Contains(t, prompt, "[1]")
	assert.Contains(t, prompt, "connection timeout")
	assert.Contains(t, prompt, "JSON")
}

func TestBuildRerankPrompt_TruncateKeepsUTF8(t *testing.T) {
	items := []ContextItem{
		{Title: "doc-1", SourceType: "doc", Content: strings.Repeat("中文证据", 80)},
	}

	prompt := buildRerankPrompt("查询", items)
	assert.True(t, utf8.ValidString(prompt))
	assert.Contains(t, prompt, "...")
}

func TestNewToolReranker_Defaults(t *testing.T) {
	reranker := NewToolReranker(nil, nil)
	assert.NotNil(t, reranker)
	assert.False(t, reranker.config.Enabled)
	assert.Equal(t, defaultToolRerankCandidateMax, reranker.config.CandidateLimit)
	assert.Equal(t, defaultToolRerankMinCandidates, reranker.config.MinCandidates)
}

func TestToolReranker_Rerank_Disabled(t *testing.T) {
	reranker := NewToolReranker(nil, &ToolRerankConfig{Enabled: false})
	items := []ContextItem{
		{Content: "a"}, {Content: "b"},
	}
	outcome := reranker.Rerank(nil, "test", items, 2)
	assert.False(t, outcome.Enabled)
	assert.Equal(t, "disabled_or_too_few", outcome.Reason)
	assert.Len(t, outcome.Items, 2)
}

func TestToolReranker_Rerank_EmptyItems(t *testing.T) {
	reranker := NewToolReranker(nil, nil)
	outcome := reranker.Rerank(nil, "test", nil, 5)
	assert.Equal(t, "empty_candidates", outcome.Reason)
}

func TestToolReranker_Rerank_TooFewCandidates(t *testing.T) {
	reranker := NewToolReranker(nil, &ToolRerankConfig{
		Enabled:       true,
		MinCandidates: 10,
	})
	items := []ContextItem{{Content: "a"}, {Content: "b"}}
	outcome := reranker.Rerank(nil, "test", items, 2)
	assert.Equal(t, "disabled_or_too_few", outcome.Reason)
}
