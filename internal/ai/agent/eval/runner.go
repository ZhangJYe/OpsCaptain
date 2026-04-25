package eval

import (
	"context"
	"fmt"
	"time"

	"SuperBizAgent/internal/ai/protocol"
)

const defaultRunnerAssignee = "supervisor"

type Dispatcher interface {
	Dispatch(context.Context, *protocol.TaskEnvelope) (*protocol.TaskResult, error)
}

type MultiAgentRunner struct {
	dispatcher   Dispatcher
	assignee     string
	sessionID    string
	responseMode string
}

func NewRuntimeRunner(dispatcher Dispatcher) *MultiAgentRunner {
	return &MultiAgentRunner{
		dispatcher:   dispatcher,
		assignee:     defaultRunnerAssignee,
		sessionID:    "eval-session",
		responseMode: "report",
	}
}

func NewDispatcherRunner(dispatcher Dispatcher, assignee string) *MultiAgentRunner {
	runner := NewRuntimeRunner(dispatcher)
	if assignee != "" {
		runner.assignee = assignee
	}
	return runner
}

func (r *MultiAgentRunner) WithSessionID(sessionID string) *MultiAgentRunner {
	if sessionID != "" {
		r.sessionID = sessionID
	}
	return r
}

func (r *MultiAgentRunner) WithResponseMode(responseMode string) *MultiAgentRunner {
	if responseMode != "" {
		r.responseMode = responseMode
	}
	return r
}

func (r *MultiAgentRunner) Run(ctx context.Context, query string) (*RunResult, error) {
	if r == nil || r.dispatcher == nil {
		return nil, fmt.Errorf("runtime runner is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID := r.sessionID
	if sessionID == "" {
		sessionID = "eval-session"
	}
	responseMode := r.responseMode
	if responseMode == "" {
		responseMode = "report"
	}
	assignee := r.assignee
	if assignee == "" {
		assignee = defaultRunnerAssignee
	}

	task := protocol.NewRootTask(sessionID, query, assignee)
	task.Input = map[string]any{
		"raw_query":     query,
		"response_mode": responseMode,
		"entrypoint":    "eval",
	}

	started := time.Now()
	result, err := r.dispatcher.Dispatch(ctx, task)
	latency := time.Since(started)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("runtime returned nil result")
	}

	intent, _ := result.Metadata["intent"].(string)
	domains := extractDomains(result.Metadata)

	return &RunResult{
		Summary:       result.Summary,
		Intent:        intent,
		Domains:       domains,
		Status:        string(result.Status),
		Latency:       latency,
		LatencyMillis: latency.Milliseconds(),
		TokensUsed:    metadataInt64(result.Metadata, "tokens_used"),
		LLMCalls:      int(metadataInt64(result.Metadata, "llm_calls")),
		Metadata:      result.Metadata,
	}, nil
}

func extractDomains(metadata map[string]any) []string {
	if raw, ok := metadata["domains"].([]string); ok {
		return raw
	}
	if converted, ok := metadata["domains"].([]any); ok {
		domains := make([]string, 0, len(converted))
		for _, item := range converted {
			if s, ok := item.(string); ok {
				domains = append(domains, s)
			}
		}
		return domains
	}
	return nil
}

func metadataInt64(metadata map[string]any, key string) int64 {
	if metadata == nil {
		return 0
	}
	switch value := metadata[key].(type) {
	case int:
		return int64(value)
	case int32:
		return int64(value)
	case int64:
		return value
	case float32:
		return int64(value)
	case float64:
		return int64(value)
	default:
		return 0
	}
}
