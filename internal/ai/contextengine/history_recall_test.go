package contextengine

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
)

// mockEmbedder 用于测试的 mock embedder
type mockEmbedder struct {
	vectors map[string][]float64
	err     error
}

func (m *mockEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	if m.err != nil {
		return nil, m.err
	}
	result := make([][]float64, len(texts))
	for i, text := range texts {
		if vec, ok := m.vectors[text]; ok {
			result[i] = vec
		} else {
			// 默认向量
			result[i] = []float64{0.1, 0.2, 0.3}
		}
	}
	return result, nil
}

func newTestMessages() []*schema.Message {
	return []*schema.Message{
		{Role: schema.User, Content: "checkoutservice CPU 告警了"},
		{Role: schema.Assistant, Content: "让我帮你查一下 checkoutservice 的 CPU 使用率"},
		{Role: schema.User, Content: "Redis 连接超时怎么排查"},
		{Role: schema.Assistant, Content: "Redis 连接超时可以检查以下几个方面..."},
		{Role: schema.User, Content: "支付服务延迟高"},
		{Role: schema.Assistant, Content: "支付服务延迟高的原因可能是..."},
		{Role: schema.User, Content: "今天的告警多吗"},
		{Role: schema.Assistant, Content: "今天共有 15 条告警..."},
	}
}

func TestHistoryRecaller_Recall_EnoughHistory(t *testing.T) {
	messages := newTestMessages()
	recaller := NewHistoryRecaller()

	// 消息数 <= topK，直接返回
	result := recaller.Recall(context.Background(), "Redis 连接超时", messages, 10)
	assert.Len(t, result.Messages, 8)
	assert.False(t, result.Degraded)
}

func TestHistoryRecaller_Recall_DegradeWhenNoEmbedder(t *testing.T) {
	messages := newTestMessages()
	recaller := NewHistoryRecaller()
	// 不初始化 embedder，应该降级

	result := recaller.Recall(context.Background(), "Redis 连接超时", messages, 4)
	assert.True(t, result.Degraded)
	assert.Len(t, result.Messages, 4)
}

func TestHistoryRecaller_Recall_WithMockEmbedder(t *testing.T) {
	messages := newTestMessages()

	// 模拟 embedding：Redis 相关的消息应该有更高的相似度
	mock := &mockEmbedder{
		vectors: map[string][]float64{
			"checkoutservice CPU 告警了":           {0.1, 0.2, 0.3},
			"让我帮你查一下 checkoutservice 的 CPU 使用率": {0.1, 0.2, 0.3},
			"Redis 连接超时怎么排查":                    {0.9, 0.8, 0.7}, // 和 query 相似
			"Redis 连接超时可以检查以下几个方面...":           {0.9, 0.8, 0.7},
			"支付服务延迟高":                           {0.3, 0.4, 0.5},
			"支付服务延迟高的原因可能是...":                  {0.3, 0.4, 0.5},
			"今天的告警多吗":                           {0.2, 0.3, 0.4},
			"今天共有 15 条告警...":                    {0.2, 0.3, 0.4},
			"Redis 连接超时":                        {0.95, 0.85, 0.75}, // query 自身
		},
	}

	recaller := NewHistoryRecaller()
	recaller.embedder = mock

	result := recaller.Recall(context.Background(), "Redis 连接超时", messages, 4)
	assert.False(t, result.Degraded)
	// 应该召回 Redis 相关的消息
	assert.True(t, len(result.Messages) > 0)
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float64
		expected float64
	}{
		{"identical", []float64{1, 0, 0}, []float64{1, 0, 0}, 1.0},
		{"orthogonal", []float64{1, 0, 0}, []float64{0, 1, 0}, 0.0},
		{"opposite", []float64{1, 0, 0}, []float64{-1, 0, 0}, -1.0},
		{"empty", []float64{}, []float64{}, 0.0},
		{"different_length", []float64{1, 0}, []float64{1, 0, 0}, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cosineSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

func TestTopKWithThreshold(t *testing.T) {
	scored := []scoredMessage{
		{message: &schema.Message{Content: "a"}, score: 0.9, position: 0},
		{message: &schema.Message{Content: "b"}, score: 0.7, position: 1},
		{message: &schema.Message{Content: "c"}, score: 0.4, position: 2},
		{message: &schema.Message{Content: "d"}, score: 0.2, position: 3},
	}

	// 阈值 0.3，topK=3：应该返回 a, b, c（都 >= 0.3）
	result := topKWithThreshold(scored, 3, 0.3)
	assert.Len(t, result, 3)
	assert.Equal(t, "a", result[0].message.Content)
	assert.Equal(t, "b", result[1].message.Content)
	assert.Equal(t, "c", result[2].message.Content)

	// 阈值 0.5，topK=3：应该返回 a, b（c=0.4 低于阈值）
	result = topKWithThreshold(scored, 3, 0.5)
	assert.Len(t, result, 2)
	assert.Equal(t, "a", result[0].message.Content)
	assert.Equal(t, "b", result[1].message.Content)

	// 阈值过滤后没有结果，至少返回 top-1
	result = topKWithThreshold(scored, 3, 0.95)
	assert.Len(t, result, 1)
	assert.Equal(t, "a", result[0].message.Content)
}

func TestPairAwareSelect(t *testing.T) {
	history := []*schema.Message{
		{Role: schema.User, Content: "问题1"},
		{Role: schema.Assistant, Content: "回答1"},
		{Role: schema.User, Content: "问题2"},
		{Role: schema.Assistant, Content: "回答2"},
		{Role: schema.User, Content: "问题3"},
		{Role: schema.Assistant, Content: "回答3"},
	}

	// 选中 position=0 的 user 消息，应该自动保留 position=1 的 assistant
	scored := []scoredMessage{
		{message: history[0], score: 0.9, position: 0},
	}

	result := pairAwareSelect(scored, history)
	assert.Len(t, result, 2)
	assert.Equal(t, "问题1", result[0].message.Content)
	assert.Equal(t, "回答1", result[1].message.Content)

	// 选中 position=3 的 assistant 消息，应该自动保留 position=2 的 user
	scored = []scoredMessage{
		{message: history[3], score: 0.8, position: 3},
	}

	result = pairAwareSelect(scored, history)
	assert.Len(t, result, 2)
	assert.Equal(t, "问题2", result[0].message.Content)
	assert.Equal(t, "回答2", result[1].message.Content)
}

func TestPairAwareSelect_BoundaryCheck(t *testing.T) {
	history := []*schema.Message{
		{Role: schema.User, Content: "问题1"},
		{Role: schema.Assistant, Content: "回答1"},
	}

	// position 越界
	scored := []scoredMessage{
		{message: &schema.Message{Content: "test"}, score: 0.9, position: -1},
		{message: &schema.Message{Content: "test"}, score: 0.9, position: 100},
	}

	result := pairAwareSelect(scored, history)
	assert.Len(t, result, 0)
}

func TestEmbeddingCache(t *testing.T) {
	cache := newEmbeddingCache(3, 1*time.Minute)

	// Set 和 Get
	cache.Set("key1", []float64{1, 2, 3})
	vec, ok := cache.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, []float64{1, 2, 3}, vec)

	// 不存在的 key
	_, ok = cache.Get("key2")
	assert.False(t, ok)

	// LRU 淘汰
	cache.Set("key2", []float64{4, 5, 6})
	cache.Set("key3", []float64{7, 8, 9})
	cache.Set("key4", []float64{10, 11, 12}) // 应该淘汰 key1

	_, ok = cache.Get("key1")
	assert.False(t, ok)
	_, ok = cache.Get("key4")
	assert.True(t, ok)
}

func TestEmbeddingCache_TTL(t *testing.T) {
	cache := newEmbeddingCache(100, 50*time.Millisecond)

	cache.Set("key1", []float64{1, 2, 3})
	_, ok := cache.Get("key1")
	assert.True(t, ok)

	// 等待过期
	time.Sleep(60 * time.Millisecond)
	_, ok = cache.Get("key1")
	assert.False(t, ok)
}

func TestFallbackPositional(t *testing.T) {
	messages := []*schema.Message{
		{Content: "a"},
		{Content: "b"},
		{Content: "c"},
		{Content: "d"},
		{Content: "e"},
	}

	// topK < len，取最后 3 条
	result := fallbackPositional(messages, 3)
	assert.Len(t, result, 3)
	assert.Equal(t, "c", result[0].Content)
	assert.Equal(t, "d", result[1].Content)
	assert.Equal(t, "e", result[2].Content)

	// topK >= len，返回全部
	result = fallbackPositional(messages, 10)
	assert.Len(t, result, 5)
}

func TestTruncateForEmbedding(t *testing.T) {
	short := "hello"
	assert.Equal(t, "hello", truncateForEmbedding(short, 10))

	long := "abcdefghijklmnopqrstuvwxyz"
	assert.Equal(t, "abcdefghij...", truncateForEmbedding(long, 10))
}

func TestContentHash(t *testing.T) {
	hash1 := contentHash("hello")
	hash2 := contentHash("hello")
	hash3 := contentHash("world")

	assert.Equal(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
}
