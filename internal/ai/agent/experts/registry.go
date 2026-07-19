package experts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"SuperBizAgent/internal/ai/belief"

	"github.com/cloudwego/eino/components/tool"
)

type ExpertAgent interface {
	Name() string
	Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *ExpertAnalysis
}

type PlannedExpertAgent interface {
	ExpertAgent
	RunPlanned(ctx context.Context, task ExpertTask) *ExpertAnalysis
}

type ExpertTask struct {
	Frontier          *belief.Frontier
	Graph             *belief.BeliefGraph
	ExpectedEvidence  []string
	AllowedTools      []string
	StopConditions    []string
	Budget            ExecutionBudget
	StopAfterEvidence bool
}

type ExecutionBudget struct {
	LLMCalls          int
	ToolCalls         int
	RAGCalls          int
	Timeout           time.Duration
	MaxRetrievalSteps int
	MaxOutputTokens   int
}

type ExpertAnalysis struct {
	ExpertName                  string                 `json:"expert_name"`
	Analysis                    string                 `json:"analysis"`
	Evidence                    []EvidenceItem         `json:"evidence"`
	Refinements                 []HypothesisRefinement `json:"refinements,omitempty"`
	CurrentHypothesisActionable *bool                  `json:"current_hypothesis_actionable,omitempty"`
	Confidence                  float64                `json:"confidence"`
	Metadata                    map[string]interface{} `json:"metadata"`
	Status                      string                 `json:"status"`
	DegradationReason           string                 `json:"degradation_reason,omitempty"`
	ToolErrors                  []ToolError            `json:"tool_errors,omitempty"`
	ToolCalls                   int                    `json:"tool_calls"`
	RAGCalls                    int                    `json:"rag_calls"`
	LLMCalls                    int                    `json:"llm_calls"`
}

type HypothesisRefinement struct {
	Label      string  `json:"label"`
	Score      float64 `json:"score"`
	Why        string  `json:"why"`
	Actionable bool    `json:"actionable"`
}

func (r *HypothesisRefinement) UnmarshalJSON(data []byte) error {
	type refinementJSON struct {
		Label      string          `json:"label"`
		Score      float64         `json:"score"`
		Why        string          `json:"why"`
		Actionable json.RawMessage `json:"actionable"`
	}
	var raw refinementJSON
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	var actionable bool
	switch string(raw.Actionable) {
	case "true", `"true"`:
		actionable = true
	case "false", `"false"`:
	default:
		return fmt.Errorf("actionable must be a JSON boolean")
	}
	*r = HypothesisRefinement{
		Label:      raw.Label,
		Score:      raw.Score,
		Why:        raw.Why,
		Actionable: actionable,
	}
	return nil
}

type ToolError struct {
	ToolName string `json:"tool_name"`
	Action   string `json:"action"`
	Error    string `json:"error"`
}

type EvidenceRelation string

const (
	EvidenceRelationSupport EvidenceRelation = "support"
	EvidenceRelationRefute  EvidenceRelation = "refute"
	EvidenceRelationNeutral EvidenceRelation = "neutral"
)

type EvidenceItem struct {
	SourceType         string           `json:"source_type"`
	SourceID           string           `json:"source_id"`
	SignalType         string           `json:"signal_type,omitempty"`
	Entity             string           `json:"entity,omitempty"`
	Title              string           `json:"title"`
	Snippet            string           `json:"snippet"`
	Score              float64          `json:"score"`
	Relation           EvidenceRelation `json:"relation"`
	TargetHypothesisID string           `json:"target_hypothesis_id"`
	Strength           float64          `json:"strength"`
	ToolName           string           `json:"tool_name,omitempty"`
	ArtifactRef        string           `json:"artifact_ref,omitempty"`
	ObservationTime    time.Time        `json:"observation_time,omitempty"`
}

type ToolRegistry struct {
	tools map[string]tool.InvokableTool
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]tool.InvokableTool),
	}
}

func (r *ToolRegistry) Register(name string, t tool.InvokableTool) {
	r.tools[name] = t
}

func (r *ToolRegistry) Get(name string) (tool.InvokableTool, bool) {
	t, ok := r.tools[name]
	return t, ok
}
