package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"

	"github.com/gogf/gf/v2/frame/g"
)

const AgentName = "supervisor"

const (
	defaultExecutionMode = "parallel"
	executionModeStaged  = "staged"
)

type Agent struct{}

type reflectionResult struct {
	Status         string
	Reason         string
	MissingDomains []string
}

var (
	supervisorExecutionMode = func(ctx context.Context) string {
		v, err := g.Cfg().Get(ctx, "multi_agent.execution_mode")
		if err == nil && strings.TrimSpace(v.String()) != "" {
			return strings.ToLower(strings.TrimSpace(v.String()))
		}
		return defaultExecutionMode
	}
	supervisorSelfReflectEnabled = func(ctx context.Context) bool {
		v, err := g.Cfg().Get(ctx, "multi_agent.self_reflect_enabled")
		return err == nil && v.Bool()
	}
)

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
	if useMultiAgent, ok := metadataBool(triageResult.Metadata, "use_multi_agent"); ok && !useMultiAgent {
		rawDomains = nil
	} else if len(rawDomains) == 0 {
		rawDomains = []string{"metrics", "logs", "knowledge"}
	}

	executionMode := normalizeExecutionMode(supervisorExecutionMode(ctx))
	childResults := dispatchSpecialists(ctx, rt, task, rawDomains, specialistQuery, rawQuery, memoryContext, responseMode, intent, executionMode)

	reportTask := protocol.NewChildTask(task, reporter.AgentName, rawQuery, map[string]any{
		"query":          rawQuery,
		"intent":         intent,
		"response_mode":  responseMode,
		"results":        childResults,
		"execution_mode": executionMode,
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
	metadata := make(map[string]any, len(reportResult.Metadata)+8)
	for k, v := range reportResult.Metadata {
		metadata[k] = v
	}
	copyTriageMetadata(metadata, triageResult.Metadata)
	metadata["intent"] = intent
	metadata["domains"] = append([]string(nil), rawDomains...)
	metadata["execution_mode"] = executionMode
	metadata["self_reflect_enabled"] = supervisorSelfReflectEnabled(ctx)
	if supervisorSelfReflectEnabled(ctx) {
		reflection := reflectExecution(reportResult, childResults, rawDomains)
		metadata["reflection_status"] = reflection.Status
		metadata["reflection_reason"] = reflection.Reason
		metadata["reflection_missing_domains"] = reflection.MissingDomains
	}

	return &protocol.TaskResult{
		TaskID:            task.TaskID,
		Agent:             a.Name(),
		Status:            reportResult.Status,
		Summary:           reportResult.Summary,
		Confidence:        reportResult.Confidence,
		DegradationReason: reportResult.DegradationReason,
		Evidence:          evidence,
		Metadata:          metadata,
	}, nil
}

func dispatchSpecialists(ctx context.Context, rt *runtime.Runtime, task *protocol.TaskEnvelope, domains []string, specialistQuery, rawQuery, memoryContext, responseMode, intent, executionMode string) []*protocol.TaskResult {
	if executionMode == executionModeStaged {
		return dispatchSpecialistsStaged(ctx, rt, task, domains, specialistQuery, rawQuery, memoryContext, responseMode, intent)
	}
	return dispatchSpecialistsParallel(ctx, rt, task, domains, specialistQuery, rawQuery, memoryContext, responseMode, intent)
}

func dispatchSpecialistsParallel(ctx context.Context, rt *runtime.Runtime, task *protocol.TaskEnvelope, domains []string, specialistQuery, rawQuery, memoryContext, responseMode, intent string) []*protocol.TaskResult {
	childResults := make([]*protocol.TaskResult, 0, len(domains))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, domain := range domains {
		agentName := specialistAgentName(domain)
		if agentName == "" {
			continue
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			result := dispatchSpecialist(ctx, rt, task, name, specialistQuery, rawQuery, memoryContext, responseMode, intent, nil)
			mu.Lock()
			childResults = append(childResults, result)
			mu.Unlock()
		}(agentName)
	}
	wg.Wait()
	return childResults
}

func dispatchSpecialistsStaged(ctx context.Context, rt *runtime.Runtime, task *protocol.TaskEnvelope, domains []string, specialistQuery, rawQuery, memoryContext, responseMode, intent string) []*protocol.TaskResult {
	childResults := make([]*protocol.TaskResult, 0, len(domains))
	for _, domain := range domains {
		agentName := specialistAgentName(domain)
		if agentName == "" {
			continue
		}
		result := dispatchSpecialist(ctx, rt, task, agentName, specialistQuery, rawQuery, memoryContext, responseMode, intent, resultSummaries(childResults))
		childResults = append(childResults, result)
	}
	return childResults
}

func dispatchSpecialist(ctx context.Context, rt *runtime.Runtime, task *protocol.TaskEnvelope, agentName, specialistQuery, rawQuery, memoryContext, responseMode, intent string, priorResults []string) *protocol.TaskResult {
	childGoal := withPriorResults(specialistQuery, priorResults)
	childTask := protocol.NewChildTask(task, agentName, childGoal, map[string]any{
		"query":          rawQuery,
		"raw_query":      rawQuery,
		"memory_context": memoryContext,
		"response_mode":  responseMode,
		"intent":         intent,
		"prior_results":  priorResults,
	})
	childTask.MemoryRefs = append([]protocol.MemoryRef(nil), task.MemoryRefs...)
	result, dispatchErr := rt.Dispatch(ctx, childTask)
	if result != nil {
		if ref, artifactErr := createResultArtifact(ctx, rt, result); artifactErr == nil && ref != nil {
			result.ArtifactRefs = append(result.ArtifactRefs, *ref)
		}
		return result
	}
	if dispatchErr != nil {
		return &protocol.TaskResult{
			TaskID:     childTask.TaskID,
			Agent:      agentName,
			Status:     protocol.ResultStatusFailed,
			Summary:    dispatchErr.Error(),
			Confidence: 0,
			Error: &protocol.TaskError{
				Message: dispatchErr.Error(),
			},
		}
	}
	return &protocol.TaskResult{
		TaskID:            childTask.TaskID,
		Agent:             agentName,
		Status:            protocol.ResultStatusDegraded,
		Summary:           "specialist returned empty result",
		Confidence:        0,
		DegradationReason: "empty_specialist_result",
	}
}

func createResultArtifact(ctx context.Context, rt *runtime.Runtime, result *protocol.TaskResult) (*protocol.ArtifactRef, error) {
	payload, err := json.Marshal(map[string]any{
		"agent":              result.Agent,
		"status":             result.Status,
		"summary":            result.Summary,
		"confidence":         result.Confidence,
		"degradation_reason": result.DegradationReason,
		"evidence":           result.Evidence,
	})
	if err != nil {
		return nil, err
	}
	return rt.CreateArtifact(ctx, "task_result", string(payload), map[string]any{
		"agent": result.Agent,
		"task":  result.TaskID,
	})
}

func normalizeExecutionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case executionModeStaged:
		return executionModeStaged
	default:
		return defaultExecutionMode
	}
}

func resultSummaries(results []*protocol.TaskResult) []string {
	out := make([]string, 0, len(results))
	for _, result := range results {
		if result == nil || strings.TrimSpace(result.Summary) == "" {
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s", result.Agent, strings.TrimSpace(result.Summary)))
	}
	return out
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

func metadataBool(metadata map[string]any, key string) (bool, bool) {
	if metadata == nil {
		return false, false
	}
	switch value := metadata[key].(type) {
	case bool:
		return value, true
	case string:
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	}
	return false, false
}

func copyTriageMetadata(dst, src map[string]any) {
	for _, key := range []string{
		"priority",
		"use_multi_agent",
		"matched_rule",
		"triage_fallback",
		"triage_mode",
		"triage_source",
		"llm_error",
	} {
		if value, ok := src[key]; ok {
			dst[key] = value
		}
	}
}

func withMemoryContext(query, memoryContext string) string {
	query = strings.TrimSpace(query)
	memoryContext = strings.TrimSpace(memoryContext)
	if memoryContext == "" {
		return query
	}
	return query + "\n\n可参考的历史上下文：\n" + memoryContext
}

func withPriorResults(query string, priorResults []string) string {
	if len(priorResults) == 0 {
		return query
	}
	return strings.TrimSpace(query) + "\n\n上游专业代理结果：\n" + strings.Join(prefixEach(priorResults, "- "), "\n")
}

func reflectExecution(reportResult *protocol.TaskResult, childResults []*protocol.TaskResult, domains []string) reflectionResult {
	if len(domains) > 0 && len(childResults) == 0 {
		return reflectionResult{
			Status:         "needs_more_evidence",
			Reason:         "triage selected domains but no specialist result was produced",
			MissingDomains: append([]string(nil), domains...),
		}
	}
	if reportResult == nil || reportResult.Status != protocol.ResultStatusSucceeded {
		return reflectionResult{
			Status: "needs_attention",
			Reason: "final report is degraded",
		}
	}
	for _, result := range childResults {
		if result == nil || result.Status != protocol.ResultStatusSucceeded {
			return reflectionResult{
				Status: "needs_attention",
				Reason: "one or more specialist results are degraded",
			}
		}
	}
	return reflectionResult{
		Status: "complete",
		Reason: "all selected specialists and reporter completed",
	}
}

func prefixEach(items []string, prefix string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, prefix+item)
	}
	return out
}
