package contextengine

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultToolRerankTimeout       = 2 * time.Second
	defaultToolRerankCandidateMax  = 20
	defaultToolRerankMinCandidates = 6
	snippetTruncateLen             = 200
)

type ToolRerankConfig struct {
	Enabled        bool
	Profiles       []string
	MinCandidates  int
	CandidateLimit int
	TimeoutMs      int
	CacheTTLSecs   int
	Model          string
}

type RerankOutcome struct {
	Items          []ContextItem
	Enabled        bool
	Degraded       bool
	Reason         string
	CandidateCount int
	LatencyMs      int64
	Scores         []float64
}

type ToolReranker struct {
	modelFactory func(ctx context.Context) (model.ToolCallingChatModel, error)
	config       *ToolRerankConfig
	modelOnce    sync.Once
	model        model.ToolCallingChatModel
	modelErr     error
}

func NewToolReranker(modelFactory func(ctx context.Context) (model.ToolCallingChatModel, error), config *ToolRerankConfig) *ToolReranker {
	if config == nil {
		config = &ToolRerankConfig{Enabled: false}
	}
	if config.CandidateLimit <= 0 {
		config.CandidateLimit = defaultToolRerankCandidateMax
	}
	if config.MinCandidates <= 0 {
		config.MinCandidates = defaultToolRerankMinCandidates
	}
	if config.TimeoutMs <= 0 {
		config.TimeoutMs = int(defaultToolRerankTimeout / time.Millisecond)
	}
	return &ToolReranker{
		modelFactory: modelFactory,
		config:       config,
	}
}

func (r *ToolReranker) ensureModel(ctx context.Context) error {
	r.modelOnce.Do(func() {
		r.model, r.modelErr = r.modelFactory(ctx)
	})
	return r.modelErr
}

func (r *ToolReranker) Rerank(ctx context.Context, query string, items []ContextItem, topK int) RerankOutcome {
	if len(items) == 0 {
		return RerankOutcome{Reason: "empty_candidates"}
	}
	if !r.config.Enabled || len(items) < r.config.MinCandidates {
		return RerankOutcome{Items: takeTopK(items, topK), Reason: "disabled_or_too_few"}
	}

	start := time.Now()

	if err := r.ensureModel(ctx); err != nil {
		return RerankOutcome{
			Items:     takeTopK(items, topK),
			Enabled:   true,
			Degraded:  true,
			Reason:    "model_init_failed",
			LatencyMs: time.Since(start).Milliseconds(),
		}
	}

	candidates := items
	if len(candidates) > r.config.CandidateLimit {
		candidates = candidates[:r.config.CandidateLimit]
	}

	sanitized := sanitizeSnippets(candidates)

	timeout := time.Duration(r.config.TimeoutMs) * time.Millisecond
	rerankCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	scores, err := r.scoreCandidates(rerankCtx, query, sanitized)
	if err != nil {
		return RerankOutcome{
			Items:          takeTopK(items, topK),
			Enabled:        true,
			Degraded:       true,
			Reason:         err.Error(),
			CandidateCount: len(candidates),
			LatencyMs:      time.Since(start).Milliseconds(),
		}
	}

	selected := r.selectByScore(candidates, scores, topK)
	return RerankOutcome{
		Items:          selected,
		Enabled:        true,
		CandidateCount: len(candidates),
		LatencyMs:      time.Since(start).Milliseconds(),
		Scores:         scores,
	}
}

func (r *ToolReranker) scoreCandidates(ctx context.Context, query string, items []ContextItem) ([]float64, error) {
	prompt := buildRerankPrompt(query, items)
	resp, err := r.model.Generate(ctx, []*schema.Message{
		schema.UserMessage(prompt),
	})
	if err != nil {
		return nil, err
	}
	scores, ok := parseRerankScores(resp.Content, len(items))
	if !ok {
		return nil, fmt.Errorf("parse_rerank_scores_failed")
	}
	return scores, nil
}

func (r *ToolReranker) selectByScore(items []ContextItem, scores []float64, topK int) []ContextItem {
	type scored struct {
		item  ContextItem
		score float64
	}
	scoredItems := make([]scored, len(items))
	for i, item := range items {
		s := 0.0
		if i < len(scores) {
			s = scores[i]
		}
		scoredItems[i] = scored{item: item, score: s}
	}
	sort.Slice(scoredItems, func(i, j int) bool {
		return scoredItems[i].score > scoredItems[j].score
	})
	result := make([]ContextItem, 0, min(topK, len(scoredItems)))
	for i := 0; i < min(topK, len(scoredItems)); i++ {
		result = append(result, scoredItems[i].item)
	}
	return result
}

func takeTopK(items []ContextItem, topK int) []ContextItem {
	if len(items) <= topK {
		return items
	}
	return items[:topK]
}

func buildRerankPrompt(query string, items []ContextItem) string {
	var sb strings.Builder
	sb.WriteString("你是运维专家。判断以下工具结果和用户问题的相关性。\n\n")
	sb.WriteString(fmt.Sprintf("用户问题：%s\n\n", query))
	sb.WriteString("工具结果（已脱敏和裁剪）：\n")
	for i, item := range items {
		text := item.Content
		text = truncateSnippetText(text, snippetTruncateLen)
		sb.WriteString(fmt.Sprintf("[%d] source=%s title=%s content=%s\n", i+1, item.SourceType, item.Title, text))
	}
	sb.WriteString("\n严格按以下 JSON 格式输出，不要添加任何其他文字：\n")
	sb.WriteString(`{"scores": [{"id": 1, "score": 9}, {"id": 2, "score": 2}]}`)
	return sb.String()
}

func parseRerankScores(resp string, expectedCount int) ([]float64, bool) {
	type scoreEntry struct {
		ID    int     `json:"id"`
		Score float64 `json:"score"`
	}
	type rerankResult struct {
		Scores []scoreEntry `json:"scores"`
	}

	var result rerankResult
	if err := json.Unmarshal([]byte(resp), &result); err == nil && len(result.Scores) > 0 {
		scores := make([]float64, expectedCount)
		for _, entry := range result.Scores {
			idx := entry.ID - 1
			if idx >= 0 && idx < expectedCount {
				scores[idx] = math.Min(10, math.Max(0, entry.Score))
			}
		}
		return scores, true
	}

	return parseRerankScoresRegex(resp, expectedCount)
}

var rerankScoreRegex = regexp.MustCompile(`\[(\d+)\]\s*(\d+(?:\.\d+)?)`)

func parseRerankScoresRegex(resp string, expectedCount int) ([]float64, bool) {
	matches := rerankScoreRegex.FindAllStringSubmatch(resp, -1)
	if len(matches) == 0 {
		return nil, false
	}
	scores := make([]float64, expectedCount)
	for _, m := range matches {
		id, _ := strconv.Atoi(m[1])
		score, _ := strconv.ParseFloat(m[2], 64)
		idx := id - 1
		if idx >= 0 && idx < expectedCount {
			scores[idx] = math.Min(10, math.Max(0, score))
		}
	}
	return scores, true
}

var (
	ipPattern      = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	tokenPattern   = regexp.MustCompile(`(?i)(token|key|secret|password|credential)[=:]\s*\S+`)
	uuidPattern    = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	macPattern     = regexp.MustCompile(`(?i)\b[0-9a-f]{2}(:[0-9a-f]{2}){5}\b`)
	emailPattern   = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)
	phonePattern   = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	hostKeyPattern = regexp.MustCompile(`(?i)\b(?:host|hostname|server)[=:]\s*\S+`)
)

func sanitizeSnippets(items []ContextItem) []ContextItem {
	result := make([]ContextItem, len(items))
	copy(result, items)
	for i := range result {
		content := result[i].Content
		content = ipPattern.ReplaceAllString(content, "[private-ip]")
		content = tokenPattern.ReplaceAllString(content, "[redacted]")
		content = uuidPattern.ReplaceAllString(content, "[uuid]")
		content = macPattern.ReplaceAllString(content, "[mac]")
		content = emailPattern.ReplaceAllString(content, "[email]")
		content = phonePattern.ReplaceAllString(content, "[phone]")
		content = hostKeyPattern.ReplaceAllString(content, "[host-redacted]")
		content = truncateSnippetText(content, snippetTruncateLen)
		result[i].Content = content
	}
	return result
}

func truncateSnippetText(content string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(content) <= limit {
		return content
	}
	runes := []rune(content)
	return string(runes[:limit]) + "..."
}
