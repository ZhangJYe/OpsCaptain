package gos_engine

import (
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/promptreg"
)

type Ingestor struct {
	graph    *belief.BeliefGraph
	logger   Logger
	cfg      *Config
	generate StructuredGenerateFunc
}

type IngestProposal struct {
	Signal       string                `json:"signal"`
	Observations []ObservationProposal `json:"observations"`
	Hypotheses   []HypothesisProposal  `json:"hypotheses"`
}

type ObservationProposal struct {
	Label      string `json:"label"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}

type HypothesisProposal struct {
	Label      string  `json:"label"`
	Score      float64 `json:"score"`
	Why        string  `json:"why"`
	Actionable *bool   `json:"actionable"`
}

type IngestOutcome struct {
	Mode             string
	FallbackReason   string
	LLMCalls         int
	ObservationCount int
	HypothesisCount  int
}

func NewIngestor(graph *belief.BeliefGraph, logger Logger) *Ingestor {
	cfg := DefaultConfig()
	return &Ingestor{
		graph:  graph,
		logger: logger,
		cfg:    cfg,
	}
}

func NewStructuredIngestor(graph *belief.BeliefGraph, cfg *Config, logger Logger, generate StructuredGenerateFunc) *Ingestor {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Ingestor{graph: graph, cfg: cfg, logger: logger, generate: generate}
}

func (i *Ingestor) Ingest(ctx context.Context, symptom string) error {
	_, err := i.IngestWithOutcome(ctx, symptom)
	return err
}

func (i *Ingestor) IngestWithOutcome(ctx context.Context, symptom string) (IngestOutcome, error) {
	if err := ctx.Err(); err != nil {
		return IngestOutcome{}, err
	}
	if i.graph == nil {
		return IngestOutcome{}, fmt.Errorf("belief graph is required")
	}
	if strings.TrimSpace(symptom) == "" {
		return IngestOutcome{}, fmt.Errorf("symptom is required")
	}

	proposal := fallbackIngestProposal(symptom)
	outcome := IngestOutcome{Mode: "rules"}
	if i.cfg.StructuredCognition.Enabled {
		outcome.Mode = "rule_fallback"
		if i.generate == nil {
			outcome.FallbackReason = "structured_generator_unavailable"
		} else {
			outcome.LLMCalls = 1
			raw, err := i.generate(ctx, renderStructuredPrompt(promptreg.GOSIngest, map[string]string{"symptom": symptom}))
			if ctx.Err() != nil {
				return outcome, ctx.Err()
			}
			if err != nil {
				outcome.FallbackReason = "structured_generation_failed"
			} else {
				var structured IngestProposal
				if err := decodeStrictJSONObject(raw, &structured); err != nil {
					outcome.FallbackReason = "structured_schema_invalid"
				} else if err := validateIngestProposal(structured, i.cfg.StructuredCognition); err != nil {
					outcome.FallbackReason = "structured_contract_invalid"
				} else {
					proposal = structured
					outcome.Mode = "structured"
					outcome.FallbackReason = ""
				}
			}
		}
	}

	result := i.graph.UpdateCopyOnWrite(func(graph *belief.BeliefGraph) error {
		return applyIngestProposal(graph, proposal, outcome)
	})
	if !result.Committed {
		return outcome, fmt.Errorf("apply ingest proposal: %w", result.Error)
	}
	outcome.ObservationCount = len(proposal.Observations)
	outcome.HypothesisCount = len(proposal.Hypotheses)
	if i.logger != nil {
		i.logger.Info("ingest completed",
			"signal_id", i.graph.StartSignalID,
			"observation_count", outcome.ObservationCount,
			"hypothesis_count", outcome.HypothesisCount,
			"mode", outcome.Mode,
			"fallback_reason", outcome.FallbackReason,
		)
	}
	return outcome, nil
}

func validateIngestProposal(proposal IngestProposal, cfg StructuredCognitionConfig) error {
	if strings.TrimSpace(proposal.Signal) == "" {
		return fmt.Errorf("signal is required")
	}
	if cfg.MaxObservations <= 0 || cfg.MaxHypotheses <= 0 {
		return fmt.Errorf("structured cognition ingest limits must be positive")
	}
	if len(proposal.Observations) > cfg.MaxObservations {
		return fmt.Errorf("observations exceed configured limit")
	}
	if len(proposal.Hypotheses) == 0 || len(proposal.Hypotheses) > cfg.MaxHypotheses {
		return fmt.Errorf("hypotheses must contain 1..%d items", cfg.MaxHypotheses)
	}
	observationLabels := make(map[string]struct{}, len(proposal.Observations))
	for _, observation := range proposal.Observations {
		label := strings.TrimSpace(observation.Label)
		if label == "" || strings.TrimSpace(observation.SourceType) == "" || strings.TrimSpace(observation.SourceID) == "" {
			return fmt.Errorf("observation label and source provenance are required")
		}
		switch observation.SourceType {
		case "user_input", "alert", "change", "telemetry":
		default:
			return fmt.Errorf("unsupported observation source_type %q", observation.SourceType)
		}
		key := strings.ToLower(label)
		if _, exists := observationLabels[key]; exists {
			return fmt.Errorf("duplicate observation %q", label)
		}
		observationLabels[key] = struct{}{}
	}
	hypothesisLabels := make(map[string]struct{}, len(proposal.Hypotheses))
	for _, hypothesis := range proposal.Hypotheses {
		label := strings.TrimSpace(hypothesis.Label)
		if label == "" || strings.TrimSpace(hypothesis.Why) == "" || hypothesis.Actionable == nil {
			return fmt.Errorf("hypothesis label, why and actionable are required")
		}
		if hypothesis.Score <= 0 || hypothesis.Score > 1 {
			return fmt.Errorf("hypothesis score must be within (0,1]")
		}
		key := strings.ToLower(label)
		if _, exists := hypothesisLabels[key]; exists {
			return fmt.Errorf("duplicate hypothesis %q", label)
		}
		hypothesisLabels[key] = struct{}{}
	}
	return nil
}

func applyIngestProposal(graph *belief.BeliefGraph, proposal IngestProposal, outcome IngestOutcome) error {
	if len(graph.Nodes) != 0 || len(graph.Edges) != 0 || graph.StartSignalID != "" {
		return fmt.Errorf("ingest requires an empty graph")
	}
	signalAttrs := map[string]interface{}{
		"semantic_type": "signal",
		"ingest_mode":   outcome.Mode,
	}
	if outcome.FallbackReason != "" {
		signalAttrs["fallback_reason"] = outcome.FallbackReason
	}
	signalID := graph.AddNodeCopy(belief.NodeSignal, strings.TrimSpace(proposal.Signal), 1, 0, signalAttrs, nil)
	graph.StartSignalID = signalID

	observationIDs := make([]string, 0, len(proposal.Observations))
	for _, observation := range proposal.Observations {
		observationID := graph.AddNodeCopy(belief.NodeSignal, strings.TrimSpace(observation.Label), 1, 0, map[string]interface{}{
			"semantic_type": "observation",
			"source_type":   strings.TrimSpace(observation.SourceType),
			"source_id":     strings.TrimSpace(observation.SourceID),
		}, nil)
		graph.AddEdgeCopy(signalID, observationID, belief.EdgeCausal, 1, "structured_ingest_v1")
		observationIDs = append(observationIDs, observationID)
	}

	for _, hypothesis := range proposal.Hypotheses {
		hypothesisID := graph.AddNodeCopy(belief.NodeHypothesis, strings.TrimSpace(hypothesis.Label), hypothesis.Score, 1, map[string]interface{}{
			"why":        strings.TrimSpace(hypothesis.Why),
			"actionable": *hypothesis.Actionable,
		}, nil)
		parents := observationIDs
		if len(parents) == 0 {
			parents = []string{signalID}
		}
		for _, parentID := range parents {
			graph.AddEdgeCopy(parentID, hypothesisID, belief.EdgeRefines, hypothesis.Score, "structured_ingest_v1")
		}
	}
	return validateBeliefGraph(graph)
}

func fallbackIngestProposal(symptom string) IngestProposal {
	actionable := false
	return IngestProposal{
		Signal: symptom,
		Hypotheses: []HypothesisProposal{
			{Label: "资源耗尽", Score: 0.6, Why: "高负载可能导致服务异常", Actionable: &actionable},
			{Label: "网络问题", Score: 0.3, Why: "网络延迟或丢包可能导致超时", Actionable: &actionable},
			{Label: "配置错误", Score: 0.1, Why: "配置变更可能导致服务异常", Actionable: &actionable},
		},
	}
}
