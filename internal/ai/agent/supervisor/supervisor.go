package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/specialists/knowledge"
	"SuperBizAgent/internal/ai/agent/specialists/logs"
	"SuperBizAgent/internal/ai/agent/specialists/metrics"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

const AgentName = "supervisor"

type Agent struct{}

func New() *Agent {
	return &Agent{}
}

func (a *Agent) Name() string {
	return AgentName
}

func (a *Agent) Capabilities() []string {
	return []string{"planning", "routing", "aggregation"}
}

func (a *Agent) Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	rt, ok := runtime.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("runtime is missing from context")
	}

	rt.EmitInfo(ctx, task, a.Name(), "supervisor started orchestration", nil)
	rawQuery := task.Goal
	memoryContext := taskInputString(task, "memory_context")
	responseMode := taskInputString(task, "response_mode")
	specialistQuery := withMemoryContext(rawQuery, memoryContext)

	triageTask := protocol.NewChildTask(task, triage.AgentName, rawQuery, map[string]any{
		"query": rawQuery,
	})
	triageTask.MemoryRefs = append([]protocol.MemoryRef(nil), task.MemoryRefs...)
	triageResult, err := rt.Dispatch(ctx, triageTask)
	if err != nil {
		return triageResult, err
	}

	intent, _ := triageResult.Metadata["intent"].(string)
	rawDomains, _ := triageResult.Metadata["domains"].([]string)
	if len(rawDomains) == 0 {
		if converted, ok := triageResult.Metadata["domains"].([]any); ok {
			for _, item := range converted {
				if s, ok := item.(string); ok {
					rawDomains = append(rawDomains, s)
				}
			}
		}
	}
	if len(rawDomains) == 0 {
		rawDomains = []string{"metrics", "logs", "knowledge"}
	}

	childResults := make([]*protocol.TaskResult, 0, len(rawDomains))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, domain := range rawDomains {
		agentName := specialistAgentName(domain)
		if agentName == "" {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			childTask := protocol.NewChildTask(task, name, specialistQuery, map[string]any{
				"query":          rawQuery,
				"raw_query":      rawQuery,
				"memory_context": memoryContext,
				"response_mode":  responseMode,
				"intent":         intent,
			})
			childTask.MemoryRefs = append([]protocol.MemoryRef(nil), task.MemoryRefs...)
			result, dispatchErr := rt.Dispatch(ctx, childTask)
			mu.Lock()
			defer mu.Unlock()
			if result != nil {
				if ref, artifactErr := createResultArtifact(ctx, rt, result); artifactErr == nil && ref != nil {
					result.ArtifactRefs = append(result.ArtifactRefs, *ref)
				}
				childResults = append(childResults, result)
			}
			if dispatchErr != nil && result == nil {
				childResults = append(childResults, &protocol.TaskResult{
					TaskID:     childTask.TaskID,
					Agent:      name,
					Status:     protocol.ResultStatusFailed,
					Summary:    dispatchErr.Error(),
					Confidence: 0,
					Error: &protocol.TaskError{
						Message: dispatchErr.Error(),
					},
				})
			}
		}(agentName)
	}
	wg.Wait()

	reportTask := protocol.NewChildTask(task, reporter.AgentName, rawQuery, map[string]any{
		"query":         rawQuery,
		"intent":        intent,
		"response_mode": responseMode,
		"results":       childResults,
	})
	reportTask.MemoryRefs = append([]protocol.MemoryRef(nil), task.MemoryRefs...)
	reportResult, err := rt.Dispatch(ctx, reportTask)
	if err != nil {
		return reportResult, err
	}

	evidence := make([]protocol.EvidenceItem, 0)
	for _, child := range childResults {
		if child != nil {
			evidence = append(evidence, child.Evidence...)
		}
	}

	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    reportResult.Summary,
		Confidence: reportResult.Confidence,
		Evidence:   evidence,
		Metadata: map[string]any{
			"intent": intent,
		},
	}, nil
}

func createResultArtifact(ctx context.Context, rt *runtime.Runtime, result *protocol.TaskResult) (*protocol.ArtifactRef, error) {
	payload, err := json.Marshal(map[string]any{
		"agent":      result.Agent,
		"status":     result.Status,
		"summary":    result.Summary,
		"confidence": result.Confidence,
		"evidence":   result.Evidence,
	})
	if err != nil {
		return nil, err
	}
	return rt.CreateArtifact(ctx, "task_result", string(payload), map[string]any{
		"agent": result.Agent,
		"task":  result.TaskID,
	})
}

func specialistAgentName(domain string) string {
	switch domain {
	case "metrics":
		return metrics.AgentName
	case "logs":
		return logs.AgentName
	case "knowledge":
		return knowledge.AgentName
	default:
		return ""
	}
}

func taskInputString(task *protocol.TaskEnvelope, key string) string {
	if task == nil || task.Input == nil {
		return ""
	}
	value, _ := task.Input[key].(string)
	return value
}

func withMemoryContext(query, memoryContext string) string {
	query = strings.TrimSpace(query)
	memoryContext = strings.TrimSpace(memoryContext)
	if memoryContext == "" {
		return query
	}
	return query + "\n\n可参考的历史上下文：\n" + memoryContext
}
