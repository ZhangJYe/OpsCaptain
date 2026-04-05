package mem

import (
	"context"
	"strings"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultMaxContextTokens = 8000
	avgCharsPerToken        = 2.5
)

type TokenBudget struct {
	MaxTokens       int
	SystemReserve   int
	MemoryReserve   int
	HistoryReserve  int
	DocumentReserve int
}

var (
	globalBudget     *TokenBudget
	globalBudgetOnce sync.Once
)

func GetTokenBudget() *TokenBudget {
	globalBudgetOnce.Do(func() {
		maxTokens := defaultMaxContextTokens

		v, err := g.Cfg().Get(context.Background(), "memory.max_context_tokens")
		if err == nil && v.Int() > 0 {
			maxTokens = v.Int()
		}

		globalBudget = &TokenBudget{
			MaxTokens:       maxTokens,
			SystemReserve:   int(float64(maxTokens) * 0.20),
			MemoryReserve:   int(float64(maxTokens) * 0.10),
			HistoryReserve:  int(float64(maxTokens) * 0.40),
			DocumentReserve: int(float64(maxTokens) * 0.15),
		}
	})
	return globalBudget
}

func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	cjkCount := 0
	asciiCount := 0
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjkCount++
		} else if r > 32 && r < 127 {
			asciiCount++
		}
	}

	cjkTokens := float64(cjkCount) * 1.5
	asciiTokens := float64(asciiCount) / 4.0
	overhead := float64(len(text)-cjkCount-asciiCount) * 0.5

	total := int(cjkTokens + asciiTokens + overhead)
	if total < 1 && len(text) > 0 {
		total = 1
	}
	return total
}

func TrimToTokenBudget(text string, maxTokens int) string {
	if maxTokens <= 0 {
		return text
	}

	current := EstimateTokens(text)
	if current <= maxTokens {
		return text
	}

	ratio := float64(maxTokens) / float64(current)
	targetLen := int(float64(len(text)) * ratio * 0.9)
	if targetLen <= 0 {
		return ""
	}
	if targetLen >= len(text) {
		return text
	}

	runes := []rune(text)
	if targetLen > len(runes) {
		return text
	}

	trimmed := string(runes[:targetLen])
	if idx := strings.LastIndex(trimmed, "\n"); idx > len(trimmed)/2 {
		trimmed = trimmed[:idx]
	}

	return trimmed + "..."
}
