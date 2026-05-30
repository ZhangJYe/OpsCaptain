package contextengine

import (
	"SuperBizAgent/internal/ai/embedder"
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultHistoryRecallTimeout = 500 * time.Millisecond
	defaultRecentKeep           = 3
	defaultSimilarityThreshold  = 0.3
	defaultCacheTTL             = 10 * time.Minute
	defaultCacheMaxSize         = 1000
	assistantTruncateLen        = 200
)

// HistoryRecallResult 包含召回结果和 trace
type HistoryRecallResult struct {
	Messages  []*schema.Message
	Scores    []float64
	CacheHits int
	Embedded  int
	LatencyMs int64
	Degraded  bool
}

// HistoryRecaller 基于 embedding 相似度召回历史消息
type HistoryRecaller struct {
	embedder   embedding.Embedder
	cache      *embeddingCache
	timeout    time.Duration
	recentKeep int
	threshold  float64
	embedOnce  sync.Once
	embedErr   error
}

// NewHistoryRecaller 创建 HistoryRecaller
func NewHistoryRecaller() *HistoryRecaller {
	return &HistoryRecaller{
		cache:      newEmbeddingCache(defaultCacheMaxSize, defaultCacheTTL),
		timeout:    defaultHistoryRecallTimeout,
		recentKeep: defaultRecentKeep,
		threshold:  defaultSimilarityThreshold,
	}
}

func (r *HistoryRecaller) ensureEmbedder(ctx context.Context) error {
	if r.embedder != nil {
		return nil
	}
	r.embedOnce.Do(func() {
		r.embedder, r.embedErr = embedder.DoubaoEmbedding(ctx)
	})
	return r.embedErr
}

// Recall 基于 embedding 相似度召回历史消息
func (r *HistoryRecaller) Recall(ctx context.Context, query string, history []*schema.Message, topK int) HistoryRecallResult {
	start := time.Now()

	// 快速路径：历史消息不多，直接返回
	if len(history) <= topK {
		return HistoryRecallResult{
			Messages:  history,
			Scores:    make([]float64, len(history)),
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}

	// 确保 embedder 可用
	if err := r.ensureEmbedder(ctx); err != nil {
		return r.degradedResult(history, topK, start)
	}

	// 分离最近消息和候选消息
	recentCount := min(r.recentKeep, len(history))
	recent := history[len(history)-recentCount:]
	candidates := history[:len(history)-recentCount]

	// 带超时的 embedding 召回
	recallCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	recalled, err := r.recallByEmbedding(recallCtx, query, candidates, topK-recentCount)
	if err != nil {
		return r.degradedResult(history, topK, start)
	}

	// 合并并排序
	merged := mergeAndSort(recalled.messages, recent, history)

	return HistoryRecallResult{
		Messages:  merged,
		Scores:    recalled.scores,
		CacheHits: recalled.cacheHits,
		Embedded:  recalled.embedded,
		LatencyMs: time.Since(start).Milliseconds(),
		Degraded:  false,
	}
}

func (r *HistoryRecaller) degradedResult(history []*schema.Message, topK int, start time.Time) HistoryRecallResult {
	return HistoryRecallResult{
		Messages:  fallbackPositional(history, topK),
		Scores:    make([]float64, min(topK, len(history))),
		LatencyMs: time.Since(start).Milliseconds(),
		Degraded:  true,
	}
}

type recallResult struct {
	messages  []*schema.Message
	scores    []float64
	cacheHits int
	embedded  int
}

func (r *HistoryRecaller) recallByEmbedding(ctx context.Context, query string, candidates []*schema.Message, topK int) (*recallResult, error) {
	if len(candidates) == 0 || topK <= 0 {
		return &recallResult{}, nil
	}

	// 1. 计算 query embedding
	queryText := truncateForEmbedding(query, assistantTruncateLen)
	queryVecs, err := r.embedder.EmbedStrings(ctx, []string{queryText})
	if err != nil {
		return nil, err
	}
	if len(queryVecs) == 0 {
		return nil, fmt.Errorf("empty query embedding")
	}
	queryVec := queryVecs[0]

	// 2. 获取 candidates embeddings（带缓存 + batch）
	vecs, cacheHits, err := r.getEmbeddings(ctx, candidates)
	if err != nil {
		return nil, err
	}

	// 3. 计算相似度 + 确定性权重增强
	totalCandidates := len(candidates)
	scored := make([]scoredMessage, totalCandidates)
	for i, msg := range candidates {
		cosine := cosineSimilarity(queryVec, vecs[i])
		combined := combinedHistoryScore(cosine, i, totalCandidates, msg, query)
		scored[i] = scoredMessage{
			message:  msg,
			score:    combined,
			position: i,
		}
	}

	// 4. 按分数排序，取 top-K（带阈值）
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})
	selected := topKWithThreshold(scored, topK, r.threshold)

	// 5. 成对保留
	paired := pairAwareSelect(selected, candidates)

	// 6. 提取结果
	messages := make([]*schema.Message, 0, len(paired))
	scores := make([]float64, 0, len(paired))
	for _, s := range paired {
		messages = append(messages, s.message)
		scores = append(scores, s.score)
	}

	return &recallResult{
		messages:  messages,
		scores:    scores,
		cacheHits: cacheHits,
		embedded:  len(candidates) - cacheHits,
	}, nil
}

func (r *HistoryRecaller) getEmbeddings(ctx context.Context, messages []*schema.Message) ([][]float64, int, error) {
	vecs := make([][]float64, len(messages))
	toEmbedIdx := make([]int, 0)
	toEmbedTexts := make([]string, 0)
	cacheHits := 0

	// 1. 查缓存
	for i, msg := range messages {
		text := truncateForEmbedding(msg.Content, assistantTruncateLen)
		hash := contentHash(text)
		if cached, ok := r.cache.Get(hash); ok {
			vecs[i] = cached
			cacheHits++
		} else {
			toEmbedIdx = append(toEmbedIdx, i)
			toEmbedTexts = append(toEmbedTexts, text)
		}
	}

	// 2. Batch embed 未缓存的
	if len(toEmbedTexts) > 0 {
		embedded, err := r.embedder.EmbedStrings(ctx, toEmbedTexts)
		if err != nil {
			return nil, 0, err
		}
		for j, idx := range toEmbedIdx {
			vecs[idx] = embedded[j]
			r.cache.Set(contentHash(toEmbedTexts[j]), embedded[j])
		}
	}

	return vecs, cacheHits, nil
}

// scoredMessage 带分数的消息
type scoredMessage struct {
	message  *schema.Message
	score    float64
	position int
}

// topKWithThreshold 取 top-K，但分数低于阈值的不取
func topKWithThreshold(scored []scoredMessage, topK int, threshold float64) []scoredMessage {
	selected := make([]scoredMessage, 0, topK)
	for _, s := range scored {
		if len(selected) >= topK {
			break
		}
		if s.score < threshold {
			break
		}
		selected = append(selected, s)
	}
	// 如果阈值过滤后没有结果，至少返回 top-1（兜底）
	if len(selected) == 0 && len(scored) > 0 {
		selected = append(selected, scored[0])
	}
	return selected
}

// pairAwareSelect 成对保留 user+assistant 消息
func pairAwareSelect(scored []scoredMessage, history []*schema.Message) []scoredMessage {
	selected := make([]scoredMessage, 0, len(scored)*2)
	used := make(map[int]bool)

	for _, s := range scored {
		if used[s.position] {
			continue
		}

		// 边界检查
		if s.position < 0 || s.position >= len(history) {
			continue
		}

		selected = append(selected, s)
		used[s.position] = true

		// 选中 user 消息时，强制保留下一条 assistant 消息
		if s.message.Role == schema.User && s.position+1 < len(history) {
			next := history[s.position+1]
			if next.Role == schema.Assistant && !used[s.position+1] {
				selected = append(selected, scoredMessage{
					message:  next,
					score:    s.score, // 继承 user 的分数
					position: s.position + 1,
				})
				used[s.position+1] = true
			}
		}

		// 选中 assistant 消息时，强制保留前一条 user 消息
		if s.message.Role == schema.Assistant && s.position-1 >= 0 {
			prev := history[s.position-1]
			if prev.Role == schema.User && !used[s.position-1] {
				selected = append(selected, scoredMessage{
					message:  prev,
					score:    s.score, // 继承 assistant 的分数
					position: s.position - 1,
				})
				used[s.position-1] = true
			}
		}
	}

	// 按原始位置排序，保持时序
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].position < selected[j].position
	})

	return selected
}

// mergeAndSort 合并召回结果和最近消息，按原始位置排序
func mergeAndSort(recalled []*schema.Message, recent []*schema.Message, history []*schema.Message) []*schema.Message {
	seen := make(map[*schema.Message]bool)
	merged := make([]*schema.Message, 0, len(recalled)+len(recent))

	for _, msg := range recalled {
		if !seen[msg] {
			merged = append(merged, msg)
			seen[msg] = true
		}
	}
	for _, msg := range recent {
		if !seen[msg] {
			merged = append(merged, msg)
			seen[msg] = true
		}
	}

	// 按原始位置排序
	sort.Slice(merged, func(i, j int) bool {
		return findPosition(merged[i], history) < findPosition(merged[j], history)
	})

	return merged
}

// fallbackPositional 降级：从后往前取 N 条
func fallbackPositional(history []*schema.Message, topK int) []*schema.Message {
	if len(history) <= topK {
		return history
	}
	return history[len(history)-topK:]
}

// findPosition 在 history 中查找消息的位置
func findPosition(msg *schema.Message, history []*schema.Message) int {
	for i, m := range history {
		if m == msg {
			return i
		}
	}
	return len(history) // 未找到放最后
}

// truncateForEmbedding 截断文本用于 embedding
func truncateForEmbedding(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

// contentHash 计算内容的 hash
func contentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8])
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

const (
	recencyDecay     = 0.9
	roleUserBonus    = 0.2
	entityMatchBonus = 0.3
)

func combinedHistoryScore(cosine float64, position int, total int, msg *schema.Message, query string) float64 {
	boost := 0.0
	boost += historyRecencyBoost(position, total)
	boost += historyRoleBoost(msg)
	boost += historyEntityBoost(msg, query)
	return cosine * (1.0 + boost)
}

func historyRecencyBoost(position int, total int) float64 {
	if total <= 1 {
		return 0
	}
	distanceFromEnd := total - 1 - position
	return 0.3 * math.Pow(recencyDecay, float64(distanceFromEnd))
}

func historyRoleBoost(msg *schema.Message) float64 {
	if msg.Role == schema.User {
		return roleUserBonus
	}
	return 0
}

func historyEntityBoost(msg *schema.Message, query string) float64 {
	lowerQuery := strings.ToLower(query)
	lowerContent := strings.ToLower(msg.Content)

	if serviceNamePattern.MatchString(lowerQuery) && serviceNamePattern.MatchString(lowerContent) {
		queryServices := serviceNamePattern.FindAllString(lowerQuery, -1)
		contentServices := serviceNamePattern.FindAllString(lowerContent, -1)
		for _, qs := range queryServices {
			for _, cs := range contentServices {
				if qs == cs {
					return entityMatchBonus
				}
			}
		}
	}

	queryEntities := entityPattern.FindAllString(lowerQuery, -1)
	contentEntities := entityPattern.FindAllString(lowerContent, -1)
	for _, qe := range queryEntities {
		for _, ce := range contentEntities {
			if qe == ce {
				return entityMatchBonus
			}
		}
	}

	return 0
}

// embeddingCache LRU 缓存
type embeddingCache struct {
	mu      sync.RWMutex
	items   map[string]*cacheEntry
	order   []string
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	vec       []float64
	expiresAt time.Time
}

func newEmbeddingCache(maxSize int, ttl time.Duration) *embeddingCache {
	return &embeddingCache{
		items:   make(map[string]*cacheEntry),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

func (c *embeddingCache) Get(key string) ([]float64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.items[key]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.vec, true
}

func (c *embeddingCache) Set(key string, vec []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 已存在则更新
	if _, ok := c.items[key]; ok {
		c.items[key] = &cacheEntry{vec: vec, expiresAt: time.Now().Add(c.ttl)}
		// 移到末尾（LRU）
		for i, k := range c.order {
			if k == key {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
		c.order = append(c.order, key)
		return
	}

	// 淘汰最旧的
	for len(c.order) >= c.maxSize {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.items, oldest)
	}

	c.items[key] = &cacheEntry{vec: vec, expiresAt: time.Now().Add(c.ttl)}
	c.order = append(c.order, key)
}
