package contextengine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/utility/mem"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultContextLLMCompressTimeout   = 1500 * time.Millisecond
	defaultContextLLMCompressMinTokens = 96
	contextCompressionPrompt           = `You compress assistant context for AIOps and support analysis.
Keep only facts relevant to the current query.
Preserve alert names, service names, pod names, operation names, timestamps, error codes, metric names, numeric values, dependency names, and direct next actions.
Do not invent facts. Do not add explanations outside the source content.
Return plain text bullet lines only.`
)

type compressionRequest struct {
	Query        string
	SourceType   string
	Title        string
	Content      string
	TargetTokens int
}

var (
	llmCompressionEnabled = func(ctx context.Context) bool {
		v, err := g.Cfg().Get(ctx, "context.llm_compress_enabled")
		return err == nil && v.Bool()
	}
	llmCompressionMinTokens = func(ctx context.Context) int {
		v, err := g.Cfg().Get(ctx, "context.llm_compress_min_tokens")
		if err == nil && v.Int() > 0 {
			return v.Int()
		}
		return defaultContextLLMCompressMinTokens
	}
	llmCompressionTimeout = func(ctx context.Context) time.Duration {
		v, err := g.Cfg().Get(ctx, "context.llm_compress_timeout_ms")
		if err == nil && v.Int64() > 0 {
			return time.Duration(v.Int64()) * time.Millisecond
		}
		return defaultContextLLMCompressTimeout
	}
	compressContextText = defaultCompressContextText
)

func fitContextItemToBudget(ctx context.Context, query string, item ContextItem, remaining int, dropReason string) (ContextItem, bool) {
	if remaining <= 0 {
		item.DroppedReason = dropReason
		return item, false
	}
	if item.TokenEstimate <= remaining {
		return item, true
	}

	if llmCompressionEnabled(ctx) && remaining >= llmCompressionMinTokens(ctx) {
		compressed, err := compressContextText(ctx, compressionRequest{
			Query:        query,
			SourceType:   item.SourceType,
			Title:        item.Title,
			Content:      item.Content,
			TargetTokens: remaining,
		})
		if err != nil {
			g.Log().Debugf(ctx, "context llm compression skipped for %s/%s: %v", item.SourceType, item.Title, err)
		} else if strings.TrimSpace(compressed) != "" {
			item.Content = strings.TrimSpace(compressed)
			item.TokenEstimate = mem.EstimateTokens(item.Content)
			item.CompressionLevel = "llm_compressed"
			if item.TokenEstimate <= remaining {
				return item, true
			}
		}
	}

	trimmed := mem.TrimToTokenBudget(item.Content, remaining)
	if strings.TrimSpace(trimmed) == "" {
		item.DroppedReason = dropReason
		return item, false
	}
	item.Content = trimmed
	item.TokenEstimate = mem.EstimateTokens(trimmed)
	if item.CompressionLevel == "llm_compressed" {
		item.CompressionLevel = "llm_compressed_trimmed"
	} else {
		item.CompressionLevel = "trimmed"
	}
	return item, true
}

func defaultCompressContextText(ctx context.Context, req compressionRequest) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, llmCompressionTimeout(ctx))
	defer cancel()

	chatModel, err := models.OpenAIForDeepSeekV3Quick(callCtx)
	if err != nil {
		return "", err
	}

	userMsg := fmt.Sprintf(
		"Query:\n%s\n\nSource type: %s\nTitle: %s\nApprox token budget: %d\n\nContent:\n%s",
		strings.TrimSpace(req.Query),
		strings.TrimSpace(req.SourceType),
		strings.TrimSpace(req.Title),
		req.TargetTokens,
		strings.TrimSpace(req.Content),
	)

	resp, err := chatModel.Generate(callCtx, []*schema.Message{
		{Role: schema.System, Content: contextCompressionPrompt},
		{Role: schema.User, Content: userMsg},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func formatCompressionNote(items []ContextItem) string {
	counts := make(map[string]int)
	for _, item := range items {
		level := strings.TrimSpace(item.CompressionLevel)
		if level == "" {
			continue
		}
		counts[level]++
	}
	if len(counts) == 0 {
		return ""
	}

	parts := make([]string, 0, len(counts))
	for level, count := range counts {
		parts = append(parts, fmt.Sprintf("%s=%d", level, count))
	}
	sort.Strings(parts)
	return "compression=" + strings.Join(parts, ",")
}
