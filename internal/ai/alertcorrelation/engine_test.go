package alertcorrelation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockTopology struct {
	upstream   map[string][]string
	downstream map[string][]string
	all        []string
}

func newMockTopology() *mockTopology {
	return &mockTopology{
		upstream: map[string][]string{
			"paymentservice": {"userservice", "cartservice"},
			"cartservice":    {"userservice"},
			"gateway":        {"paymentservice", "userservice", "cartservice"},
		},
		downstream: map[string][]string{
			"userservice":    {"paymentservice", "cartservice", "gateway"},
			"cartservice":    {"paymentservice", "gateway"},
			"paymentservice": {"gateway"},
		},
		all: []string{"paymentservice", "userservice", "cartservice", "gateway"},
	}
}

func (m *mockTopology) GetUpstream(service string) []string   { return m.upstream[service] }
func (m *mockTopology) GetDownstream(service string) []string { return m.downstream[service] }
func (m *mockTopology) GetAllServices() []string              { return m.all }

func TestAnalyze_EmptyAlerts(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	result := engine.Analyze([]SimplifiedAlert{})

	assert.True(t, result.Success)
	assert.Equal(t, 0, result.TotalAlerts)
	assert.Contains(t, result.Summary, "没有活跃告警")
}

func TestAnalyze_SingleAlert(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	alerts := []SimplifiedAlert{
		{
			AlertName:   "HighErrorRate",
			Description: "paymentservice error rate high",
			State:       "firing",
			ActiveAt:    time.Now(),
			Labels:      map[string]string{"service": "paymentservice"},
		},
	}

	result := engine.Analyze(alerts)

	assert.True(t, result.Success)
	assert.Equal(t, 1, result.TotalAlerts)
	assert.Len(t, result.AlertGroups, 1)
	assert.Contains(t, result.AlertGroups[0].Services, "paymentservice")
}

func TestAnalyze_CascadingFailure(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	now := time.Now()
	alerts := []SimplifiedAlert{
		{
			AlertName:  "HighLatency",
			State:      "firing",
			ActiveAt:   now,
			Labels:     map[string]string{"service": "userservice"},
		},
		{
			AlertName:  "HighErrorRate",
			State:      "firing",
			ActiveAt:   now.Add(2 * time.Minute),
			Labels:     map[string]string{"service": "paymentservice"},
		},
		{
			AlertName:  "GatewayTimeout",
			State:      "firing",
			ActiveAt:   now.Add(4 * time.Minute),
			Labels:     map[string]string{"service": "gateway"},
		},
	}

	result := engine.Analyze(alerts)

	assert.True(t, result.Success)
	assert.Equal(t, 3, result.TotalAlerts)

	// Should detect propagation chains
	assert.NotEmpty(t, result.Propagation, "should detect propagation chains")

	// userservice should be root cause candidate
	require.NotEmpty(t, result.RootCandidates)
	assert.Equal(t, "userservice", result.RootCandidates[0].Service)
	assert.Contains(t, result.RootCandidates[0].Reason, "最早告警")
}

func TestAnalyze_IndependentAlerts(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	now := time.Now()
	// Use a service not in the topology dependency graph
	alerts := []SimplifiedAlert{
		{
			AlertName:  "HighCPU",
			State:      "firing",
			ActiveAt:   now,
			Labels:     map[string]string{"service": "paymentservice"},
		},
		{
			AlertName:  "DiskFull",
			State:      "firing",
			ActiveAt:   now.Add(30 * time.Minute),
			Labels:     map[string]string{"service": "userservice"},
		},
	}

	result := engine.Analyze(alerts)

	assert.True(t, result.Success)
	// paymentservice depends on userservice, so if paymentservice alerts first
	// and userservice alerts later, that's unusual (downstream before upstream)
	// which is correctly flagged as unusual propagation
	// But the key assertion is: no NORMAL upstream_to_downstream propagation
	for _, chain := range result.Propagation {
		assert.NotEqual(t, "upstream_to_downstream", chain.Direction,
			"independent alerts should not have normal propagation")
	}
}

func TestAnalyze_TimeWindowGrouping(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	now := time.Now()
	alerts := []SimplifiedAlert{
		{AlertName: "A1", ActiveAt: now, Labels: map[string]string{"service": "a"}},
		{AlertName: "A2", ActiveAt: now.Add(1 * time.Minute), Labels: map[string]string{"service": "b"}},
		{AlertName: "A3", ActiveAt: now.Add(20 * time.Minute), Labels: map[string]string{"service": "c"}},
	}

	result := engine.Analyze(alerts)

	// First two should be in same group, third in different group
	require.Len(t, result.AlertGroups, 2)
	assert.Len(t, result.AlertGroups[0].Alerts, 2)
	assert.Len(t, result.AlertGroups[1].Alerts, 1)
}

func TestInferService_FromLabels(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	alert := SimplifiedAlert{
		Labels: map[string]string{"service": "paymentservice:8080"},
	}
	svc := engine.inferService(alert)
	assert.Equal(t, "paymentservice", svc)
}

func TestInferService_FromAlertName(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	alert := SimplifiedAlert{
		AlertName:  "HighErrorRate_paymentservice",
		Labels:     map[string]string{},
	}
	svc := engine.inferService(alert)
	assert.Equal(t, "paymentservice", svc)
}

func TestAnalyze_AllDownstreamAffected(t *testing.T) {
	topo := newMockTopology()
	engine := NewEngine(topo, 5, 0.7)

	now := time.Now()
	// userservice fails first, then all downstream follow
	alerts := []SimplifiedAlert{
		{AlertName: "DBConnectionLost", ActiveAt: now, Labels: map[string]string{"service": "userservice"}},
		{AlertName: "AuthTimeout", ActiveAt: now.Add(1 * time.Minute), Labels: map[string]string{"service": "paymentservice"}},
		{AlertName: "CartError", ActiveAt: now.Add(1*time.Minute + 30*time.Second), Labels: map[string]string{"service": "cartservice"}},
		{AlertName: "Gateway500", ActiveAt: now.Add(3 * time.Minute), Labels: map[string]string{"service": "gateway"}},
	}

	result := engine.Analyze(alerts)

	assert.Equal(t, 4, result.TotalAlerts)
	// Should have propagation chains
	assert.NotEmpty(t, result.Propagation)
	// Root should be userservice
	require.NotEmpty(t, result.RootCandidates)
	assert.Equal(t, "userservice", result.RootCandidates[0].Service)
}
