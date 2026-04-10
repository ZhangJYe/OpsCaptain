package knowledge_index_pipeline

import (
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"

	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "knowledge_indexer"

type IndexResult struct {
	SourcePath      string
	ResolvedSource  string
	DeletedExisting int
	ChunkCount      int
	ChunkIDs        []string
}

type Indexer interface {
	IndexForAgent(ctx context.Context, path string) (IndexResult, error)
}

type Agent struct {
	indexer Indexer
}

func NewAgent(indexer Indexer) *Agent {
	return &Agent{indexer: indexer}
}

func (a *Agent) Name() string { return AgentName }

func (a *Agent) Capabilities() []string {
	return []string{"knowledge-indexing", "document-indexing", "vector-indexing"}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	path, _ := task.Input["path"].(string)
	if path == "" {
		path = task.Goal
	}
	if strings.TrimSpace(path) == "" {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusFailed,
			Summary:    "no document path provided",
			Confidence: 0,
			Error:      &protocol.TaskError{Code: "MISSING_PATH", Message: "input 'path' is required"},
		}, nil
	}

	g.Log().Infof(ctx, "[%s] indexing document: %s", AgentName, path)
	result, err := a.indexer.IndexForAgent(ctx, path)
	if err != nil {
		return &protocol.TaskResult{
			TaskID:     task.TaskID,
			Agent:      a.Name(),
			Status:     protocol.ResultStatusFailed,
			Summary:    fmt.Sprintf("indexing failed for %s: %v", path, err),
			Confidence: 0,
			Error:      &protocol.TaskError{Code: "INDEX_FAILED", Message: err.Error()},
		}, nil
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    fmt.Sprintf("indexed %s: %d chunks (deleted %d existing)", result.ResolvedSource, result.ChunkCount, result.DeletedExisting),
		Confidence: 1.0,
		Metadata: map[string]any{
			"source_path":      result.SourcePath,
			"resolved_source":  result.ResolvedSource,
			"chunk_count":      result.ChunkCount,
			"chunk_ids":        result.ChunkIDs,
			"deleted_existing": result.DeletedExisting,
		},
	}, nil
}
