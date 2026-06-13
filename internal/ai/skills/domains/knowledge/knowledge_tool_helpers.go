package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/tools"

	"github.com/gogf/gf/v2/frame/g"
)

func runKnowledgeLookupWithFocus(ctx context.Context, task *protocol.TaskEnvelope, mode string, focus string) (*protocol.TaskResult, error) {
	t := newQueryInternalDocsTool()
	if t == nil {
		return &protocol.TaskResult{
			TaskID:            task.TaskID,
			Agent:             AgentName,
			Status:            protocol.ResultStatusDegraded,
			Summary:           "知识库检索工具不可用",
			DegradationReason: "query_internal_docs tool init failed",
		}, nil
	}
	retrievalQuery := buildKnowledgeQuery(task.Goal, focus)
	args, _ := json.Marshal(&tools.QueryInternalDocsInput{Query: retrievalQuery})
	queryCtx, cancel := context.WithTimeout(ctx, knowledgeQueryTimeout(ctx))
	defer cancel()

	output, err := t.InvokableRun(queryCtx, string(args))
	if err != nil {
		summary := fmt.Sprintf("knowledge lookup failed: %v", err)
		if queryCtx.Err() == context.DeadlineExceeded {
			summary = "knowledge lookup timed out; skipped"
		}
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    summary,
			Confidence: confidenceLookupFailed,
			Metadata: map[string]any{
				"error":           err.Error(),
				"knowledge_mode":  mode,
				"knowledge_query": retrievalQuery,
			},
		}, nil
	}

	return buildKnowledgeLookupResult(task, mode, retrievalQuery, output, buildKnowledgeNextActions(mode)), nil
}

func buildKnowledgeQuery(goal string, focus string) string {
	goal = strings.TrimSpace(goal)
	focus = strings.TrimSpace(focus)
	if focus == "" {
		return goal
	}
	if goal == "" {
		return focus
	}
	return goal + "\nFocus: " + focus
}

func buildKnowledgeMetadata(task *protocol.TaskEnvelope, mode string, retrievalQuery string, documentCount int) map[string]any {
	metadata := map[string]any{
		"document_count":  documentCount,
		"knowledge_mode":  mode,
		"knowledge_query": retrievalQuery,
	}
	if mode == "service_error_code_lookup" {
		if codes := extractErrorCodes(task.Goal); len(codes) > 0 {
			metadata["extracted_error_codes"] = codes
		}
	}
	return metadata
}

func buildKnowledgeNextActions(mode string) []string {
	switch mode {
	case "service_error_code_lookup":
		return []string{
			"confirm which service emitted the error code and whether it came from an upstream dependency",
			"compare the code meaning with the latest release, database change, and downstream status",
		}
	default:
		return nil
	}
}

func buildKnowledgeLookupResult(task *protocol.TaskEnvelope, mode string, retrievalQuery string, output string, nextActions []string) *protocol.TaskResult {
	parsed, err := parseKnowledgeLookupOutput(output)
	if err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      AgentName,
			Status:     protocol.ResultStatusDegraded,
			Summary:    "knowledge lookup returned an unreadable document payload",
			Confidence: confidenceDocumentUnreadable,
			Metadata: map[string]any{
				"knowledge_mode":  mode,
				"knowledge_query": retrievalQuery,
				"decode_failed":   true,
				"decode_error":    err.Error(),
				"document_count":  0,
			},
		}
	}

	if parsed.degraded {
		summary := parsed.message
		if summary == "" {
			summary = "knowledge lookup degraded"
		}
		return &protocol.TaskResult{
			TaskID:            task.TaskID,
			Agent:             AgentName,
			Status:            protocol.ResultStatusDegraded,
			Summary:           summary,
			Confidence:        confidenceLookupFailed,
			DegradationReason: summary,
			Metadata: map[string]any{
				"knowledge_mode":  mode,
				"knowledge_query": retrievalQuery,
				"document_count":  parsed.documentCount,
				"tool_degraded":   true,
			},
		}
	}

	summary := "knowledge lookup returned no reusable documents"
	if len(parsed.evidence) > 0 {
		summary = fmt.Sprintf("knowledge lookup found %d relevant documents", parsed.documentCount)
		if len(parsed.highlights) > 0 {
			summary += ": " + strings.Join(parsed.highlights, " | ")
		}
	}

	return &protocol.TaskResult{
		TaskID:      task.TaskID,
		Agent:       AgentName,
		Status:      protocol.ResultStatusSucceeded,
		Summary:     summary,
		Confidence:  confidenceKnowledgeSucceeded,
		Evidence:    parsed.evidence,
		NextActions: nextActions,
		Metadata:    buildKnowledgeMetadata(task, mode, retrievalQuery, parsed.documentCount),
	}
}

func knowledgeEvidenceLimit() int {
	v, err := g.Cfg().Get(context.Background(), "multi_agent.knowledge_evidence_limit")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	v, err = g.Cfg().Get(context.Background(), "retriever.top_k")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return 3
}