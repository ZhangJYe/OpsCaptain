package experts

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"SuperBizAgent/internal/ai/belief"

	einotool "github.com/cloudwego/eino/components/tool"
	einoschema "github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedAtomTestTool struct {
	output string
}

func (t *recordedAtomTestTool) Info(context.Context) (*einoschema.ToolInfo, error) {
	return &einoschema.ToolInfo{Name: "query_recorded_telemetry"}, nil
}

func (t *recordedAtomTestTool) InvokableRun(context.Context, string, ...einotool.Option) (string, error) {
	return t.output, nil
}

func TestAtomizeToolEvidenceSplitsRecordedSignalsAndPreservesProvenance(t *testing.T) {
	observedAt := time.Date(2025, 6, 18, 8, 15, 0, 0, time.UTC)
	document := `# Telemetry Evidence Case

## Metric Signals

- pod_cpu_usage [cartservice-0]: score=9

## Log Signals

- checkoutservice-0 @ 2025-06-18T08:12:00Z: timeout while dialing

## Trace Signals

- request_latency [frontend]: p95=60s`
	payload, err := json.Marshal(map[string]any{
		"success":      true,
		"artifact_ref": "recorded://case-a",
		"data":         document,
	})
	require.NoError(t, err)

	items := atomizeToolEvidence("query_recorded_telemetry", string(payload), "hypothesis-1", observedAt, 12)

	require.Len(t, items, 3)
	assert.Equal(t, []string{"metric", "log", "trace"}, []string{items[0].SignalType, items[1].SignalType, items[2].SignalType})
	assert.Equal(t, "cartservice-0", items[0].Entity)
	assert.Equal(t, "checkoutservice-0", items[1].Entity)
	assert.Equal(t, "frontend", items[2].Entity)
	assert.Equal(t, "recorded://case-a", items[0].ArtifactRef)
	assert.Equal(t, "recorded://case-a", items[1].ArtifactRef)
	assert.Equal(t, "recorded://case-a", items[2].ArtifactRef)
	assert.Equal(t, observedAt, items[0].ObservationTime)
	assert.Equal(t, time.Date(2025, 6, 18, 8, 12, 0, 0, time.UTC), items[1].ObservationTime)
	assert.Contains(t, items[2].Snippet, "p95=60s")
	assert.NotEqual(t, items[0].SourceID, items[1].SourceID)
	assert.NotEqual(t, items[1].SourceID, items[2].SourceID)
	replayed := atomizeToolEvidence("query_recorded_telemetry", string(payload), "hypothesis-1", observedAt.Add(time.Hour), 12)
	require.Len(t, replayed, 3)
	assert.Equal(t, items[0].SourceID, replayed[0].SourceID)
	for _, item := range items {
		assert.Equal(t, "hypothesis-1", item.TargetHypothesisID)
		assert.Equal(t, "query_recorded_telemetry", item.ToolName)
	}
}

func TestAtomizeToolEvidenceLimitRetainsSignalCoverage(t *testing.T) {
	document := `## Metric Signals
- cpu [service-a]: score=9
- memory [service-a]: score=8
- disk [service-a]: score=7

## Log Signals
- service-a @ 2025-06-18T08:12:00Z: timeout

## Trace Signals
- latency [service-a]: p95=60s`

	items := atomizeToolEvidence("query_recorded_telemetry", document, "hypothesis-1", time.Now().UTC(), 3)

	require.Len(t, items, 3)
	assert.Equal(t, []string{"metric", "log", "trace"}, []string{items[0].SignalType, items[1].SignalType, items[2].SignalType})
}

func TestAtomizeRAGEvidenceKeepsDocumentsIndependent(t *testing.T) {
	observedAt := time.Date(2025, 6, 18, 8, 15, 0, 0, time.UTC)
	docs := []*einoschema.Document{
		{ID: "doc-a", Content: "CPU saturation runbook", MetaData: map[string]any{"artifact_ref": "kb://doc-a", "entity": "checkoutservice"}},
		{ID: "doc-b", Content: "Timeout recovery runbook", MetaData: map[string]any{"artifact_ref": "kb://doc-b", "entity": "frontend"}},
	}

	items := atomizeRAGEvidence(docs, "hypothesis-1", observedAt, 12)

	require.Len(t, items, 2)
	assert.Equal(t, "kb://doc-a", items[0].ArtifactRef)
	assert.Equal(t, "checkoutservice", items[0].Entity)
	assert.Equal(t, "CPU saturation runbook", items[0].Snippet)
	assert.Equal(t, "kb://doc-b", items[1].ArtifactRef)
	assert.Equal(t, "frontend", items[1].Entity)
	assert.Equal(t, "Timeout recovery runbook", items[1].Snippet)
	assert.NotEqual(t, items[0].SourceID, items[1].SourceID)
	assert.Len(t, formatEvidenceHistory(items, "root cause", "rag", 8), 2)
}

func TestBaseExpertAssessesEveryAtomicToolSignal(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"success":      true,
		"artifact_ref": "recorded://case-a",
		"data": `## Metric Signals
- cpu [checkoutservice]: score=9
## Log Signals
- checkoutservice: timeout
## Trace Signals
- latency [frontend]: p95=60s`,
	})
	require.NoError(t, err)
	registry := NewToolRegistry()
	registry.Register("query_recorded_telemetry", &recordedAtomTestTool{output: string(payload)})
	historyCount := 0
	expert := NewBaseExpert(ExpertRuntimeConfig{
		Name:              "linux_sre",
		ToolNames:         []string{"query_recorded_telemetry"},
		MaxRetrievalSteps: 2,
		EvidenceMaxItems:  12,
		CallTimeout:       time.Second,
		GenerateContentFunc: func(_ context.Context, _ *belief.Frontier, _ *belief.BeliefGraph, history []RetrievalRecord, decision map[string]string) (string, error) {
			if decision["action"] == "tool_call" {
				return "collect recorded telemetry", nil
			}
			historyCount = len(history)
			return `{"analysis":"three signals assessed","confidence":0.8,"evidence":[{"index":0,"relation":"support","strength":0.9},{"index":1,"relation":"support","strength":0.8},{"index":2,"relation":"neutral","strength":0.2}]}`, nil
		},
	}, registry)
	frontier := &belief.Frontier{NodeID: "hypothesis-1", Label: "dependency timeout", Why: "verify telemetry"}

	result := expert.Run(context.Background(), frontier, belief.NewBeliefGraph())

	require.Equal(t, "succeeded", result.Status)
	require.Len(t, result.Evidence, 3)
	assert.Equal(t, 3, historyCount)
	assert.Equal(t, []string{"metric", "log", "trace"}, []string{result.Evidence[0].SignalType, result.Evidence[1].SignalType, result.Evidence[2].SignalType})
	assert.Equal(t, EvidenceRelationSupport, result.Evidence[0].Relation)
	assert.Equal(t, EvidenceRelationSupport, result.Evidence[1].Relation)
	assert.Equal(t, EvidenceRelationNeutral, result.Evidence[2].Relation)
}
