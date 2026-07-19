package gos_engine

import (
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/belief"
)

type EventEmitter func(ctx context.Context, message string, payload map[string]any)
type StructuredGenerateFunc func(ctx context.Context, prompt string) (string, error)

type Config struct {
	Enabled             bool                      `yaml:"enabled"`
	ModelPath           string                    `yaml:"model_path"`
	Temperature         float64                   `yaml:"temperature"`
	MaxTokens           int                       `yaml:"max_tokens"`
	EvidenceMaxChars    int                       `yaml:"evidence_max_chars"`
	EvidenceMaxItems    int                       `yaml:"evidence_max_items"`
	SessionMaxSteps     int                       `yaml:"session_max_steps"`
	MaxRetrievalSteps   int                       `yaml:"max_retrieval_steps"`
	CallTimeoutMs       int                       `yaml:"call_timeout_ms"`
	FSM                 FSMConfig                 `yaml:"fsm"`
	Confidence          ConfidenceConfig          `yaml:"confidence"`
	Graph               GraphConfig               `yaml:"graph"`
	StateConversion     StateConversionConfig     `yaml:"state_conversion"`
	EvidenceBootstrap   EvidenceBootstrapConfig   `yaml:"evidence_bootstrap"`
	StructuredCognition StructuredCognitionConfig `yaml:"structured_cognition"`
	Execution           ExecutionConfig           `yaml:"execution"`
	Report              ReportConfig              `yaml:"report"`
	Experts             []ExpertConfig            `yaml:"experts"`
	HeadAgent           string                    `yaml:"head_agent"`
	Emit                EventEmitter              `yaml:"-"`
	StructuredGenerate  StructuredGenerateFunc    `yaml:"-"`
}

type FSMConfig struct {
	GapDelta      float64 `yaml:"gap_delta"`
	MinSupport    int     `yaml:"min_support"`
	MaxSteps      int     `yaml:"max_steps"`
	MinConfidence float64 `yaml:"min_confidence"`
}

type ConfidenceConfig struct {
	SupportWeight float64 `yaml:"support_weight"`
	RefuteWeight  float64 `yaml:"refute_weight"`
	Deduplicate   bool    `yaml:"deduplicate"`
}

type GraphConfig struct {
	CheckpointInterval int `yaml:"checkpoint_interval" json:"checkpoint_interval"`
	MaxNodes           int `yaml:"max_nodes" json:"max_nodes"`
	MaxEdges           int `yaml:"max_edges" json:"max_edges"`
	MaxDepth           int `yaml:"max_depth" json:"max_depth"`
	MaxSnapshots       int `yaml:"max_snapshots" json:"max_snapshots"`
	MaxDeltas          int `yaml:"max_deltas" json:"max_deltas"`
}

type StateConversionConfig struct {
	Enabled         bool             `yaml:"enabled"`
	MaxDepth        int              `yaml:"max_depth"`
	TieEpsilon      float64          `yaml:"tie_epsilon"`
	RefinementRules []RefinementRule `yaml:"refinement_rules"`
}

type EvidenceBootstrapConfig struct {
	Enabled                 bool             `yaml:"enabled" json:"enabled"`
	ExpertName              string           `yaml:"expert_name" json:"expert_name"`
	AllowedTools            []string         `yaml:"allowed_tools" json:"allowed_tools"`
	MaxEvidenceItems        int              `yaml:"max_evidence_items" json:"max_evidence_items"`
	EvidenceSnippetMaxChars int              `yaml:"evidence_snippet_max_chars" json:"evidence_snippet_max_chars"`
	Budget                  PlanBudgetConfig `yaml:"budget" json:"budget"`
}

type StructuredCognitionConfig struct {
	Enabled         bool             `yaml:"enabled"`
	CallTimeoutMs   int              `yaml:"call_timeout_ms"`
	MaxHypotheses   int              `yaml:"max_hypotheses"`
	MaxObservations int              `yaml:"max_observations"`
	MaxPlanItems    int              `yaml:"max_plan_items"`
	PlanBudget      PlanBudgetConfig `yaml:"plan_budget"`
}

type PlanBudgetConfig struct {
	LLMCalls          int `yaml:"llm_calls" json:"llm_calls"`
	ToolCalls         int `yaml:"tool_calls" json:"tool_calls"`
	RAGCalls          int `yaml:"rag_calls" json:"rag_calls"`
	TimeoutMs         int `yaml:"timeout_ms" json:"timeout_ms"`
	MaxRetrievalSteps int `yaml:"max_retrieval_steps" json:"max_retrieval_steps"`
	MaxOutputTokens   int `yaml:"max_output_tokens" json:"max_output_tokens"`
}

type ExecutionConfig struct {
	MaxConcurrentExperts int `yaml:"max_concurrent_experts"`
	NoProgressRoundLimit int `yaml:"no_progress_round_limit"`
}

type ReportConfig struct {
	ConflictStrengthThreshold float64 `yaml:"conflict_strength_threshold"`
	MaxEvidenceItems          int     `yaml:"max_evidence_items"`
	EvidenceSnippetMaxChars   int     `yaml:"evidence_snippet_max_chars"`
	MaxNextActions            int     `yaml:"max_next_actions"`
}

type RefinementRule struct {
	Parent   string                `yaml:"parent"`
	Children []RefinementCandidate `yaml:"children"`
}

type RefinementCandidate struct {
	Label      string  `yaml:"label"`
	Score      float64 `yaml:"score"`
	Why        string  `yaml:"why"`
	Actionable bool    `yaml:"actionable"`
}

type ExpertConfig struct {
	Name              string           `yaml:"name"`
	Description       string           `yaml:"description"`
	Tools             []string         `yaml:"tools"`
	MaxRetrievalSteps int              `yaml:"max_retrieval_steps"`
	Budget            PlanBudgetConfig `yaml:"budget"`
}

func DefaultConfig() *Config {
	return &Config{
		Enabled:           false,
		ModelPath:         "chat_model_fast",
		Temperature:       0.8,
		MaxTokens:         4096,
		EvidenceMaxChars:  8192,
		EvidenceMaxItems:  12,
		SessionMaxSteps:   5,
		MaxRetrievalSteps: 3,
		CallTimeoutMs:     30000,
		FSM: FSMConfig{
			GapDelta:      0.3,
			MinSupport:    2,
			MaxSteps:      3,
			MinConfidence: 0.7,
		},
		Confidence: ConfidenceConfig{
			SupportWeight: 0.25,
			RefuteWeight:  0.35,
			Deduplicate:   true,
		},
		Graph: GraphConfig{
			CheckpointInterval: 10,
			MaxNodes:           256,
			MaxEdges:           512,
			MaxDepth:           4,
			MaxSnapshots:       32,
			MaxDeltas:          10,
		},
		StateConversion: StateConversionConfig{
			Enabled:    false,
			MaxDepth:   2,
			TieEpsilon: 0.01,
			RefinementRules: []RefinementRule{
				{
					Parent: "资源耗尽",
					Children: []RefinementCandidate{
						{Label: "CPU 饱和", Score: 0.6, Why: "验证 CPU 使用率、负载和节流状态", Actionable: true},
						{Label: "内存压力", Score: 0.3, Why: "验证内存使用、交换和 OOM 事件", Actionable: true},
						{Label: "磁盘 IO 饱和", Score: 0.1, Why: "验证 IO 等待、吞吐和磁盘容量", Actionable: true},
					},
				},
				{
					Parent: "网络问题",
					Children: []RefinementCandidate{
						{Label: "网络延迟升高", Score: 0.5, Why: "验证链路延迟和跨区 RTT", Actionable: true},
						{Label: "网络丢包", Score: 0.3, Why: "验证重传、丢包率和接口错误", Actionable: true},
						{Label: "DNS 解析异常", Score: 0.2, Why: "验证 DNS 延迟、失败率和解析结果", Actionable: true},
					},
				},
				{
					Parent: "配置错误",
					Children: []RefinementCandidate{
						{Label: "应用配置不一致", Score: 0.5, Why: "核对当前配置与最近变更", Actionable: true},
						{Label: "依赖地址配置错误", Score: 0.3, Why: "核对依赖端点、凭据引用和环境变量", Actionable: true},
						{Label: "发布参数错误", Score: 0.2, Why: "核对发布清单和运行参数", Actionable: true},
					},
				},
			},
		},
		EvidenceBootstrap: EvidenceBootstrapConfig{
			Enabled:                 false,
			ExpertName:              "linux_sre",
			AllowedTools:            []string{"query_logs", "query_prometheus_instant", "query_prometheus_alerts", "query_internal_docs"},
			MaxEvidenceItems:        2,
			EvidenceSnippetMaxChars: 2048,
			Budget: PlanBudgetConfig{
				LLMCalls:          2,
				ToolCalls:         1,
				RAGCalls:          1,
				TimeoutMs:         10000,
				MaxRetrievalSteps: 1,
				MaxOutputTokens:   512,
			},
		},
		StructuredCognition: StructuredCognitionConfig{
			Enabled:         false,
			CallTimeoutMs:   30000,
			MaxHypotheses:   4,
			MaxObservations: 8,
			MaxPlanItems:    2,
			PlanBudget: PlanBudgetConfig{
				LLMCalls:          3,
				ToolCalls:         2,
				RAGCalls:          1,
				TimeoutMs:         30000,
				MaxRetrievalSteps: 2,
				MaxOutputTokens:   1024,
			},
		},
		Execution: ExecutionConfig{
			MaxConcurrentExperts: 3,
			NoProgressRoundLimit: 2,
		},
		Report: ReportConfig{
			ConflictStrengthThreshold: 0.5,
			MaxEvidenceItems:          8,
			EvidenceSnippetMaxChars:   512,
			MaxNextActions:            3,
		},
		Experts: []ExpertConfig{
			{
				Name:              "linux_sre",
				Description:       "Linux SRE 专家",
				Tools:             []string{"query_logs", "query_prometheus_instant", "query_prometheus_alerts", "query_internal_docs"},
				MaxRetrievalSteps: 3,
				Budget:            PlanBudgetConfig{LLMCalls: 3, ToolCalls: 2, RAGCalls: 1, TimeoutMs: 30000, MaxRetrievalSteps: 2, MaxOutputTokens: 1024},
			},
			{
				Name:              "network_sre",
				Description:       "网络 SRE 专家",
				Tools:             []string{"query_logs", "query_prometheus_instant", "query_prometheus_alerts", "query_internal_docs"},
				MaxRetrievalSteps: 3,
				Budget:            PlanBudgetConfig{LLMCalls: 3, ToolCalls: 2, RAGCalls: 1, TimeoutMs: 30000, MaxRetrievalSteps: 2, MaxOutputTokens: 1024},
			},
			{
				Name:              "database_sre",
				Description:       "数据库 SRE 专家",
				Tools:             []string{"query_logs", "query_prometheus_instant", "query_prometheus_alerts", "query_internal_docs"},
				MaxRetrievalSteps: 3,
				Budget:            PlanBudgetConfig{LLMCalls: 3, ToolCalls: 2, RAGCalls: 1, TimeoutMs: 30000, MaxRetrievalSteps: 2, MaxOutputTokens: 1024},
			},
		},
		HeadAgent: "sre_commander",
	}
}

func (c *Config) ToFSMThresholds() belief.FSMThresholds {
	return belief.FSMThresholds{
		GapDelta:   c.FSM.GapDelta,
		MinSupport: c.FSM.MinSupport,
		MaxSteps:   c.FSM.MaxSteps,
	}
}

func (c *Config) ToGraphPolicy() belief.GraphPolicy {
	return belief.GraphPolicy{
		CheckpointInterval: c.Graph.CheckpointInterval,
		MaxNodes:           c.Graph.MaxNodes,
		MaxEdges:           c.Graph.MaxEdges,
		MaxDepth:           c.Graph.MaxDepth,
		MaxSnapshots:       c.Graph.MaxSnapshots,
		MaxDeltas:          c.Graph.MaxDeltas,
	}
}

func (c *Config) ValidateGraphConfig() error {
	if c == nil {
		return fmt.Errorf("GoS config is required")
	}
	values := []struct {
		name  string
		value int
	}{
		{name: "checkpoint_interval", value: c.Graph.CheckpointInterval},
		{name: "max_nodes", value: c.Graph.MaxNodes},
		{name: "max_edges", value: c.Graph.MaxEdges},
		{name: "max_depth", value: c.Graph.MaxDepth},
		{name: "max_snapshots", value: c.Graph.MaxSnapshots},
		{name: "max_deltas", value: c.Graph.MaxDeltas},
	}
	for _, item := range values {
		if item.value <= 0 {
			return fmt.Errorf("aiops.gos.graph.%s must be positive", item.name)
		}
	}
	if c.StateConversion.Enabled && c.Graph.MaxDepth < c.StateConversion.MaxDepth {
		return fmt.Errorf("aiops.gos.graph.max_depth must be >= state_conversion.max_depth")
	}
	if c.EvidenceBootstrap.Enabled {
		if !c.StructuredCognition.Enabled {
			return fmt.Errorf("aiops.gos.structured_cognition.enabled must be true when evidence_bootstrap is enabled")
		}
		if strings.TrimSpace(c.EvidenceBootstrap.ExpertName) == "" {
			return fmt.Errorf("aiops.gos.evidence_bootstrap.expert_name is required")
		}
		if c.EvidenceBootstrap.MaxEvidenceItems <= 0 || c.EvidenceBootstrap.EvidenceSnippetMaxChars <= 0 {
			return fmt.Errorf("aiops.gos.evidence_bootstrap evidence limits must be positive")
		}
		budget := c.EvidenceBootstrap.Budget
		if budget.LLMCalls <= 0 || budget.ToolCalls <= 0 || budget.RAGCalls <= 0 || budget.TimeoutMs <= 0 || budget.MaxRetrievalSteps <= 0 || budget.MaxOutputTokens <= 0 {
			return fmt.Errorf("aiops.gos.evidence_bootstrap budget values must be positive")
		}
		for _, toolName := range c.EvidenceBootstrap.AllowedTools {
			if !isReadOnlyBootstrapTool(toolName) {
				return fmt.Errorf("aiops.gos.evidence_bootstrap tool %q is not read-only", toolName)
			}
		}
	}
	return nil
}
