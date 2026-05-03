package eval

import (
	"context"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/ai/agent/supervisor"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

// MultiAgentRunner executes queries through the supervisor/triage/reporter pipeline.
//
// Deprecated: This runner depends on the chat_multi_agent pipeline which has been
// superseded by the Chat Pipeline (ReAct Agent) and Plan-Execute-Replan paths.
// Use the active agent pipelines for new evaluation work.
type MultiAgentRunner struct {
	rt *runtime.Runtime
}

// NewMultiAgentRunner creates a new MultiAgentRunner.
//
// Deprecated: See MultiAgentRunner deprecation notice.
func NewMultiAgentRunner() (*MultiAgentRunner, error) {
	rt := runtime.New()
	for _, agent := range []runtime.Agent{
		supervisor.New(),
		triage.New(),
		metrics.New(),
		logs.New(),
		knowledge.New(),
		reporter.New(),
	} {
		if err := rt.Register(agent); err != nil {
			return nil, err
		}
	}
	return &MultiAgentRunner{rt: rt}, nil
}

// Run executes a query through the multi-agent pipeline.
func (r *MultiAgentRunner) Run(ctx context.Context, query string) (*RunResult, error) {
	task := protocol.NewRootTask("eval-session", query, supervisor.AgentName)
	task.Input = map[string]any{
		"raw_query":     query,
		"response_mode": "report",
		"entrypoint":    "eval",
	}

	result, err := r.rt.Dispatch(ctx, task)
	if err != nil {
		return nil, err
	}

	intent, _ := result.Metadata["intent"].(string)
	domains := extractDomains(result.Metadata)

	return &RunResult{
		Summary:  result.Summary,
		Intent:   intent,
		Domains:  domains,
		Metadata: result.Metadata,
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
