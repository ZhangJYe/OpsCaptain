package eval

import (
	"context"
	"time"

	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
)

type TriageRunner struct {
	agent *triage.Agent
}

func NewTriageRunner() *TriageRunner {
	return &TriageRunner{agent: triage.New()}
}

func (r *TriageRunner) Run(ctx context.Context, query string) (*RunResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	agent := r.agent
	if agent == nil {
		agent = triage.New()
	}
	task := protocol.NewRootTask("eval-triage", query, triage.AgentName)
	started := time.Now()
	result, err := agent.Handle(ctx, task)
	latency := time.Since(started)
	if err != nil {
		return nil, err
	}
	intent, _ := result.Metadata["intent"].(string)
	return &RunResult{
		Summary:       result.Summary,
		Intent:        intent,
		Domains:       extractDomains(result.Metadata),
		Status:        string(result.Status),
		Latency:       latency,
		LatencyMillis: latency.Milliseconds(),
		Metadata:      result.Metadata,
	}, nil
}
