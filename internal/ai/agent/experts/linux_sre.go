package experts

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/rag"

	einomodel "github.com/cloudwego/eino/components/model"
	einoschema "github.com/cloudwego/eino/schema"
)

type RAGQueryFunc func(ctx context.Context, query string) ([]*einoschema.Document, error)

type GenerateContentFunc func(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error)
type ChatModelFactory func(ctx context.Context) (einomodel.ToolCallingChatModel, error)

type ExpertRuntimeConfig struct {
	Name                string
	Description         string
	ToolNames           []string
	MaxRetrievalSteps   int
	ModelPath           string
	Temperature         float64
	MaxTokens           int
	RAGQueryFunc        RAGQueryFunc
	GenerateContentFunc GenerateContentFunc
	ChatModelFactory    ChatModelFactory
	CallTimeout         time.Duration
}

type RetrievalRecord struct {
	Query  string
	Output string
	Tool   string
}

type ToolOutput struct {
	Success           bool        `json:"success"`
	Degraded          bool        `json:"degraded"`
	Error             string      `json:"error"`
	IsError           bool        `json:"isError"`
	Content           interface{} `json:"content"`
	Data              interface{} `json:"data"`
	HasExplicitFields bool        `json:"-"`
	HasSuccess        bool        `json:"-"`
}

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
	result := &ExpertAnalysis{
		ExpertName: e.name,
		Status:     "succeeded",
		Evidence:   []EvidenceItem{},
		ToolErrors: []ToolError{},
	}

	history := []RetrievalRecord{}
	attemptedTools := make(map[string]bool)

	for step := 0; step < e.cfg.MaxRetrievalSteps; step++ {
		if err := ctx.Err(); err != nil {
			result.ToolErrors = append(result.ToolErrors, ToolError{
				ToolName: "context",
				Action:   "cancelled",
				Error:    err.Error(),
			})
			result.Status = "degraded"
			result.DegradationReason = "context_cancelled"
			return result
		}

		isLastStep := step == e.cfg.MaxRetrievalSteps-1
		hasEvidence := len(history) > 0 || len(result.Evidence) > 0

		decision, err := e.makeDecision(ctx, frontier, graph, history, attemptedTools, isLastStep, hasEvidence)
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

		content, err := e.generateContent(ctx, frontier, graph, history, decision)
		result.LLMCalls++
		if err != nil {
			result.ToolErrors = append(result.ToolErrors, ToolError{
				ToolName: "llm",
				Action:   "content",
				Error:    err.Error(),
			})
			result.Status = "degraded"
			result.DegradationReason = fmt.Sprintf("content_failed step %d", step)
			continue
		}

		switch decision["action"] {
		case "tool_call":
			toolName := decision["tool"]
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

			sanitizedOutput := truncateString(redactSecrets(output), 500)
			history = append(history, RetrievalRecord{Query: content, Output: sanitizedOutput, Tool: toolName})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType: "tool",
				SourceID:   fmt.Sprintf("%s-%d", toolName, step),
				Title:      fmt.Sprintf("%s output", toolName),
				Snippet:    sanitizedOutput,
				Score:      1.0,
			})

		case "retrieve":
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
			sanitizedCombined := truncateString(redactSecrets(combined), 500)
			history = append(history, RetrievalRecord{Query: content, Output: sanitizedCombined, Tool: "rag"})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType: "rag",
				SourceID:   fmt.Sprintf("rag-%d", step),
				Title:      "RAG retrieval",
				Snippet:    sanitizedCombined,
				Score:      1.0,
			})

		case "analyze":
			result.Analysis = content
			if conf, ok := decision["confidence"]; ok {
				fmt.Sscanf(conf, "%f", &result.Confidence)
			}
			return result
		}
	}

	if result.Analysis == "" && (len(history) > 0 || len(result.Evidence) > 0) {
		if err := ctx.Err(); err != nil {
			result.ToolErrors = append(result.ToolErrors, ToolError{
				ToolName: "context",
				Action:   "cancelled",
				Error:    err.Error(),
			})
			result.Status = "degraded"
			result.DegradationReason = "context_cancelled"
			return result
		}

		analysis, err := e.generateContent(ctx, frontier, graph, history, map[string]string{
			"action":     "analyze",
			"confidence": "0.5",
		})
		if err == nil {
			result.Analysis = analysis
			result.Confidence = 0.5
			if result.Status == "succeeded" {
				result.Status = "degraded"
				result.DegradationReason = "forced_analyze_with_partial_evidence"
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

func (e *BaseExpert) callTimeout() time.Duration {
	if e.cfg.CallTimeout > 0 {
		return e.cfg.CallTimeout
	}
	return 5 * time.Second
}

func (e *BaseExpert) runTool(ctx context.Context, adapter *ToolAdapter, content string) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, e.callTimeout())
	defer cancel()

	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		output, err := adapter.Run(callCtx, content)
		ch <- result{output: output, err: err}
	}()

	select {
	case res := <-ch:
		return res.output, res.err
	case <-callCtx.Done():
		return "", callCtx.Err()
	}
}

func (e *BaseExpert) runRAG(ctx context.Context, content string) ([]*einoschema.Document, error) {
	callCtx, cancel := context.WithTimeout(ctx, e.callTimeout())
	defer cancel()

	type result struct {
		docs []*einoschema.Document
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		var docs []*einoschema.Document
		var err error
		if e.cfg.RAGQueryFunc != nil {
			docs, err = e.cfg.RAGQueryFunc(callCtx, content)
		} else {
			docs, _, err = rag.Query(callCtx, rag.SharedPool(), content)
		}
		ch <- result{docs: docs, err: err}
	}()

	select {
	case res := <-ch:
		return res.docs, res.err
	case <-callCtx.Done():
		return nil, callCtx.Err()
	}
}

func (e *BaseExpert) makeDecision(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, attemptedTools map[string]bool, isLastStep bool, hasEvidence bool) (map[string]string, error) {
	if isLastStep && hasEvidence {
		return map[string]string{
			"action":     "analyze",
			"reason":     "last step with evidence",
			"confidence": "0.5",
		}, nil
	}

	if len(history) == 0 && len(e.adapters) > 0 {
		for _, toolName := range e.toolNames {
			if _, ok := e.adapters[toolName]; ok {
				if !attemptedTools[toolName] {
					return map[string]string{
						"action": "tool_call",
						"tool":   toolName,
						"reason": "initial tool call",
					}, nil
				}
			}
		}
	}

	if len(history) < 2 {
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

func (e *BaseExpert) generateContent(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error) {
	if e.cfg.GenerateContentFunc != nil {
		return e.cfg.GenerateContentFunc(ctx, frontier, graph, history, decision)
	}
	return e.generateContentWithLLM(ctx, frontier, graph, history, decision)
}

func (e *BaseExpert) generateContentWithLLM(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error) {
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

	prompt := e.buildContentPrompt(symptom, frontier, history, decision)
	messages := []*einoschema.Message{
		einoschema.SystemMessage("你是 AIOps SRE 专家。只输出当前步骤需要的内容，不要输出解释、Markdown 或多余前后缀。"),
		einoschema.UserMessage(prompt),
	}
	type llmResult struct {
		resp *einoschema.Message
		err  error
	}
	ch := make(chan llmResult, 1)
	go func() {
		resp, err := chatModel.Generate(callCtx, messages)
		ch <- llmResult{resp: resp, err: err}
	}()

	var resp *einoschema.Message
	select {
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		resp = res.resp
	case <-callCtx.Done():
		return "", callCtx.Err()
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

func (e *BaseExpert) buildContentPrompt(symptom string, frontier *belief.Frontier, history []RetrievalRecord, decision map[string]string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("专家：%s\n", e.name))
	b.WriteString(fmt.Sprintf("动作：%s\n", decision["action"]))
	b.WriteString(fmt.Sprintf("假设：%s\n", frontier.Label))
	b.WriteString(fmt.Sprintf("依据：%s\n", frontier.Why))
	b.WriteString(fmt.Sprintf("症状：%s\n", symptom))
	if len(history) > 0 {
		b.WriteString("已获得证据：\n")
		for _, h := range history {
			b.WriteString(fmt.Sprintf("- 来源=%s 查询=%s 输出=%s\n", h.Tool, h.Query, h.Output))
		}
	}
	switch decision["action"] {
	case "tool_call":
		b.WriteString("请生成一句适合传给日志或知识库工具的中文查询语句。")
	case "retrieve":
		b.WriteString("请生成一句适合传给 RAG 的检索查询语句。")
	case "analyze":
		b.WriteString("请基于证据输出最终诊断结论和简短建议。")
	default:
		b.WriteString("请输出下一步分析内容。")
	}
	return b.String()
}

func parseToolOutput(output string) ToolOutput {
	var toolOutput ToolOutput
	if err := json.Unmarshal([]byte(output), &toolOutput); err != nil {
		return ToolOutput{
			Content: output,
		}
	}

	hasExplicitFields := false
	hasSuccess := false
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(output), &raw); err == nil {
		if _, ok := raw["success"]; ok {
			hasExplicitFields = true
			hasSuccess = true
		}
		if _, ok := raw["degraded"]; ok {
			hasExplicitFields = true
		}
		if _, ok := raw["isError"]; ok {
			hasExplicitFields = true
		}
	}

	toolOutput.HasExplicitFields = hasExplicitFields
	toolOutput.HasSuccess = hasSuccess

	if !hasExplicitFields {
		return ToolOutput{
			Content: output,
		}
	}

	return toolOutput
}

func isEmptyRetrievalOutput(output string) bool {
	s := strings.TrimSpace(output)
	if s == "" || s == "[]" || s == "null" {
		return true
	}

	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		return len(arr) == 0
	}

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return false
	}

	for _, key := range []string{"data", "content"} {
		value, ok := obj[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case []interface{}:
			if len(v) == 0 {
				return true
			}
		case string:
			if strings.TrimSpace(v) == "" || strings.TrimSpace(v) == "[]" {
				return true
			}
		}
	}

	return false
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret|authorization)\s*[:=]\s*["']?([^\s"']{8,})["']?`),
	regexp.MustCompile(`(?i)bearer\s+[^\s]+`),
	regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
	regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
}

func redactSecrets(s string) string {
	result := s
	for _, pattern := range secretPatterns {
		result = pattern.ReplaceAllString(result, "[REDACTED]")
	}
	return result
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

type LinuxSREExpert struct {
	*BaseExpert
}

func NewLinuxSREExpert(cfg ExpertRuntimeConfig, toolReg *ToolRegistry) *LinuxSREExpert {
	return &LinuxSREExpert{
		BaseExpert: NewBaseExpert(cfg, toolReg),
	}
}

type NetworkSREExpert struct {
	*BaseExpert
}

func NewNetworkSREExpert(cfg ExpertRuntimeConfig, toolReg *ToolRegistry) *NetworkSREExpert {
	return &NetworkSREExpert{
		BaseExpert: NewBaseExpert(cfg, toolReg),
	}
}

type DatabaseSREExpert struct {
	*BaseExpert
}

func NewDatabaseSREExpert(cfg ExpertRuntimeConfig, toolReg *ToolRegistry) *DatabaseSREExpert {
	return &DatabaseSREExpert{
		BaseExpert: NewBaseExpert(cfg, toolReg),
	}
}
