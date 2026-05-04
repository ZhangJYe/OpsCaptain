package contextengine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolRecaller_Recall_SmallList(t *testing.T) {
	items := []ContextItem{
		{Title: "log-1", Content: "connection timeout to Redis"},
		{Title: "log-2", Content: "CPU usage 87%"},
	}

	recaller := NewToolRecaller()
	result := recaller.Recall(context.Background(), "Redis 连接超时", items, 10)
	assert.Len(t, result.Items, 2)
}

func TestToolRecaller_Recall_KeywordMatch(t *testing.T) {
	items := []ContextItem{
		{Title: "log-1", Content: "connection timeout to Redis"},
		{Title: "log-2", Content: "CPU usage 87% on checkoutservice"},
		{Title: "log-3", Content: "Redis connection refused"},
		{Title: "log-4", Content: "payment service latency high"},
		{Title: "log-5", Content: "normal log entry"},
	}

	recaller := NewToolRecaller()
	result := recaller.Recall(context.Background(), "Redis 连接超时", items, 3)

	// 应该召回 Redis 相关的 log-1 和 log-3
	assert.Len(t, result.Items, 3)
	assert.Equal(t, "log-1", result.Items[0].Title)
}

func TestToolRecaller_Recall_MetadataWeight(t *testing.T) {
	items := []ContextItem{
		{Title: "Redis timeout report", Content: "some content", SourceType: "alert"},
		{Title: "log-2", Content: "Redis connection timeout in log"},
	}

	recaller := NewToolRecaller()
	result := recaller.Recall(context.Background(), "Redis 超时", items, 2)

	// Title 匹配的应该分数更高
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "Redis timeout report", result.Items[0].Title)
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected []string
	}{
		{
			name:     "英文服务名",
			query:    "checkoutservice CPU 告警",
			expected: []string{"checkoutservice", "告警", "alert"},
		},
		{
			name:     "中文运维术语",
			query:    "Redis 连接超时怎么排查",
			expected: []string{"redis", "timeout", "超时", "connection", "连接"},
		},
		{
			name:     "错误码",
			query:    "支付服务报 503 了",
			expected: []string{"503"},
		},
		{
			name:     "指标名",
			query:    "CPU 和 memory 使用率高",
			expected: []string{"cpu", "memory"},
		},
		{
			name:     "混合",
			query:    "paymentservice 延迟飙升",
			expected: []string{"paymentservice", "latency", "延迟", "spike", "飙升"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keywords := extractKeywords(tt.query)
			for _, expected := range tt.expected {
				assert.Contains(t, keywords, expected)
			}
		})
	}
}

func TestMatchScore(t *testing.T) {
	item := ContextItem{
		Title:      "Redis timeout report",
		SourceType: "alert",
		SourceID:   "alert-001",
		Content:    "connection timeout to Redis server",
	}

	keywords := []string{"redis", "timeout"}
	score := matchScore(keywords, item)
	assert.True(t, score > 0)

	// 空关键词
	score = matchScore([]string{}, item)
	assert.Equal(t, 0.0, score)
}

func TestMatchScore_MetadataHigherWeight(t *testing.T) {
	itemMeta := ContextItem{
		Title:   "Redis timeout",
		Content: "some unrelated content",
	}

	itemContent := ContextItem{
		Title:   "unrelated title",
		Content: "Redis timeout in logs",
	}

	keywords := []string{"redis", "timeout"}

	scoreMeta := matchScore(keywords, itemMeta)
	scoreContent := matchScore(keywords, itemContent)

	// Title 匹配权重更高
	assert.True(t, scoreMeta > scoreContent)
}

func TestUniqueStrings(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b", ""}
	result := uniqueStrings(input)
	assert.Len(t, result, 3)
	assert.Contains(t, result, "a")
	assert.Contains(t, result, "b")
	assert.Contains(t, result, "c")
}
