package gos_engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/promptreg"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/utility/common"
	utilitymetrics "SuperBizAgent/utility/metrics"
)

const gosRuntimeVersion = "gos-runtime-v1"

type RuntimeVersions struct {
	Runtime   string `json:"runtime"`
	Prompt    string `json:"prompt"`
	ModelPath string `json:"model_path"`
	Model     string `json:"model"`
	Config    string `json:"config"`
}

func newRunStats() *RunStats {
	return &RunStats{PhaseLatencyMs: make(map[string]int64)}
}

func (s *RunStats) addPhaseLatency(phase string, elapsed time.Duration) {
	if s == nil {
		return
	}
	if s.PhaseLatencyMs == nil {
		s.PhaseLatencyMs = make(map[string]int64)
	}
	s.PhaseLatencyMs[phase] += elapsed.Milliseconds()
}

func (s *RunStats) observeProgress(before, after graphProgress) {
	if s == nil {
		return
	}
	if after.frontierID != "" && before.frontierID != "" && after.frontierID != before.frontierID {
		s.FrontierChanges++
	}
	if added := after.activeEvidence - before.activeEvidence; added > 0 {
		s.NewEvidenceCount += added
	}
	s.ConfidenceDelta += after.frontierScore - before.frontierScore
}

func (s *RunStats) observeGraph(graph *belief.BeliefGraph) {
	if s == nil || graph == nil {
		return
	}
	current := graph.ResourceStats()
	if current.Nodes > s.Graph.Nodes {
		s.Graph.Nodes = current.Nodes
	}
	if current.Edges > s.Graph.Edges {
		s.Graph.Edges = current.Edges
	}
	if current.Depth > s.Graph.Depth {
		s.Graph.Depth = current.Depth
	}
	if current.Snapshots > s.Graph.Snapshots {
		s.Graph.Snapshots = current.Snapshots
	}
	if current.Deltas > s.Graph.Deltas {
		s.Graph.Deltas = current.Deltas
	}
	if current.HistoryBytes > s.Graph.HistoryBytes {
		s.Graph.HistoryBytes = current.HistoryBytes
	}
}

func (e *GoSEngine) finalizeResult(result *protocol.TaskResult, graph *belief.BeliefGraph, startedAt time.Time, stats *RunStats) *protocol.TaskResult {
	if result == nil {
		return nil
	}
	if stats == nil {
		stats = newRunStats()
	}
	stats.observeGraph(graph)
	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}
	if result.StartedAt == 0 {
		result.StartedAt = startedAt.UnixMilli()
	}
	if result.FinishedAt == 0 {
		result.FinishedAt = time.Now().UnixMilli()
	}
	duration := time.Duration(result.FinishedAt-result.StartedAt) * time.Millisecond
	versions := e.runtimeVersions()
	observability := map[string]any{
		"duration_ms":        duration.Milliseconds(),
		"phase_latency_ms":   copyInt64Map(stats.PhaseLatencyMs),
		"frontier_changes":   stats.FrontierChanges,
		"backtrack_count":    stats.BacktrackCount,
		"new_evidence_count": stats.NewEvidenceCount,
		"confidence_delta":   stats.ConfidenceDelta,
		"evidence_bootstrap": map[string]any{
			"status":         stats.BootstrapStatus,
			"reason":         stats.BootstrapReason,
			"evidence_count": stats.BootstrapEvidence,
		},
		"graph": stats.Graph,
		"calls": map[string]int{
			"llm":  stats.LLMCalls,
			"tool": stats.ToolCalls,
			"rag":  stats.RAGCalls,
		},
	}
	result.Metadata["observability"] = observability
	result.Metadata["versions"] = versions
	result.Metadata["storage_boundary"] = map[string]string{
		"graph":          "task_result.metadata.belief_graph",
		"trace":          "runtime.ledger.events",
		"result":         "runtime.ledger.result",
		"multi_instance": "redis_ledger_required",
		"artifact":       "external_shared_storage_required",
	}
	result.Metadata["phase_latency_ms"] = copyInt64Map(stats.PhaseLatencyMs)
	result.Metadata["frontier_changes"] = stats.FrontierChanges
	result.Metadata["backtrack_count"] = stats.BacktrackCount
	result.Metadata["new_evidence_count"] = stats.NewEvidenceCount
	result.Metadata["confidence_delta"] = stats.ConfidenceDelta
	result.Metadata["evidence_bootstrap"] = map[string]any{
		"status":         stats.BootstrapStatus,
		"reason":         stats.BootstrapReason,
		"evidence_count": stats.BootstrapEvidence,
	}
	result.Metadata["graph_resource_stats"] = stats.Graph
	e.emit(context.Background(), "observability", "GoS 运行指标", map[string]any{
		"status":             result.Status,
		"phase_latency_ms":   copyInt64Map(stats.PhaseLatencyMs),
		"frontier_changes":   stats.FrontierChanges,
		"backtrack_count":    stats.BacktrackCount,
		"new_evidence_count": stats.NewEvidenceCount,
		"confidence_delta":   stats.ConfidenceDelta,
		"graph":              stats.Graph,
		"versions":           versions,
	})

	utilitymetrics.ObserveGoSRun(
		string(result.Status),
		duration,
		stats.PhaseLatencyMs,
		map[string]int{"llm": stats.LLMCalls, "tool": stats.ToolCalls, "rag": stats.RAGCalls},
		map[string]int{
			"nodes": stats.Graph.Nodes, "edges": stats.Graph.Edges, "depth": stats.Graph.Depth,
			"snapshots": stats.Graph.Snapshots, "deltas": stats.Graph.Deltas, "history_bytes": stats.Graph.HistoryBytes,
		},
		map[string]int{
			"frontier_changes": stats.FrontierChanges,
			"backtracks":       stats.BacktrackCount,
			"new_evidence":     stats.NewEvidenceCount,
		},
	)
	return result
}

func (e *GoSEngine) runtimeVersions() RuntimeVersions {
	modelPath := ""
	if e != nil && e.cfg != nil {
		modelPath = e.cfg.ModelPath
	}
	modelVersion := modelPath
	if resolved, err := common.LoadChatModelConfig(context.Background(), modelPath); err == nil {
		modelVersion = resolved.Provider + ":" + resolved.Model
	}
	return RuntimeVersions{
		Runtime: gosRuntimeVersion,
		Prompt: hashVersion(
			promptreg.GOSIngest,
			promptreg.GOSPlanner,
			promptreg.GOSExpertSystem,
			promptreg.GOSExpertToolCall,
			promptreg.GOSExpertRetrieve,
			promptreg.GOSExpertAnalyze,
		),
		ModelPath: modelPath,
		Model:     modelVersion,
		Config:    e.configVersion(),
	}
}

func (e *GoSEngine) configVersion() string {
	if e == nil || e.cfg == nil {
		return hashVersion("nil")
	}
	payload := struct {
		Enabled             bool
		ModelPath           string
		Temperature         float64
		MaxTokens           int
		EvidenceMaxChars    int
		SessionMaxSteps     int
		MaxRetrievalSteps   int
		CallTimeoutMs       int
		FSM                 FSMConfig
		Confidence          ConfidenceConfig
		Graph               GraphConfig
		StateConversion     StateConversionConfig
		EvidenceBootstrap   EvidenceBootstrapConfig
		StructuredCognition StructuredCognitionConfig
		Execution           ExecutionConfig
		Report              ReportConfig
		Experts             []ExpertConfig
		HeadAgent           string
	}{
		Enabled:             e.cfg.Enabled,
		ModelPath:           e.cfg.ModelPath,
		Temperature:         e.cfg.Temperature,
		MaxTokens:           e.cfg.MaxTokens,
		EvidenceMaxChars:    e.cfg.EvidenceMaxChars,
		SessionMaxSteps:     e.cfg.SessionMaxSteps,
		MaxRetrievalSteps:   e.cfg.MaxRetrievalSteps,
		CallTimeoutMs:       e.cfg.CallTimeoutMs,
		FSM:                 e.cfg.FSM,
		Confidence:          e.cfg.Confidence,
		Graph:               e.cfg.Graph,
		StateConversion:     e.cfg.StateConversion,
		EvidenceBootstrap:   e.cfg.EvidenceBootstrap,
		StructuredCognition: e.cfg.StructuredCognition,
		Execution:           e.cfg.Execution,
		Report:              e.cfg.Report,
		Experts:             e.cfg.Experts,
		HeadAgent:           e.cfg.HeadAgent,
	}
	encoded, _ := json.Marshal(payload)
	return hashVersion(string(encoded))
}

func hashVersion(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))[:16]
}

func copyInt64Map(values map[string]int64) map[string]int64 {
	copy := make(map[string]int64, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}

func graphFailureReason(fallback string, err error) string {
	var limitErr *belief.GraphResourceLimitError
	if errors.As(err, &limitErr) {
		return "graph_resource_limit"
	}
	return fallback
}
