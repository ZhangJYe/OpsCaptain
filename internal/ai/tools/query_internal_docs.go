package tools

import (
	"SuperBizAgent/internal/ai/rag"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

type QueryInternalDocsInput struct {
	Query string `json:"query" jsonschema:"description=The query string to search in internal documentation for relevant information and processing steps"`
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
				return "", fmt.Errorf("failed to query internal docs: %w", err)
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
