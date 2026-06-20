package incidentlifecycle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransition_DetectedToTriaged(t *testing.T) {
	lc := &IncidentLifecycle{Status: LifecycleDetected, DetectedAt: 1000}
	err := lc.Transition(LifecycleTriaged)
	require.NoError(t, err)
	assert.Equal(t, LifecycleTriaged, lc.Status)
	assert.Greater(t, lc.TriagedAt, int64(0))
	assert.Greater(t, lc.MTTA, int64(0))
}

func TestTransition_DetectedToCancelled(t *testing.T) {
	lc := &IncidentLifecycle{Status: LifecycleDetected}
	err := lc.Transition(LifecycleCancelled)
	require.NoError(t, err)
	assert.Equal(t, LifecycleCancelled, lc.Status)
}

func TestTransition_InvalidTransition(t *testing.T) {
	lc := &IncidentLifecycle{Status: LifecycleDetected}
	err := lc.Transition(LifecycleResolved)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot transition")
}

func TestTransition_FullLifecycle(t *testing.T) {
	lc := &IncidentLifecycle{Status: LifecycleDetected, DetectedAt: 1000}

	steps := []LifecycleStatus{
		LifecycleTriaged,
		LifecycleResponding,
		LifecycleMitigated,
		LifecycleResolved,
		LifecyclePostmortem,
	}

	for _, step := range steps {
		err := lc.Transition(step)
		require.NoError(t, err, "transition to %s failed", step)
		assert.Equal(t, step, lc.Status)
	}

	assert.Greater(t, lc.MTTA, int64(0))
	assert.Greater(t, lc.MTTR, int64(0))
	assert.Greater(t, lc.PostmortemAt, int64(0))
}

func TestTransition_RespondingToTriaged(t *testing.T) {
	lc := &IncidentLifecycle{Status: LifecycleResponding}
	err := lc.Transition(LifecycleTriaged)
	require.NoError(t, err)
	assert.Equal(t, LifecycleTriaged, lc.Status)
}

func TestTransition_RespondingToMitigated(t *testing.T) {
	lc := &IncidentLifecycle{Status: LifecycleResponding}
	err := lc.Transition(LifecycleMitigated)
	require.NoError(t, err)
	assert.Equal(t, LifecycleMitigated, lc.Status)
	assert.Greater(t, lc.MitigatedAt, int64(0))
}

func TestInferSeverity_MultipleServices(t *testing.T) {
	tests := []struct {
		services   []string
		alertCount int
		downstream bool
		expected   IncidentSeverity
	}{
		{[]string{"a", "b", "c", "d", "e"}, 10, true, SeverityP0},
		{[]string{"a", "b", "c"}, 3, true, SeverityP0},
		{[]string{"a", "b", "c"}, 3, false, SeverityP1},
		{[]string{"a", "b"}, 3, false, SeverityP2},
		{[]string{"a"}, 1, false, SeverityP3},
		{nil, 0, false, SeverityP4},
	}

	for _, tt := range tests {
		result := InferSeverity(tt.services, tt.alertCount, tt.downstream)
		assert.Equal(t, tt.expected, result, "services=%v alerts=%d downstream=%v", tt.services, tt.alertCount, tt.downstream)
	}
}

func TestSeverityDescription(t *testing.T) {
	assert.Contains(t, SeverityDescription(SeverityP0), "全站")
	assert.Contains(t, SeverityDescription(SeverityP1), "严重")
	assert.Contains(t, SeverityDescription(SeverityP4), "低优")
}

func TestFormatDuration(t *testing.T) {
	assert.Equal(t, "-", FormatDuration(0))
	assert.Equal(t, "-", FormatDuration(-1))
	assert.Equal(t, "5s", FormatDuration(5000))
	assert.Equal(t, "2m30s", FormatDuration(150000))
	assert.Equal(t, "1h0m0s", FormatDuration(3600000))
}

func TestParseSeverity(t *testing.T) {
	assert.Equal(t, SeverityP0, ParseSeverity("p0"))
	assert.Equal(t, SeverityP1, ParseSeverity("P1"))
	assert.Equal(t, SeverityP3, ParseSeverity("unknown"))
}

func TestTransition_CancelledIsTerminal(t *testing.T) {
	lc := &IncidentLifecycle{Status: LifecycleCancelled}
	err := lc.Transition(LifecycleDetected)
	assert.Error(t, err)
}
