package gos_engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"

	"github.com/cloudwego/eino/schema"
)

func newStructuredGenerate(cfg *Config) StructuredGenerateFunc {
	return func(ctx context.Context, prompt string) (string, error) {
		timeout := time.Duration(cfg.StructuredCognition.CallTimeoutMs) * time.Millisecond
		if timeout <= 0 {
			return "", fmt.Errorf("structured cognition call_timeout_ms must be positive")
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		chatModel, err := models.OpenAIChatModelFactory(cfg.ModelPath)(callCtx)
		if err != nil {
			return "", fmt.Errorf("create structured cognition model: %w", err)
		}
		response, err := chatModel.Generate(callCtx, []*schema.Message{schema.UserMessage(prompt)})
		if err != nil {
			return "", fmt.Errorf("generate structured cognition proposal: %w", err)
		}
		if response == nil || strings.TrimSpace(response.Content) == "" {
			return "", fmt.Errorf("structured cognition returned empty content")
		}
		return strings.TrimSpace(response.Content), nil
	}
}
