package belief

import (
	"fmt"
	"time"
)

type NodeType string

const (
	NodeSignal     NodeType = "Signal"
	NodeEvidence   NodeType = "Evidence"
	NodeHypothesis NodeType = "Hypothesis"
)

type EdgeType string

const (
	EdgeSupport EdgeType = "support"
	EdgeRefute  EdgeType = "refute"
	EdgeRefines EdgeType = "refines"
	EdgeCausal  EdgeType = "causal"
)

type NodeStatus string

const (
	StatusActive     NodeStatus = "active"
	StatusRetracted  NodeStatus = "retracted"
	StatusSuperseded NodeStatus = "superseded"
)

type EvidenceSource struct {
	SourceType         string    `json:"source_type"`
	SourceID           string    `json:"source_id"`
	ToolName           string    `json:"tool_name,omitempty"`
	RetrievalQuery     string    `json:"retrieval_query,omitempty"`
	Timestamp          time.Time `json:"timestamp"`
	SummarySnippet     string    `json:"summary_snippet"`
	ArtifactRef        string    `json:"artifact_ref,omitempty"`
	Relation           string    `json:"relation"`
	TargetHypothesisID string    `json:"target_hypothesis_id"`
	Strength           float64   `json:"strength"`
}

type Node struct {
	ID           string                 `json:"id"`
	Type         NodeType               `json:"type"`
	Label        string                 `json:"label"`
	Score        float64                `json:"score"`
	Status       NodeStatus             `json:"status"`
	Level        int                    `json:"level"`
	Attrs        map[string]interface{} `json:"attrs"`
	Source       *EvidenceSource        `json:"source,omitempty"`
	RetractedBy  string                 `json:"retracted_by,omitempty"`
	SupersededBy string                 `json:"superseded_by,omitempty"`
	RetractedAt  *time.Time             `json:"retracted_at,omitempty"`
	StepID       string                 `json:"step_id"`
}

type Edge struct {
	Src              string     `json:"src"`
	Dst              string     `json:"dst"`
	Type             EdgeType   `json:"type"`
	Status           NodeStatus `json:"status"`
	Confidence       float64    `json:"confidence"`
	DerivationType   string     `json:"derivation_type"`
	ExtractorVersion string     `json:"extractor_version"`
	StepID           string     `json:"step_id"`
	RetractedBy      string     `json:"retracted_by,omitempty"`
	RetractedAt      *time.Time `json:"retracted_at,omitempty"`
}

type Frontier struct {
	NodeID   string  `json:"node_id"`
	Label    string  `json:"label"`
	Why      string  `json:"why"`
	Score    float64 `json:"score"`
	Level    int     `json:"level"`
	Supports int     `json:"supports"`
	Refutes  int     `json:"refutes"`
}

type LevelNode struct {
	NodeID     string  `json:"node_id"`
	Confidence float64 `json:"confidence"`
}

type GraphSnapshot struct {
	StepID    string           `json:"step_id"`
	Timestamp time.Time        `json:"timestamp"`
	Action    string           `json:"action"`
	Nodes     map[string]*Node `json:"nodes"`
	Edges     map[string]*Edge `json:"edges"`
}

type GraphDelta struct {
	StepID      string           `json:"step_id"`
	Timestamp   time.Time        `json:"timestamp"`
	Action      string           `json:"action"`
	UpsertNodes map[string]*Node `json:"upsert_nodes,omitempty"`
	UpsertEdges map[string]*Edge `json:"upsert_edges,omitempty"`
}

type GraphPolicy struct {
	CheckpointInterval int `json:"checkpoint_interval"`
	MaxNodes           int `json:"max_nodes"`
	MaxEdges           int `json:"max_edges"`
	MaxDepth           int `json:"max_depth"`
	MaxSnapshots       int `json:"max_snapshots"`
	MaxDeltas          int `json:"max_deltas"`
}

type GraphResourceStats struct {
	Nodes        int `json:"nodes"`
	Edges        int `json:"edges"`
	Depth        int `json:"depth"`
	Snapshots    int `json:"snapshots"`
	Deltas       int `json:"deltas"`
	HistoryBytes int `json:"history_bytes"`
}

type GraphResourceLimitError struct {
	Resource string
	Limit    int
	Actual   int
}

func (e *GraphResourceLimitError) Error() string {
	return fmt.Sprintf("belief graph %s limit exceeded: limit=%d actual=%d", e.Resource, e.Limit, e.Actual)
}

type GraphUpdateResult struct {
	Committed bool  `json:"committed"`
	Error     error `json:"error,omitempty"`
}

type FSMState int

const (
	StateDrilling FSMState = iota
	StateReporting
	StateDone
)

type FSMDecision struct {
	Action string
	Reason string
}

type FSMTransition struct {
	From      FSMState `json:"from"`
	To        FSMState `json:"to"`
	FromLevel int      `json:"from_level"`
	ToLevel   int      `json:"to_level"`
	Reason    string   `json:"reason"`
	StepID    string   `json:"step_id"`
}

type FSMThresholds struct {
	GapDelta   float64 `yaml:"gap_delta"`
	MinSupport int     `yaml:"min_support"`
	MaxSteps   int     `yaml:"max_steps"`
}
