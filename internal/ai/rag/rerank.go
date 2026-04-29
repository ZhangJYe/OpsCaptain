package rag

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/models"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultRerankTimeout = 5 * time.Second
	rerankSystemPrompt   = `You are a document relevance judge for IT operations.
Given a query and a list of documents, rate each document's relevance to the query on a scale of 0-10.
Output ONLY a comma-separated list of scores in the same order as the documents.
Example output: 9,3,7,1,8
Do not output anything else.`
)

type RerankResult struct {
	Docs    []*schema.Document
	Scores  []float64
	Enabled bool
}

func Rerank(ctx context.Context, query string, docs []*schema.Document, topK int) RerankResult {
	if len(docs) <= 1 {
		return RerankResult{Docs: docs, Enabled: false}
	}

	rerankCtx, cancel := context.WithTimeout(ctx, rerankTimeout(ctx))
	defer cancel()

	chatModel, err := models.OpenAIForGLMFast(rerankCtx)
	if err != nil {
		g.Log().Debugf(ctx, "rerank skipped: model init failed: %v", err)
		return RerankResult{Docs: docs, Enabled: false}
	}

	var sb strings.Builder
	for i, doc := range docs {
		title := docTitle(doc)
		content := doc.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Fprintf(&sb, "[%d] %s\n%s\n\n", i+1, title, content)
	}

	userMsg := fmt.Sprintf("Query: %s\n\nDocuments:\n%s", query, sb.String())

	resp, err := chatModel.Generate(rerankCtx, []*schema.Message{
		{Role: schema.System, Content: rerankSystemPrompt},
		{Role: schema.User, Content: userMsg},
	})
	if err != nil {
		g.Log().Debugf(ctx, "rerank failed: %v", err)
		return RerankResult{Docs: docs, Enabled: false}
	}

	scores := parseScores(resp.Content, len(docs))
	if scores == nil {
		g.Log().Debugf(ctx, "rerank score parsing failed: %q", resp.Content)
		return RerankResult{Docs: docs, Enabled: false}
	}

	type indexedDoc struct {
		idx   int
		doc   *schema.Document
		score float64
	}
	items := make([]indexedDoc, len(docs))
	for i := range docs {
		items[i] = indexedDoc{idx: i, doc: docs[i], score: scores[i]}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	limit := topK
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	reranked := make([]*schema.Document, 0, limit)
	rerankedScores := make([]float64, 0, limit)
	for _, item := range items[:limit] {
		reranked = append(reranked, item.doc)
		rerankedScores = append(rerankedScores, item.score)
	}

	g.Log().Debugf(ctx, "rerank completed: %d -> %d docs", len(docs), len(reranked))
	return RerankResult{Docs: reranked, Scores: rerankedScores, Enabled: true}
}

func parseScores(raw string, expected int) []float64 {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.Trim(cleaned, "[]")
	parts := strings.Split(cleaned, ",")
	if len(parts) != expected {
		return nil
	}
	scores := make([]float64, len(parts))
	for i, p := range parts {
		s, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return nil
		}
		scores[i] = s
	}
	return scores
}

func rerankTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "rag.rerank_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultRerankTimeout
}

func docTitle(doc *schema.Document) string {
	if doc == nil || doc.MetaData == nil {
		return ""
	}
	for _, key := range []string{"title", "file_name", "filename", "source"} {
		if v, ok := doc.MetaData[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}
