package gos_engine

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
)

type backtrackingEventExpert struct {
	calls int
}

func (e *backtrackingEventExpert) Name() string {
	return "linux_sre"
}

func (e *backtrackingEventExpert) Run(_ context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *experts.ExpertAnalysis {
	e.calls++
	targetID := frontier.NodeID
	relation := experts.EvidenceRelationSupport
	if e.calls == 2 {
		parents := graph.GetActiveRefinesParentCopies(frontier.NodeID)
		if len(parents) > 0 {
			targetID = parents[0].ID
			relation = experts.EvidenceRelationRefute
		}
	}
	return &experts.ExpertAnalysis{
		ExpertName: "linux_sre",
		Status:     "succeeded",
		Confidence: 0.9,
		Evidence: []experts.EvidenceItem{{
			SourceType: "metric", SourceID: fmt.Sprintf("event-evidence-%d", e.calls), Title: "state conversion evidence",
			Relation: relation, TargetHypothesisID: targetID, Strength: 1,
		}},
	}
}

func TestGenerateReportUsesGraphConfidenceAndSourceMappedEvidence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FSM.MinConfidence = 0.5
	cfg.FSM.MinSupport = 2
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("CPU 饱和", 0.7, 1, "核验 CPU 指标")
	frontier := &belief.Frontier{NodeID: hypothesisID, Label: "CPU 饱和", Level: 1}
	update := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "linux_sre", Analysis: "expert confidence must not replace graph confidence", Confidence: 0.99,
		Evidence: []experts.EvidenceItem{
			{SourceType: "metric", SourceID: "cpu-usage", Title: "CPU 使用率 96%", Snippet: "cpu=0.96", Relation: experts.EvidenceRelationSupport, TargetHypothesisID: hypothesisID, Strength: 0.9},
			{SourceType: "log", SourceID: "throttle-log", Title: "CPU throttle", Snippet: "throttled", Relation: experts.EvidenceRelationSupport, TargetHypothesisID: hypothesisID, Strength: 0.8},
		},
	}}, frontier)
	require.True(t, update.Committed, update.Error)
	fsm := belief.NewBeliefFSM(cfg.ToFSMThresholds())

	result := engine.generateReport(context.Background(), graph, fsm, time.Now(), &RunStats{})

	require.Equal(t, protocol.ResultStatusSucceeded, result.Status)
	report, ok := result.Metadata["evidence_report"].(EvidenceReport)
	require.True(t, ok)
	assert.Equal(t, hypothesisID, report.Conclusion.HypothesisID)
	assert.Equal(t, graph.Nodes[hypothesisID].Score, report.Confidence)
	assert.NotEqual(t, 0.99, report.Confidence)
	require.Len(t, report.SupportingEvidence, 2)
	for _, evidence := range report.SupportingEvidence {
		assert.NotEmpty(t, evidence.NodeID)
		assert.NotEmpty(t, evidence.EdgeRef)
		assert.NotEmpty(t, evidence.SourceType)
		assert.NotEmpty(t, evidence.SourceID)
		assert.Equal(t, hypothesisID, evidence.TargetHypothesisID)
	}
	assert.Contains(t, result.Summary, "## 支持证据")
	assert.Contains(t, result.Summary, "metric:cpu-usage")
	assert.Contains(t, result.Summary, "expert confidence must not replace graph confidence")
	assert.Contains(t, result.Summary, "cpu=0.96")
	assert.NotEmpty(t, result.NextActions)
}

func TestCompactReportTextNormalizesAndLimitsUnicode(t *testing.T) {
	assert.Equal(t, "HTTP 503 without backoff", compactReportText(" HTTP 503\nwithout   backoff ", 64))
	assert.Equal(t, "连接池等…", compactReportText("连接池等待超时", 4))
}

func TestGenerateReportTruncatesDisplayOnlyAndPreservesProtocolEvidence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FSM.MinConfidence = 0.1
	cfg.FSM.MinSupport = 1
	cfg.Report.MaxEvidenceItems = 1
	cfg.Report.EvidenceSnippetMaxChars = 12
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("缓存耗尽", 0.8, 1, "核验缓存指标")
	frontier := &belief.Frontier{NodeID: hypothesisID, Label: "缓存耗尽", Level: 1}
	observedAt := time.Date(2025, 6, 18, 8, 12, 0, 0, time.UTC)
	fullSnippet := "redis connected clients reached the configured maximum and rejected new connections"
	update := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "linux_sre",
		Evidence: []experts.EvidenceItem{
			{
				SourceType: "metric", SignalType: "metric", SourceID: "redis-connections", Entity: "redis-cart",
				Title: "Redis 连接数达到上限", Snippet: fullSnippet, ArtifactRef: "recorded://case-a",
				ObservationTime: observedAt, Relation: experts.EvidenceRelationSupport,
				TargetHypothesisID: hypothesisID, Strength: 0.9,
			},
			{
				SourceType: "log", SignalType: "log", SourceID: "redis-rejection", Entity: "redis-cart",
				Title: "Redis 拒绝连接", Snippet: "maximum number of clients reached", ArtifactRef: "recorded://case-a",
				ObservationTime: observedAt, Relation: experts.EvidenceRelationSupport,
				TargetHypothesisID: hypothesisID, Strength: 0.8,
			},
		},
	}}, frontier)
	require.True(t, update.Committed, update.Error)

	result := engine.generateReport(context.Background(), graph, belief.NewBeliefFSM(cfg.ToFSMThresholds()), time.Now(), &RunStats{})

	assert.NotContains(t, result.Summary, fullSnippet)
	assert.Contains(t, result.Summary, "redis connec…")
	require.Len(t, result.Evidence, 2)
	assert.Equal(t, fullSnippet, result.Evidence[0].Snippet)
	assert.Equal(t, "metric", result.Evidence[0].SignalType)
	assert.Equal(t, "redis-cart", result.Evidence[0].Entity)
	assert.Equal(t, "recorded://case-a", result.Evidence[0].URI)
	assert.Equal(t, observedAt, result.Evidence[0].ObservationTime)
	assert.Equal(t, "maximum number of clients reached", result.Evidence[1].Snippet)
	assert.NotContains(t, result.Summary, "Redis 拒绝连接")
	report := result.Metadata["evidence_report"].(EvidenceReport)
	require.Len(t, report.SupportingEvidence, 1)
	assert.Equal(t, fullSnippet, report.SupportingEvidence[0].Snippet)
	assert.Equal(t, "metric", report.SupportingEvidence[0].SignalType)
	assert.Equal(t, "redis-cart", report.SupportingEvidence[0].Entity)
	assert.Equal(t, observedAt, report.SupportingEvidence[0].ObservationTime)
}

func TestGenerateReportDegradesOnCriticalRefutingEvidence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FSM.MinConfidence = 0.1
	cfg.FSM.MinSupport = 1
	cfg.Report.ConflictStrengthThreshold = 0.5
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("网络丢包", 0.8, 1, "核验丢包")
	frontier := &belief.Frontier{NodeID: hypothesisID, Label: "网络丢包", Level: 1}
	update := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "network_sre", Confidence: 0.9,
		Evidence: []experts.EvidenceItem{
			{SourceType: "metric", SourceID: "packet-loss", Title: "丢包率升高", Relation: experts.EvidenceRelationSupport, TargetHypothesisID: hypothesisID, Strength: 0.8},
			{SourceType: "trace", SourceID: "healthy-path", Title: "关键路径无丢包", Relation: experts.EvidenceRelationRefute, TargetHypothesisID: hypothesisID, Strength: 0.7},
		},
	}}, frontier)
	require.True(t, update.Committed, update.Error)

	result := engine.generateReport(context.Background(), graph, belief.NewBeliefFSM(cfg.ToFSMThresholds()), time.Now(), &RunStats{})

	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Contains(t, result.DegradationReason, "critical_evidence_conflict")
	report := result.Metadata["evidence_report"].(EvidenceReport)
	require.Len(t, report.Conflicts, 1)
	assert.Equal(t, hypothesisID, report.Conflicts[0].TargetHypothesisID)
	assert.Contains(t, result.Summary, "反驳与冲突证据")
	assert.Contains(t, result.Summary, "healthy-path")
}

func TestGenerateReportExcludesRetractedEvidence(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FSM.MinConfidence = 0.1
	cfg.FSM.MinSupport = 1
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	hypothesisID := graph.AddHypothesis("配置错误", 0.8, 1, "核验配置")
	frontier := &belief.Frontier{NodeID: hypothesisID, Label: "配置错误", Level: 1}
	update := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "linux_sre", Confidence: 0.8,
		Evidence: []experts.EvidenceItem{{
			SourceType: "change", SourceID: "deploy-1", Title: "错误配置发布", Relation: experts.EvidenceRelationSupport, TargetHypothesisID: hypothesisID, Strength: 0.9,
		}},
	}}, frontier)
	require.True(t, update.Committed, update.Error)
	for _, node := range graph.GetActiveNodeCopies() {
		if node.Type == belief.NodeEvidence {
			graph.RetractNode(node.ID, "test", "source invalidated")
		}
	}

	result := engine.generateReport(context.Background(), graph, belief.NewBeliefFSM(cfg.ToFSMThresholds()), time.Now(), &RunStats{})

	assert.Equal(t, protocol.ResultStatusDegraded, result.Status)
	report := result.Metadata["evidence_report"].(EvidenceReport)
	assert.Empty(t, report.SupportingEvidence)
	assert.NotContains(t, result.Summary, "deploy-1")
	assert.Empty(t, result.Evidence)
}

func TestBuildEvidenceReportExcludesEvidenceForSiblingHypotheses(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FSM.MinConfidence = 0.9
	cfg.FSM.MinSupport = 2
	engine := NewGoSEngine(cfg, &testLogger{})
	graph := belief.NewBeliefGraph()
	frontierID := graph.AddHypothesis("上游故障", 0.6, 1, "检查上游")
	siblingID := graph.AddHypothesis("重试风暴", 0.58, 1, "检查重试")
	update := engine.updateGraph(context.Background(), graph, []*experts.ExpertAnalysis{{
		ExpertName: "network_sre",
		Evidence: []experts.EvidenceItem{{
			SourceType: "log", SourceID: "retry-log", Title: "503 without backoff", Snippet: "requests amplified",
			Relation: experts.EvidenceRelationSupport, TargetHypothesisID: siblingID, Strength: 0.9,
		}},
	}}, &belief.Frontier{NodeID: frontierID, Label: "上游故障", Level: 1})
	require.True(t, update.Committed, update.Error)

	report := engine.buildEvidenceReport(graph, &belief.Frontier{NodeID: frontierID, Label: "上游故障", Level: 1, Score: 0.6}, &RunStats{})

	assert.Empty(t, report.SupportingEvidence)
	assert.NotContains(t, formatEvidenceReport(report), "retry-log")
}

func TestGoSEngineEmitsOrderedUserStateEvents(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SessionMaxSteps = 1
	cfg.FSM.MaxSteps = 1
	cfg.FSM.GapDelta = 0.1
	cfg.FSM.MinConfidence = 0.1
	cfg.FSM.MinSupport = 1
	engine := NewGoSEngine(cfg, &testLogger{})
	engine.RegisterExpert("linux_sre", &mockExpert{name: "linux_sre", response: &experts.ExpertAnalysis{
		ExpertName: "linux_sre", Status: "succeeded", Confidence: 0.9,
		Evidence: []experts.EvidenceItem{
			{SourceType: "metric", SourceID: "cpu-1", Title: "CPU high", Relation: experts.EvidenceRelationSupport, Strength: 0.9},
			{SourceType: "metric", SourceID: "cpu-normal", Title: "CPU normal", Relation: experts.EvidenceRelationRefute, Strength: 0.8},
		},
	}})
	stateKinds := make([]string, 0)
	engine.SetEmitter(func(_ context.Context, _ string, payload map[string]any) {
		if payload["stage"] == "state_transition" {
			stateKinds = append(stateKinds, payload["state_kind"].(string))
		}
	})

	result := engine.Run(context.Background(), "服务响应超时")

	require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Equal(t, []string{"explore", "report", "degraded"}, stateKinds)
}

func TestGoSEngineEmitsDrillDownAndBacktrackUserStates(t *testing.T) {
	cfg := phase2TestConfig()
	cfg.SessionMaxSteps = 3
	cfg.FSM.MinConfidence = 0.6
	cfg.FSM.MinSupport = 1
	cfg.FSM.GapDelta = 0.1
	cfg.Confidence.SupportWeight = 1
	cfg.Confidence.RefuteWeight = 1
	engine := NewGoSEngine(cfg, &testLogger{})
	engine.RegisterExpert("linux_sre", &backtrackingEventExpert{})
	stateKinds := make([]string, 0)
	engine.SetEmitter(func(_ context.Context, _ string, payload map[string]any) {
		if payload["stage"] == "state_transition" {
			stateKinds = append(stateKinds, payload["state_kind"].(string))
		}
	})

	result := engine.Run(context.Background(), "CPU 使用率持续升高")

	require.Equal(t, protocol.ResultStatusDegraded, result.Status)
	assert.Contains(t, stateKinds, "drill_down")
	assert.Contains(t, stateKinds, "backtrack")
	drillIndex := indexOfString(stateKinds, "drill_down")
	backtrackIndex := indexOfString(stateKinds, "backtrack")
	assert.Greater(t, backtrackIndex, drillIndex)
}

func indexOfString(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}
