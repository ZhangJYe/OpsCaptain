package gos_engine

import (
	"context"

	"SuperBizAgent/internal/ai/belief"
)

type Ingestor struct {
	graph  *belief.BeliefGraph
	logger Logger
}

func NewIngestor(graph *belief.BeliefGraph, logger Logger) *Ingestor {
	return &Ingestor{
		graph:  graph,
		logger: logger,
	}
}

func (i *Ingestor) Ingest(ctx context.Context, symptom string) error {
	signalID := i.graph.AddSignal(symptom)
	i.graph.StartSignalID = signalID

	evidence := i.extractEvidence(symptom)
	for _, ev := range evidence {
		i.graph.AddEvidence(ev, nil)
	}

	i.generateL1Hypotheses(symptom, evidence)

	i.logger.Info("ingest completed",
		"signal_id", signalID,
		"evidence_count", len(evidence),
	)

	return nil
}

func (i *Ingestor) extractEvidence(symptom string) []string {
	var evidence []string

	if len(symptom) > 0 {
		evidence = append(evidence, symptom)
	}

	return evidence
}

func (i *Ingestor) generateL1Hypotheses(symptom string, evidence []string) {
	hypotheses := []struct {
		label  string
		score  float64
		reason string
	}{
		{
			label:  "资源耗尽",
			score:  0.6,
			reason: "高负载可能导致服务异常",
		},
		{
			label:  "网络问题",
			score:  0.3,
			reason: "网络延迟或丢包可能导致超时",
		},
		{
			label:  "配置错误",
			score:  0.1,
			reason: "配置变更可能导致服务异常",
		},
	}

	for _, h := range hypotheses {
		hypoID := i.graph.AddHypothesis(h.label, h.score, 1, h.reason)

		for _, evID := range i.graph.Nodes {
			if evID.Type == belief.NodeEvidence {
				i.graph.AddEdge(evID.ID, hypoID, belief.EdgeSupport, 0.5, "auto_generated")
			}
		}
	}
}
