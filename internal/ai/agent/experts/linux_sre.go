package experts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/promptreg"
	"SuperBizAgent/internal/ai/rag"

	einomodel "github.com/cloudwego/eino/components/model"
	einoschema "github.com/cloudwego/eino/schema"
)

type BaseExpert struct {
	name      string
	cfg       ExpertRuntimeConfig
	adapters  map[string]*ToolAdapter
	toolNames []string
}

func NewBaseExpert(cfg ExpertRuntimeConfig, toolReg *ToolRegistry) *BaseExpert {
	adapters := make(map[string]*ToolAdapter)
	for _, tn := range cfg.ToolNames {
		if t, ok := toolReg.Get(tn); ok {
			if a, err := NewToolAdapter(tn, t); err == nil {
				adapters[tn] = a
			}
		}
	}
	return &BaseExpert{
		name:      cfg.Name,
		cfg:       cfg,
		adapters:  adapters,
		toolNames: cfg.ToolNames,
	}
}

func (e *BaseExpert) Name() string {
	return e.name
}

func (e *BaseExpert) Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *ExpertAnalysis {
	return e.run(ctx, ExpertTask{Frontier: frontier, Graph: graph})
}

func (e *BaseExpert) RunPlanned(ctx context.Context, task ExpertTask) *ExpertAnalysis {
	return e.run(ctx, task)
}

type expertExecutionContext struct {
	budget           ExecutionBudget
	expectedEvidence []string
	allowedTools     []string
	allowedToolSet   map[string]struct{}
	stopConditions   []string
}

func (e *BaseExpert) run(ctx context.Context, task ExpertTask) *ExpertAnalysis {
	execution := e.normalizeExecution(task)
	if execution.budget.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, execution.budget.Timeout)
		defer cancel()
	}
	result := &ExpertAnalysis{
		ExpertName: e.name,
		Status:     "succeeded",
		Evidence:   []EvidenceItem{},
		ToolErrors: []ToolError{},
		Metadata: map[string]interface{}{
			"expected_evidence": append([]string(nil), execution.expectedEvidence...),
			"allowed_tools":     append([]string(nil), execution.allowedTools...),
			"stop_conditions":   append([]string(nil), execution.stopConditions...),
			"budget":            execution.budget,
		},
	}

	history := []RetrievalRecord{}
	attemptedTools := make(map[string]bool)

	for step := 0; step < execution.budget.MaxRetrievalSteps; step++ {
		if err := ctx.Err(); err != nil {
			e.markContextError(result, err)
			return result
		}

		isLastStep := step == execution.budget.MaxRetrievalSteps-1
		hasEvidence := len(history) > 0 || len(result.Evidence) > 0

		decision, err := e.makeDecision(
			ctx,
			task.Frontier,
			task.Graph,
			history,
			attemptedTools,
			execution.allowedToolSet,
			result.ToolCalls < execution.budget.ToolCalls,
			result.RAGCalls < execution.budget.RAGCalls,
			isLastStep,
			hasEvidence,
		)
		if err != nil {
			result.ToolErrors = append(result.ToolErrors, ToolError{
				ToolName: "llm",
				Action:   "decision",
				Error:    err.Error(),
			})
			result.Status = "degraded"
			result.DegradationReason = fmt.Sprintf("decision_failed step %d", step)
			continue
		}
		if result.LLMCalls >= execution.budget.LLMCalls {
			e.markBudgetExhausted(result, "llm")
			return result
		}

		result.LLMCalls++
		content, err := e.generateContent(ctx, task.Frontier, task.Graph, history, decision, execution)
		if err != nil {
			result.ToolErrors = append(result.ToolErrors, ToolError{
				ToolName: "llm",
				Action:   "content",
				Error:    err.Error(),
			})
			result.Status = "degraded"
			result.DegradationReason = fmt.Sprintf("content_failed step %d", step)
			if decision["action"] != "tool_call" && decision["action"] != "retrieve" {
				continue
			}
			content = e.fallbackEvidenceQuery(task.Frontier, task.Graph)
		}

		switch decision["action"] {
		case "tool_call":
			toolName := decision["tool"]
			if _, allowed := execution.allowedToolSet[toolName]; !allowed {
				result.ToolErrors = append(result.ToolErrors, ToolError{ToolName: toolName, Action: "authorize", Error: "tool is not authorized by plan"})
				result.Status = "degraded"
				result.DegradationReason = "tool_not_authorized"
				continue
			}
			if result.ToolCalls >= execution.budget.ToolCalls {
				e.markBudgetExhausted(result, "tool")
				continue
			}
			adapter, ok := e.adapters[toolName]
			if !ok {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: toolName,
					Action:   "execute",
					Error:    "tool not found",
				})
				result.Status = "degraded"
				result.ToolCalls++
				continue
			}

			attemptedTools[toolName] = true
			result.ToolCalls++

			output, err := e.runTool(ctx, adapter, content)
			if err != nil {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: toolName,
					Action:   "execute",
					Error:    err.Error(),
				})
				result.Status = "degraded"
				result.DegradationReason = fmt.Sprintf("tool %s failed", toolName)
				continue
			}

			toolOutput := parseToolOutput(output)
			if toolOutput.IsError {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: toolName,
					Action:   "execute",
					Error:    fmt.Sprintf("tool error: %s", toolOutput.Error),
				})
				result.Status = "failed"
				result.DegradationReason = fmt.Sprintf("tool %s failed", toolName)
				continue
			}
			if toolOutput.Degraded {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: toolName,
					Action:   "execute",
					Error:    fmt.Sprintf("tool degraded: %s", toolOutput.Error),
				})
				result.Status = "degraded"
				result.DegradationReason = fmt.Sprintf("tool %s degraded", toolName)
				continue
			}
			if toolOutput.HasSuccess && !toolOutput.Success {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: toolName,
					Action:   "execute",
					Error:    fmt.Sprintf("tool success=false: %s", toolOutput.Error),
				})
				result.Status = "degraded"
				result.DegradationReason = fmt.Sprintf("tool %s success=false", toolName)
				continue
			}
			if isEmptyRetrievalOutput(output) {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: toolName,
					Action:   "execute",
					Error:    "no documents found",
				})
				result.Status = "degraded"
				result.DegradationReason = fmt.Sprintf("tool %s returned no documents", toolName)
				continue
			}

			sanitizedOutput := truncateString(redactSecrets(output), e.evidenceMaxChars())
			history = append(history, RetrievalRecord{Query: content, Output: sanitizedOutput, Tool: toolName})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType:         "tool",
				SourceID:           evidenceSourceID(toolName, sanitizedOutput),
				Title:              fmt.Sprintf("%s output", toolName),
				Snippet:            sanitizedOutput,
				Score:              1.0,
				Relation:           EvidenceRelationNeutral,
				TargetHypothesisID: task.Frontier.NodeID,
				Strength:           0,
				ToolName:           toolName,
				ObservationTime:    time.Now(),
			})

		case "retrieve":
			if result.RAGCalls >= execution.budget.RAGCalls {
				e.markBudgetExhausted(result, "rag")
				continue
			}
			result.RAGCalls++
			docs, ragErr := e.runRAG(ctx, content)
			if ragErr != nil {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: "rag",
					Action:   "retrieve",
					Error:    ragErr.Error(),
				})
				result.Status = "degraded"
				result.DegradationReason = "rag_retrieve_failed"
				continue
			}

			if len(docs) == 0 {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: "rag",
					Action:   "retrieve",
					Error:    "no documents found",
				})
				result.Status = "degraded"
				result.DegradationReason = "no_rag_hits"
				continue
			}

			var combined string
			for _, d := range docs {
				combined += d.Content + "\n"
			}
			sanitizedCombined := truncateString(redactSecrets(combined), e.evidenceMaxChars())
			history = append(history, RetrievalRecord{Query: content, Output: sanitizedCombined, Tool: "rag"})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType:         "rag",
				SourceID:           evidenceSourceID("rag", sanitizedCombined),
				Title:              "RAG retrieval",
				Snippet:            sanitizedCombined,
				Score:              1.0,
				Relation:           EvidenceRelationNeutral,
				TargetHypothesisID: task.Frontier.NodeID,
				Strength:           0,
				ObservationTime:    time.Now(),
			})

		case "analyze":
			if err := applyAnalysisProposal(result, content); err != nil {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: "llm",
					Action:   "evidence_assessment",
					Error:    err.Error(),
				})
				result.Status = "degraded"
				result.DegradationReason = "structured_assessment_invalid"
			}
			return result
		}
	}

	if result.Analysis == "" && (len(history) > 0 || len(result.Evidence) > 0) {
		if err := ctx.Err(); err != nil {
			e.markContextError(result, err)
			return result
		}
		if result.LLMCalls >= execution.budget.LLMCalls {
			e.markBudgetExhausted(result, "llm")
			return result
		}

		result.LLMCalls++
		analysis, err := e.generateContent(ctx, task.Frontier, task.Graph, history, map[string]string{
			"action":     "analyze",
			"confidence": "0.5",
		}, execution)
		if err == nil {
			if proposalErr := applyAnalysisProposal(result, analysis); proposalErr != nil {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: "llm",
					Action:   "evidence_assessment",
					Error:    proposalErr.Error(),
				})
				result.Status = "degraded"
				result.DegradationReason = "structured_assessment_invalid"
			}
			return result
		}
	}

	if result.Analysis == "" {
		result.Analysis = "信息不足，无法完成分析"
		result.Confidence = 0
		if result.Status == "succeeded" {
			result.Status = "degraded"
			result.DegradationReason = "max_steps_reached"
		}
	}
	return result
}

func (e *BaseExpert) normalizeExecution(task ExpertTask) expertExecutionContext {
	base := e.cfg.ExecutionBudget
	if base.MaxRetrievalSteps <= 0 {
		base.MaxRetrievalSteps = e.cfg.MaxRetrievalSteps
	}
	if base.MaxRetrievalSteps <= 0 {
		base.MaxRetrievalSteps = 1
	}
	if base.LLMCalls <= 0 {
		base.LLMCalls = base.MaxRetrievalSteps + 1
	}
	if base.ToolCalls <= 0 {
		base.ToolCalls = base.MaxRetrievalSteps
	}
	if base.RAGCalls <= 0 {
		base.RAGCalls = base.MaxRetrievalSteps
	}
	if base.MaxOutputTokens <= 0 {
		base.MaxOutputTokens = e.cfg.MaxTokens
	}
	if base.MaxOutputTokens <= 0 {
		base.MaxOutputTokens = 1024
	}
	budget := capExecutionBudget(base, task.Budget)

	configured := make(map[string]struct{}, len(e.toolNames))
	for _, toolName := range e.toolNames {
		configured[toolName] = struct{}{}
	}
	requested := task.AllowedTools
	if len(requested) == 0 {
		requested = e.toolNames
	}
	allowed := make([]string, 0, len(requested))
	allowedSet := make(map[string]struct{}, len(requested))
	for _, toolName := range requested {
		toolName = strings.TrimSpace(toolName)
		if _, ok := configured[toolName]; !ok {
			continue
		}
		if _, exists := allowedSet[toolName]; exists {
			continue
		}
		allowed = append(allowed, toolName)
		allowedSet[toolName] = struct{}{}
	}
	return expertExecutionContext{
		budget:           budget,
		expectedEvidence: nonEmptyValues(task.ExpectedEvidence),
		allowedTools:     allowed,
		allowedToolSet:   allowedSet,
		stopConditions:   nonEmptyValues(task.StopConditions),
	}
}

func capExecutionBudget(base ExecutionBudget, requested ExecutionBudget) ExecutionBudget {
	if requested == (ExecutionBudget{}) {
		return base
	}
	return ExecutionBudget{
		LLMCalls:          minPositive(base.LLMCalls, requested.LLMCalls),
		ToolCalls:         minPositive(base.ToolCalls, requested.ToolCalls),
		RAGCalls:          minPositive(base.RAGCalls, requested.RAGCalls),
		Timeout:           minPositiveDuration(base.Timeout, requested.Timeout),
		MaxRetrievalSteps: minPositive(base.MaxRetrievalSteps, requested.MaxRetrievalSteps),
		MaxOutputTokens:   minPositive(base.MaxOutputTokens, requested.MaxOutputTokens),
	}
}

func minPositive(a, b int) int {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func minPositiveDuration(a, b time.Duration) time.Duration {
	switch {
	case a <= 0:
		return b
	case b <= 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

func nonEmptyValues(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (e *BaseExpert) markBudgetExhausted(result *ExpertAnalysis, resource string) {
	result.ToolErrors = append(result.ToolErrors, ToolError{
		ToolName: resource,
		Action:   "budget",
		Error:    resource + " call budget exhausted",
	})
	result.Status = "degraded"
	result.DegradationReason = resource + "_budget_exhausted"
}

func (e *BaseExpert) markContextError(result *ExpertAnalysis, err error) {
	reason := "context_cancelled"
	if err == context.DeadlineExceeded {
		reason = "expert_timeout"
	}
	result.ToolErrors = append(result.ToolErrors, ToolError{
		ToolName: "context",
		Action:   reason,
		Error:    err.Error(),
	})
	result.Status = "degraded"
	result.DegradationReason = reason
}

func (e *BaseExpert) fallbackEvidenceQuery(frontier *belief.Frontier, graph *belief.BeliefGraph) string {
	parts := make([]string, 0, 3)
	if graph != nil && graph.StartSignalID != "" {
		if node, ok := graph.Nodes[graph.StartSignalID]; ok && strings.TrimSpace(node.Label) != "" {
			parts = append(parts, strings.TrimSpace(node.Label))
		}
	}
	if frontier != nil {
		if label := strings.TrimSpace(frontier.Label); label != "" {
			parts = append(parts, label)
		}
		if why := strings.TrimSpace(frontier.Why); why != "" {
			parts = append(parts, why)
		}
	}
	if len(parts) == 0 {
		return "检查当前故障的指标、日志和追踪证据"
	}
	return strings.Join(parts, "；")
}

func evidenceSourceID(sourceName, content string) string {
	sum := sha256.Sum256([]byte(canonicalEvidenceIdentity(sourceName, content)))
	return fmt.Sprintf("%s:%x", sourceName, sum[:8])
}

func canonicalEvidenceIdentity(sourceName, content string) string {
	content = strings.TrimSpace(content)
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return content
	}

	switch sourceName {
	case "query_logs":
		delete(payload, "query")
		delete(payload, "message")
	case "query_prometheus_instant":
		delete(payload, "query")
		delete(payload, "time")
		delete(payload, "message")
		stripPrometheusInstantTimestamps(payload["samples"])
		stripPrometheusInstantTimestamps(payload["evidence"])
		stripPrometheusInstantTimestamps(payload["scalar"])
	default:
		return content
	}

	canonical, err := json.Marshal(payload)
	if err != nil {
		return content
	}
	return string(canonical)
}

func stripPrometheusInstantTimestamps(value any) {
	switch items := value.(type) {
	case []any:
		for _, item := range items {
			if sample, ok := item.(map[string]any); ok {
				delete(sample, "timestamp")
			}
		}
	case map[string]any:
		delete(items, "timestamp")
	}
}

type analysisProposal struct {
	Analysis                    string                 `json:"analysis"`
	Confidence                  float64                `json:"confidence"`
	Evidence                    []evidenceAssessment   `json:"evidence"`
	Refinements                 []HypothesisRefinement `json:"refinements,omitempty"`
	CurrentHypothesisActionable optionalBoolean        `json:"current_hypothesis_actionable,omitempty"`
}

type optionalBoolean struct {
	value bool
	set   bool
}

func (b *optionalBoolean) UnmarshalJSON(data []byte) error {
	switch string(data) {
	case "true", `"true"`:
		b.value, b.set = true, true
	case "false", `"false"`:
		b.value, b.set = false, true
	case "null":
		b.value, b.set = false, false
	default:
		return fmt.Errorf("current_hypothesis_actionable must be a JSON boolean")
	}
	return nil
}

type evidenceAssessment struct {
	Index    int              `json:"index"`
	Relation EvidenceRelation `json:"relation"`
	Strength float64          `json:"strength"`
}

func applyAnalysisProposal(result *ExpertAnalysis, content string) error {
	if result == nil {
		return fmt.Errorf("expert analysis result is required")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(content))
	decoder.DisallowUnknownFields()
	var proposal analysisProposal
	if err := decoder.Decode(&proposal); err != nil {
		return fmt.Errorf("decode structured analysis proposal: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("structured analysis proposal must contain one JSON object")
	}
	if strings.TrimSpace(proposal.Analysis) == "" {
		return fmt.Errorf("structured analysis proposal requires analysis")
	}
	if proposal.Confidence < 0 || proposal.Confidence > 1 {
		return fmt.Errorf("structured analysis confidence must be within [0,1]")
	}
	if len(proposal.Evidence) != len(result.Evidence) {
		return fmt.Errorf("structured analysis must assess all %d evidence items", len(result.Evidence))
	}
	seen := make(map[int]struct{}, len(proposal.Evidence))
	hasDirectionalEvidence := false
	hasSupportingEvidence := false
	for _, assessment := range proposal.Evidence {
		if assessment.Index < 0 || assessment.Index >= len(result.Evidence) {
			return fmt.Errorf("evidence assessment index %d is out of range", assessment.Index)
		}
		if _, exists := seen[assessment.Index]; exists {
			return fmt.Errorf("evidence assessment index %d is duplicated", assessment.Index)
		}
		seen[assessment.Index] = struct{}{}
		switch assessment.Relation {
		case EvidenceRelationSupport, EvidenceRelationRefute:
			if assessment.Strength <= 0 || assessment.Strength > 1 {
				return fmt.Errorf("%s evidence strength must be within (0,1]", assessment.Relation)
			}
			hasDirectionalEvidence = true
			if assessment.Relation == EvidenceRelationSupport {
				hasSupportingEvidence = true
			}
		case EvidenceRelationNeutral:
			if assessment.Strength < 0 || assessment.Strength > 1 {
				return fmt.Errorf("neutral evidence strength must be within [0,1]")
			}
		default:
			return fmt.Errorf("unsupported evidence relation %q", assessment.Relation)
		}
	}
	if len(proposal.Refinements) > 0 && !hasDirectionalEvidence {
		return fmt.Errorf("structured analysis refinements require at least one support or refute evidence assessment")
	}
	if proposal.CurrentHypothesisActionable.set && proposal.CurrentHypothesisActionable.value && !hasSupportingEvidence {
		return fmt.Errorf("marking the current hypothesis actionable requires at least one support evidence assessment")
	}
	refinementLabels := make(map[string]struct{}, len(proposal.Refinements))
	for _, refinement := range proposal.Refinements {
		label := strings.TrimSpace(refinement.Label)
		if label == "" || strings.TrimSpace(refinement.Why) == "" {
			return fmt.Errorf("refinement label and why are required")
		}
		if refinement.Score <= 0 || refinement.Score > 1 {
			return fmt.Errorf("refinement score must be within (0,1]")
		}
		key := strings.ToLower(label)
		if _, exists := refinementLabels[key]; exists {
			return fmt.Errorf("refinement label %q is duplicated", label)
		}
		refinementLabels[key] = struct{}{}
	}

	result.Analysis = strings.TrimSpace(proposal.Analysis)
	result.Confidence = proposal.Confidence
	for _, assessment := range proposal.Evidence {
		result.Evidence[assessment.Index].Relation = assessment.Relation
		result.Evidence[assessment.Index].Strength = assessment.Strength
	}
	result.Refinements = append([]HypothesisRefinement(nil), proposal.Refinements...)
	if proposal.CurrentHypothesisActionable.set {
		actionable := proposal.CurrentHypothesisActionable.value
		result.CurrentHypothesisActionable = &actionable
	}
	return nil
}

func (e *BaseExpert) callTimeout() time.Duration {
	if e.cfg.CallTimeout > 0 {
		return e.cfg.CallTimeout
	}
	return 5 * time.Second
}

func (e *BaseExpert) evidenceMaxChars() int {
	if e.cfg.EvidenceMaxChars > 0 {
		return e.cfg.EvidenceMaxChars
	}
	return 500
}

func (e *BaseExpert) runTool(ctx context.Context, adapter *ToolAdapter, content string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, e.callTimeout())
	defer cancel()
	return adapter.Run(callCtx, content)
}

func (e *BaseExpert) runRAG(ctx context.Context, content string) ([]*einoschema.Document, error) {
	callCtx, cancel := context.WithTimeout(ctx, e.callTimeout())
	defer cancel()

	if e.cfg.RAGQueryFunc != nil {
		return e.cfg.RAGQueryFunc(callCtx, content)
	}
	docs, _, err := rag.Query(callCtx, rag.SharedPool(), content)
	return docs, err
}

func (e *BaseExpert) makeDecision(
	ctx context.Context,
	frontier *belief.Frontier,
	graph *belief.BeliefGraph,
	history []RetrievalRecord,
	attemptedTools map[string]bool,
	allowedTools map[string]struct{},
	toolBudgetAvailable bool,
	ragBudgetAvailable bool,
	isLastStep bool,
	hasEvidence bool,
) (map[string]string, error) {
	if toolBudgetAvailable && len(e.adapters) > 0 {
		for _, toolName := range e.toolNames {
			if _, allowed := allowedTools[toolName]; !allowed {
				continue
			}
			if _, ok := e.adapters[toolName]; ok {
				if !attemptedTools[toolName] {
					return map[string]string{
						"action": "tool_call",
						"tool":   toolName,
						"reason": "collect complementary tool evidence",
					}, nil
				}
			}
		}
	}

	if isLastStep && hasEvidence {
		return map[string]string{
			"action":     "analyze",
			"reason":     "last step with evidence",
			"confidence": "0.5",
		}, nil
	}

	if ragBudgetAvailable && len(history) < 2 {
		return map[string]string{
			"action": "retrieve",
			"reason": "gather more evidence",
		}, nil
	}

	return map[string]string{
		"action":     "analyze",
		"reason":     "sufficient evidence gathered",
		"confidence": "0.8",
	}, nil
}

func (e *BaseExpert) generateContent(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string, execution expertExecutionContext) (string, error) {
	if e.cfg.GenerateContentFunc != nil {
		return e.cfg.GenerateContentFunc(ctx, frontier, graph, history, decision)
	}
	return e.generateContentWithLLM(ctx, frontier, graph, history, decision, execution)
}

func (e *BaseExpert) generateContentWithLLM(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string, execution expertExecutionContext) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, e.callTimeout())
	defer cancel()

	factory := e.cfg.ChatModelFactory
	if factory == nil {
		factory = models.OpenAIForGLMFast
	}
	chatModel, err := factory(callCtx)
	if err != nil {
		return "", err
	}

	symptom := ""
	if graph.StartSignalID != "" {
		if node, ok := graph.Nodes[graph.StartSignalID]; ok {
			symptom = node.Label
		}
	}

	prompt := e.buildContentPrompt(symptom, frontier, graph, history, decision, execution)
	messages := []*einoschema.Message{
		einoschema.SystemMessage(promptreg.GOSExpertSystem),
		einoschema.UserMessage(prompt),
	}
	resp, err := chatModel.Generate(callCtx, messages, einomodel.WithMaxTokens(execution.budget.MaxOutputTokens))
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("empty llm response")
	}

	content := strings.TrimSpace(resp.Content)
	if content == "" {
		return "", fmt.Errorf("empty llm content")
	}
	return redactSecrets(content), nil
}

func (e *BaseExpert) buildContentPrompt(symptom string, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string, execution expertExecutionContext) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("专家：%s\n", e.name))
	b.WriteString(fmt.Sprintf("动作：%s\n", decision["action"]))
	if toolName := strings.TrimSpace(decision["tool"]); toolName != "" {
		b.WriteString(fmt.Sprintf("工具：%s\n", toolName))
	}
	b.WriteString(fmt.Sprintf("假设：%s\n", frontier.Label))
	b.WriteString(fmt.Sprintf("依据：%s\n", frontier.Why))
	b.WriteString(fmt.Sprintf("症状：%s\n", symptom))
	if graphSummary := compressedGraphSummary(graph); graphSummary != "" {
		b.WriteString("相关压缩图：\n")
		b.WriteString(graphSummary)
		b.WriteString("\n")
	}
	if len(execution.expectedEvidence) > 0 {
		b.WriteString("预期证据：" + strings.Join(execution.expectedEvidence, "；") + "\n")
	}
	if len(execution.allowedTools) > 0 {
		b.WriteString("授权工具：" + strings.Join(execution.allowedTools, "、") + "\n")
	}
	if len(execution.stopConditions) > 0 {
		b.WriteString("停止条件：" + strings.Join(execution.stopConditions, "；") + "\n")
	}
	if len(history) > 0 {
		b.WriteString("已获得证据：\n")
		for index, h := range history {
			b.WriteString(fmt.Sprintf("- 索引=%d 来源=%s 查询=%s 输出=%s\n", index, h.Tool, h.Query, h.Output))
		}
	}
	switch decision["action"] {
	case "tool_call":
		b.WriteString(promptreg.GOSExpertToolCall)
	case "retrieve":
		b.WriteString(promptreg.GOSExpertRetrieve)
	case "analyze":
		b.WriteString(promptreg.GOSExpertAnalyze)
	default:
		b.WriteString("请输出下一步分析内容。")
	}
	return b.String()
}

func compressedGraphSummary(graph *belief.BeliefGraph) string {
	if graph == nil {
		return ""
	}
	nodes := graph.GetActiveNodeCopies()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	parts := make([]string, 0, len(nodes))
	for _, node := range nodes {
		parts = append(parts, fmt.Sprintf("%s|%s|L%d|%.2f|%s", node.ID, node.Type, node.Level, node.Score, strings.TrimSpace(node.Label)))
	}
	return strings.Join(parts, "\n")
}
