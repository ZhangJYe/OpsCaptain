package tools

import (
	"SuperBizAgent/internal/ai/rag"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

type QueryInternalDocsInput struct {
	Query string `json:"query" jsonschema:"description=The query string to search in internal documentation for relevant information and processing steps"`
}

type QueryInternalDocsOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

const defaultInternalDocsQueryTimeout = 5 * time.Second

func NewQueryInternalDocsTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_internal_docs",
		"Use this tool to search internal documentation and knowledge base for relevant information. It performs RAG (Retrieval-Augmented Generation) to find similar documents and extract processing steps. This is useful when you need to understand internal procedures, best practices, or step-by-step guides stored in the company's documentation.",
		func(ctx context.Context, input *QueryInternalDocsInput, opts ...tool.Option) (output string, err error) {
			queryCtx, cancel := context.WithTimeout(ctx, internalDocsQueryTimeout(ctx))
			defer cancel()

			resp, _, err := rag.Query(queryCtx, rag.SharedPool(), input.Query)
			if err != nil {
				g.Log().Warningf(ctx, "query_internal_docs degraded: %v", err)
				out := QueryInternalDocsOutput{
					Success: false,
					Message: "内部知识库检索暂时不可用。请继续基于可用的 metrics、logs 和用户提供的上下文诊断，并明确标注缺失知识库证据。",
					Error:   fmt.Sprintf("failed to query internal docs: %v", err),
				}
				respBytes, _ := json.Marshal(out)
				return string(respBytes), nil
			}
			respBytes, err := json.Marshal(resp)
			if err != nil {
				return "", fmt.Errorf("failed to marshal response: %w", err)
			}
			return string(respBytes), nil
		})
	if err != nil {
		panic(fmt.Sprintf("failed to create query_internal_docs tool: %v", err))
	}
	return t
}

func internalDocsQueryTimeout(ctx context.Context) time.Duration {
	return rag.DurationFromConfig(ctx, defaultInternalDocsQueryTimeout, "multi_agent.knowledge_query_timeout_ms")
}
