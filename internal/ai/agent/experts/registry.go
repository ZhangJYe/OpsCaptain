package experts

import (
	"context"

	"SuperBizAgent/internal/ai/belief"
)

type ExpertAgent interface {
	Name() string
	Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *ExpertAnalysis
}

type ExpertAnalysis struct {
	ExpertName        string                 `json:"expert_name"`
	Analysis          string                 `json:"analysis"`
	Evidence          []EvidenceItem         `json:"evidence"`
	Confidence        float64                `json:"confidence"`
	Metadata          map[string]interface{} `json:"metadata"`
	Status            string                 `json:"status"`
	DegradationReason string                 `json:"degradation_reason,omitempty"`
	ToolErrors        []ToolError            `json:"tool_errors,omitempty"`
}

type ToolError struct {
	ToolName string `json:"tool_name"`
	Action   string `json:"action"`
	Error    string `json:"error"`
}

type EvidenceItem struct {
	SourceType string  `json:"source_type"`
	SourceID   string  `json:"source_id"`
	Title      string  `json:"title"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
}
