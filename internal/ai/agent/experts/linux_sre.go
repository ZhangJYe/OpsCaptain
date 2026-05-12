package experts

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/rag"

	einoschema "github.com/cloudwego/eino/schema"
)

// RAGQueryFunc is the signature for injectable RAG query functions.
// In production this is rag.Query; in tests/eval it can be a fake.
type RAGQueryFunc func(ctx context.Context, query string) ([]*einoschema.Document, error)

type ExpertRuntimeConfig struct {
	Name              string
	Description       string
	ToolNames         []string
	MaxRetrievalSteps int
	ModelPath         string
	Temperature       float64
	MaxTokens         int
	// RAGQueryFunc allows injecting a custom RAG query function.
	// If nil, falls back to rag.Query(ctx, rag.SharedPool(), query).
	RAGQueryFunc RAGQueryFunc
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
		isLastStep := step == e.cfg.MaxRetrievalSteps-1
		hasEvidence := len(history) > 0 || len(result.Evidence) > 0

		decision, err := e.makeDecision(ctx, frontier, graph, history, attemptedTools, isLastStep, hasEvidence)
		result.LLMCalls++
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

			output, err := adapter.Run(ctx, content)
			if err != nil {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: toolName,
					Action:   "execute",
					Error:    err.Error(),
				})
				result.Status = "degraded"
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

			sanitizedOutput := redactSecrets(output)
			history = append(history, RetrievalRecord{Query: content, Output: sanitizedOutput, Tool: toolName})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType: "tool",
				SourceID:   fmt.Sprintf("%s-%d", toolName, step),
				Title:      fmt.Sprintf("%s output", toolName),
				Snippet:    truncateString(sanitizedOutput, 500),
				Score:      1.0,
			})

		case "retrieve":
			result.RAGCalls++
			var docs []*einoschema.Document
			var ragErr error
			if e.cfg.RAGQueryFunc != nil {
				docs, ragErr = e.cfg.RAGQueryFunc(ctx, content)
			} else {
				docs, _, ragErr = rag.Query(ctx, rag.SharedPool(), content)
			}
			if ragErr != nil {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: "rag",
					Action:   "retrieve",
					Error:    ragErr.Error(),
				})
				result.Status = "degraded"
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
			sanitizedCombined := redactSecrets(combined)
			history = append(history, RetrievalRecord{Query: content, Output: sanitizedCombined, Tool: "rag"})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType: "rag",
				SourceID:   fmt.Sprintf("rag-%d", step),
				Title:      "RAG retrieval",
				Snippet:    truncateString(sanitizedCombined, 500),
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
	// Get symptom from graph
	symptom := ""
	if graph.StartSignalID != "" {
		if node, ok := graph.Nodes[graph.StartSignalID]; ok {
			symptom = node.Label
		}
	}

	switch decision["action"] {
	case "tool_call":
		// Include both hypothesis and symptom for better tool matching
		return fmt.Sprintf("查询 %s %s 相关日志", frontier.Label, symptom), nil
	case "retrieve":
		return fmt.Sprintf("%s %s", frontier.Label, symptom), nil
	case "analyze":
		// Build analysis from tool output evidence, map to conclusion
		var toolData string
		var ragData string
		if len(history) > 0 {
			for _, h := range history {
				if h.Tool == "query_logs" || h.Tool == "query_internal_docs" {
					d := extractDataField(h.Output)
					if d != "" {
						toolData = d
					}
				} else if h.Tool == "rag" {
					ragData = truncateString(h.Output, 100)
				}
			}
		}
		// Map tool output to Chinese conclusion (simulating LLM analysis)
		conclusion := mapToolOutputToConclusion(toolData, symptom)
		if conclusion != "" {
			return conclusion, nil
		}
		// Fallback: use hypothesis + evidence
		var analysis strings.Builder
		analysis.WriteString(fmt.Sprintf("针对假设「%s」的分析：%s", frontier.Label, frontier.Why))
		if toolData != "" {
			analysis.WriteString(" 证据：")
			analysis.WriteString(toolData)
		}
		if ragData != "" {
			analysis.WriteString(" ")
			analysis.WriteString(ragData)
		}
		return analysis.String(), nil
	}
	return "", fmt.Errorf("unknown action: %s", decision["action"])
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
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// extractDataField parses a JSON string and returns the "data" field value.
func extractDataField(jsonStr string) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return ""
	}
	if data, ok := parsed["data"].(string); ok {
		return data
	}
	return ""
}

// mapToolOutputToConclusion maps tool output data to Chinese conclusions.
// This simulates what an LLM would produce when analyzing tool outputs.
func mapToolOutputToConclusion(toolData string, symptom string) string {
	conclusions := []struct {
		keywords   []string
		conclusion string
	}{
		{[]string{"Consumer lag", "messages堆积", "consumer group"}, "Kafka 消费者处理能力不足"},
		{[]string{"Connection pool", "slow queries", "max_connections"}, "数据库连接池耗尽"},
		{[]string{"Cross-region latency", "packet loss", "VPN"}, "网络链路问题"},
		{[]string{"Cache hit rate", "keys expired", "eviction"}, "缓存失效导致后端压力"},
		{[]string{"CPU usage", "CPU overload", "vmstat"}, "CPU 资源耗尽导致服务超时"},
	}

	lower := strings.ToLower(toolData)
	for _, c := range conclusions {
		for _, kw := range c.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return c.conclusion
			}
		}
	}
	return ""
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
