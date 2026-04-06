package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/tools"

	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "knowledge"

const defaultKnowledgeQueryTimeout = 5 * time.Second

var (
	newQueryInternalDocsTool = tools.NewQueryInternalDocsTool
	knowledgeQueryTimeout    = func(ctx context.Context) time.Duration {
		v, err := g.Cfg().Get(ctx, "multi_agent.knowledge_query_timeout_ms")
		if err == nil && v.Int64() > 0 {
			return time.Duration(v.Int64()) * time.Millisecond
		}
		return defaultKnowledgeQueryTimeout
	}
)

type Agent struct{}

func New() *Agent {
	return &Agent{}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return []string{"knowledge-rag"}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	tool := newQueryInternalDocsTool()
	args, _ := json.Marshal(&tools.QueryInternalDocsInput{Query: task.Goal})
	queryCtx, cancel := context.WithTimeout(ctx, knowledgeQueryTimeout(ctx))
	defer cancel()

	output, err := tool.InvokableRun(queryCtx, string(args))
	if err != nil {
		summary := fmt.Sprintf("知识库检索失败: %v", err)
		if queryCtx.Err() == context.DeadlineExceeded {
			summary = "知识库检索超时，已跳过该步骤。"
		}
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusDegraded,
			Summary:    summary,
			Confidence: 0.25,
			Metadata: map[string]any{
				"error": err.Error(),
			},
		}, nil
	}

	var docs []map[string]any
	if err := json.Unmarshal([]byte(output), &docs); err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusDegraded,
			Summary:    "知识库检索已执行，但文档结果解析失败。",
			Confidence: 0.3,
			Metadata: map[string]any{
				"raw_output": output,
			},
		}, nil
	}

	limit := knowledgeEvidenceLimit(ctx)
	evidence := make([]protocol.EvidenceItem, 0, min(limit, len(docs)))
	highlights := make([]string, 0, min(limit, len(docs)))
	for idx, doc := range docs {
		if idx >= limit {
			break
		}
		content := firstNonEmptyString(doc, "content", "page_content", "text")
		title := firstNonEmptyString(doc, "id", "title", "source")
		if title == "" {
			title = fmt.Sprintf("doc-%d", idx+1)
		}
		snippet := shorten(content, 160)
		highlights = append(highlights, snippet)
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "knowledge",
			SourceID:   title,
			Title:      title,
			Snippet:    snippet,
			Score:      extractDocScore(doc, idx),
		})
	}

	summary := "知识库未检索到可直接复用的文档。"
	if len(evidence) > 0 {
		summary = fmt.Sprintf("知识库命中 %d 条相关文档，优先参考：%s。", len(docs), strings.Join(highlights, " | "))
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    summary,
		Confidence: 0.78,
		Evidence:   evidence,
		Metadata: map[string]any{
			"document_count": len(docs),
		},
	}, nil
}

func extractDocScore(doc map[string]any, idx int) float64 {
	for _, key := range []string{"_score", "score"} {
		if raw, ok := doc[key]; ok {
			switch v := raw.(type) {
			case float64:
				return v
			case float32:
				return float64(v)
			}
		}
	}
	return 0.74 - float64(idx)*0.08
}

func firstNonEmptyString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func shorten(input string, max int) string {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if len(input) <= max {
		return input
	}
	return input[:max] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func knowledgeEvidenceLimit(ctx context.Context) int {
	v, err := g.Cfg().Get(ctx, "multi_agent.knowledge_evidence_limit")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return rag.RetrieverTopK(ctx)
}
