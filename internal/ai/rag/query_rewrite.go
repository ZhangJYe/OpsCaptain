package rag

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/promptreg"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const defaultRewriteTimeout = 3 * time.Second

type RewriteResult struct {
	Query     string
	Attempted bool
	Applied   bool
	Degraded  bool
	Reason    string
}

func RewriteQuery(ctx context.Context, query string) string {
	return RewriteQueryWithResult(ctx, query).Query
}

func RewriteQueryWithResult(ctx context.Context, query string) RewriteResult {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return RewriteResult{Query: query, Reason: "empty_query"}
	}
	result := RewriteResult{Query: query, Attempted: true}

	rewriteCtx, cancel := context.WithTimeout(ctx, rewriteTimeout(ctx))
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(rewriteCtx)
	if err != nil {
		g.Log().Debugf(ctx, "query rewrite skipped: model init failed: %v", err)
		result.Degraded = true
		result.Reason = "model_init_failed"
		return result
	}

	resp, err := chatModel.Generate(rewriteCtx, []*schema.Message{
		{Role: schema.System, Content: promptreg.RAGRewrite},
		{Role: schema.User, Content: trimmed},
	})
	if err != nil {
		g.Log().Debugf(ctx, "query rewrite failed: %v", err)
		result.Degraded = true
		result.Reason = modelFailureReason(err)
		return result
	}

	rewritten := strings.TrimSpace(resp.Content)
	if rewritten == "" {
		result.Degraded = true
		result.Reason = "empty_response"
		return result
	}

	g.Log().Debugf(ctx, "query rewrite: %q -> %q", query, rewritten)
	result.Query = rewritten
	result.Applied = true
	return result
}

func modelFailureReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "model_call_failed"
	}
}

func rewriteTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "rag.rewrite_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultRewriteTimeout
}

func RewriteQueryMulti(ctx context.Context, query string, n int) []string {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" || n <= 1 {
		return []string{query}
	}

	rewriteCtx, cancel := context.WithTimeout(ctx, rewriteTimeout(ctx))
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(rewriteCtx)
	if err != nil {
		g.Log().Warningf(ctx, "[rag] query rewrite multi skipped: model init failed: %v", err)
		return []string{query}
	}

	multiPrompt := fmt.Sprintf(`You are a search query optimizer. Generate %d diverse search queries for the following question.
Each query should capture a different angle or use different keywords.
Output one query per line, no numbering, no explanation.`, n)

	resp, err := chatModel.Generate(rewriteCtx, []*schema.Message{
		{Role: schema.System, Content: multiPrompt},
		{Role: schema.User, Content: trimmed},
	})
	if err != nil {
		g.Log().Warningf(ctx, "[rag] query rewrite multi failed, using original: %v", err)
		return []string{query}
	}

	lines := strings.Split(strings.TrimSpace(resp.Content), "\n")
	queries := make([]string, 0, len(lines)+1)
	queries = append(queries, query)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && line != query {
			queries = append(queries, line)
		}
	}
	return queries
}
