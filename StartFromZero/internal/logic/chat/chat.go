package chat

import (
	"context"
	"errors"
	"strings"
)

type Service interface {
	Ask(ctx context.Context, question string) (string, error)
}

type ModelClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type service struct {
	model ModelClient
}

func New(model ModelClient) Service {
	return &service{model: model}
}

func (s *service) Ask(ctx context.Context, question string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	q := strings.TrimSpace(question)
	if q == "" {
		return "", errors.New("question is empty")
	}
	if s.model == nil {
		return "收到你的问题：" + q + "（当前为本地 mock 模式）", nil
	}
	return s.model.Generate(ctx, q)
}
