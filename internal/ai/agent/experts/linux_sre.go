package experts

import (
	"context"
	"fmt"

	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/rag"
)

type ExpertRuntimeConfig struct {
	Name              string
	Description       string
	ToolNames         []string
	MaxRetrievalSteps int
	ModelPath         string
	Temperature       float64
	MaxTokens         int
}

type RetrievalRecord struct {
	Query  string
	Output string
	Tool   string
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

	for step := 0; step < e.cfg.MaxRetrievalSteps; step++ {
		decision, err := e.makeDecision(ctx, frontier, graph, history)
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
				continue
			}

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

			history = append(history, RetrievalRecord{Query: content, Output: output, Tool: toolName})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType: "tool",
				SourceID:   fmt.Sprintf("%s-%d", toolName, step),
				Title:      fmt.Sprintf("%s output", toolName),
				Snippet:    truncateAndSanitize(output, 500),
				Score:      1.0,
			})

		case "retrieve":
			docs, _, err := rag.Query(ctx, rag.SharedPool(), content)
			if err != nil {
				result.ToolErrors = append(result.ToolErrors, ToolError{
					ToolName: "rag",
					Action:   "retrieve",
					Error:    err.Error(),
				})
				result.Status = "degraded"
				continue
			}

			var combined string
			for _, d := range docs {
				combined += d.Content + "\n"
			}
			history = append(history, RetrievalRecord{Query: content, Output: combined, Tool: "rag"})
			result.Evidence = append(result.Evidence, EvidenceItem{
				SourceType: "rag",
				SourceID:   fmt.Sprintf("rag-%d", step),
				Title:      "RAG retrieval",
				Snippet:    truncateAndSanitize(combined, 500),
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

func (e *BaseExpert) makeDecision(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord) (map[string]string, error) {
	return map[string]string{
		"action": "analyze",
		"reason": "default decision",
	}, nil
}

func (e *BaseExpert) generateContent(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error) {
	return fmt.Sprintf("Analysis for %s: %s", frontier.Label, frontier.Why), nil
}

func truncateAndSanitize(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
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
