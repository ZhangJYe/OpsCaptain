package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObserveGoSRunExposesLatencyCallsAndGraphMetrics(t *testing.T) {
	ObserveGoSRun(
		"degraded",
		150*time.Millisecond,
		map[string]int64{"ingest": 10, "act": 100, "report": 40},
		map[string]int{"llm": 3, "tool": 2, "rag": 1},
		map[string]int{"nodes": 12, "edges": 16, "history_bytes": 2048},
		map[string]int{"frontier_changes": 1, "backtracks": 1, "new_evidence": 2},
	)

	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	names := make(map[string]struct{}, len(families))
	for _, family := range families {
		names[family.GetName()] = struct{}{}
	}
	for _, name := range []string{
		"opscaptionai_gos_run_duration_seconds",
		"opscaptionai_gos_phase_duration_seconds",
		"opscaptionai_gos_calls_per_run",
		"opscaptionai_gos_graph_size",
		"opscaptionai_gos_transitions_per_run",
	} {
		_, exists := names[name]
		assert.True(t, exists, name)
	}
}
