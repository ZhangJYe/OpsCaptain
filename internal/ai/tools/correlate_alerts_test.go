package tools

import (
	"SuperBizAgent/internal/ai/cmdb"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCorrelateRepo struct {
	services   map[string]*cmdb.ServiceInfo
	all        []cmdb.ServiceInfo
	dependents map[string][]string
}

func newFakeCorrelateRepo(services ...cmdb.ServiceInfo) *fakeCorrelateRepo {
	r := &fakeCorrelateRepo{
		services:   make(map[string]*cmdb.ServiceInfo),
		all:        services,
		dependents: make(map[string][]string),
	}
	for i := range services {
		r.services[services[i].Name] = &services[i]
		for _, dep := range services[i].Dependencies {
			r.dependents[dep] = append(r.dependents[dep], services[i].Name)
		}
	}
	return r
}

func (r *fakeCorrelateRepo) GetService(name string) (*cmdb.ServiceInfo, bool) {
	s, ok := r.services[name]
	return s, ok
}
func (r *fakeCorrelateRepo) SearchServices(keyword string, limit int) []cmdb.ServiceInfo { return nil }
func (r *fakeCorrelateRepo) ListServicesByCluster(cluster string) []cmdb.ServiceInfo {
	var result []cmdb.ServiceInfo
	for _, s := range r.all {
		if s.Cluster == cluster {
			result = append(result, s)
		}
	}
	return result
}
func (r *fakeCorrelateRepo) ListServicesByTeam(team string) []cmdb.ServiceInfo     { return nil }
func (r *fakeCorrelateRepo) GetDependents(name string) []string                    { return r.dependents[name] }
func (r *fakeCorrelateRepo) ListAll() []cmdb.ServiceInfo                           { return r.all }
func (r *fakeCorrelateRepo) CreateService(svc cmdb.ServiceInfo) error              { return nil }
func (r *fakeCorrelateRepo) UpdateService(name string, svc cmdb.ServiceInfo) error { return nil }
func (r *fakeCorrelateRepo) DeleteService(name string) error                       { return nil }

func TestCorrelateAlertsTool_NilRepo(t *testing.T) {
	tool := NewCorrelateAlertsTool(nil)
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}

	input := `{"lookback_minutes": 60}`
	result, err := tool.InvokableRun(context.Background(), input)
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}

	var output CorrelateAlertsOutput
	require.NoError(t, json.Unmarshal([]byte(result), &output))
	assert.True(t, output.Degraded)
	assert.Contains(t, output.Error, "CMDB not available")
}

func TestCorrelateAlertsTool_WithRepo(t *testing.T) {
	repo := newFakeCorrelateRepo(
		cmdb.ServiceInfo{Name: "a", Dependencies: []string{"b"}},
		cmdb.ServiceInfo{Name: "b"},
	)
	tool := NewCorrelateAlertsTool(repo)
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}

	// Prometheus may or may not be available in the developer environment.
	// Both a valid correlation result and an explicit degraded result are acceptable.
	input := `{"lookback_minutes": 60}`
	result, err := tool.InvokableRun(context.Background(), input)
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}

	var output CorrelateAlertsOutput
	require.NoError(t, json.Unmarshal([]byte(result), &output))
	if output.Success {
		require.NotNil(t, output.Result)
		return
	}
	assert.True(t, output.Degraded)
	assert.NotEmpty(t, output.Error)
}
