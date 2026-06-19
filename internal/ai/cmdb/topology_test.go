package cmdb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"SuperBizAgent/internal/infra/jaeger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockServiceRepository struct {
	services   map[string]*ServiceInfo
	all        []ServiceInfo
	byCluster  map[string][]ServiceInfo
	dependents map[string][]string
}

func newMockRepo(services ...ServiceInfo) *mockServiceRepository {
	m := &mockServiceRepository{
		services:   make(map[string]*ServiceInfo),
		all:        services,
		byCluster:  make(map[string][]ServiceInfo),
		dependents: make(map[string][]string),
	}
	for i := range services {
		m.services[services[i].Name] = &services[i]
		m.byCluster[services[i].Cluster] = append(m.byCluster[services[i].Cluster], services[i])
		for _, dep := range services[i].Dependencies {
			m.dependents[dep] = append(m.dependents[dep], services[i].Name)
		}
	}
	return m
}

func (m *mockServiceRepository) GetService(name string) (*ServiceInfo, bool) {
	svc, ok := m.services[name]
	return svc, ok
}

func (m *mockServiceRepository) SearchServices(keyword string, limit int) []ServiceInfo { return nil }
func (m *mockServiceRepository) ListServicesByCluster(cluster string) []ServiceInfo {
	return m.byCluster[cluster]
}
func (m *mockServiceRepository) ListServicesByTeam(team string) []ServiceInfo   { return nil }
func (m *mockServiceRepository) GetDependents(name string) []string             { return m.dependents[name] }
func (m *mockServiceRepository) ListAll() []ServiceInfo                         { return m.all }
func (m *mockServiceRepository) CreateService(svc ServiceInfo) error            { return nil }
func (m *mockServiceRepository) UpdateService(name string, svc ServiceInfo) error { return nil }
func (m *mockServiceRepository) DeleteService(name string) error                { return nil }

func newJaegerServer(edges []jaeger.DependencyEdge) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(edges)
	}))
}

func TestMerge_JaegerEdgeWithoutCMDBOverride(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1"}
	repo := newMockRepo(svcA)

	jaegerServer := newJaegerServer([]jaeger.DependencyEdge{
		{Parent: "a", Child: "b", CallCount: 100},
	})
	defer jaegerServer.Close()

	jaegerClient := jaeger.NewClient(jaegerServer.URL)
	merger := NewTopologyMerger(repo, jaegerClient)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Nodes, 2)
	assert.Len(t, result.Edges, 1)
	assert.Equal(t, "a", result.Edges[0].Source)
	assert.Equal(t, "b", result.Edges[0].Target)
	assert.Equal(t, "jaeger", result.Edges[0].DataSource)

	bNode := findNode(result.Nodes, "b")
	require.NotNil(t, bNode)
	assert.Equal(t, "b", bNode.Label)
	assert.Equal(t, "jaeger", bNode.Source)
}

func TestMerge_CMDBOverrideReplacesSameEdge_JaegerKeepsRest(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	jaegerServer := newJaegerServer([]jaeger.DependencyEdge{
		{Parent: "a", Child: "c", CallCount: 50},
	})
	defer jaegerServer.Close()

	jaegerClient := jaeger.NewClient(jaegerServer.URL)
	merger := NewTopologyMerger(repo, jaegerClient)

	result := merger.GetTopology(context.Background(), "", "")

	edgeTargets := make(map[string]string)
	for _, e := range result.Edges {
		edgeTargets[e.Target] = e.DataSource
	}

	assert.Equal(t, "cmdb", edgeTargets["b"])
	assert.Equal(t, "jaeger", edgeTargets["c"])
}

func TestMerge_CMDBOverridesSome_JaegerKeepsRest(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	jaegerServer := newJaegerServer([]jaeger.DependencyEdge{
		{Parent: "a", Child: "c", CallCount: 50},
		{Parent: "a", Child: "b", CallCount: 30},
	})
	defer jaegerServer.Close()

	jaegerClient := jaeger.NewClient(jaegerServer.URL)
	merger := NewTopologyMerger(repo, jaegerClient)

	result := merger.GetTopology(context.Background(), "", "")

	edgeTargets := make(map[string]string)
	for _, e := range result.Edges {
		key := e.Source + "->" + e.Target
		edgeTargets[key] = e.DataSource
	}

	assert.Equal(t, "cmdb", edgeTargets["a->b"])
	assert.Equal(t, "jaeger", edgeTargets["a->c"])
}

func TestMerge_JaegerUnavailable_FallbackToCMDBOnly(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	merger := NewTopologyMerger(repo, nil)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Nodes, 2)
	assert.Len(t, result.Edges, 1)
	assert.Equal(t, "cmdb", result.Edges[0].DataSource)
}

func TestMerge_JaegerTimeout_FallbackToCMDBOnly(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer slowServer.Close()

	jaegerClient := jaeger.NewClientWithTimeout(slowServer.URL, 50*time.Millisecond)
	merger := NewTopologyMerger(repo, jaegerClient)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Edges, 1)
	assert.Equal(t, "cmdb", result.Edges[0].DataSource)
}

func TestMerge_ClusterFilter(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	svcC := ServiceInfo{Name: "c", DisplayName: "C", Cluster: "staging", Owner: "team3"}
	repo := newMockRepo(svcA, svcB, svcC)

	merger := NewTopologyMerger(repo, nil)

	result := merger.GetTopology(context.Background(), "prod", "")

	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeIDs[n.ID] = true
	}
	assert.True(t, nodeIDs["a"])
	assert.True(t, nodeIDs["b"])
	assert.False(t, nodeIDs["c"])
}

func TestMerge_SpecificServiceFilter(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	svcC := ServiceInfo{Name: "c", DisplayName: "C", Cluster: "prod", Owner: "team3"}
	repo := newMockRepo(svcA, svcB, svcC)

	merger := NewTopologyMerger(repo, nil)

	result := merger.GetTopology(context.Background(), "", "a")

	nodeIDs := make(map[string]bool)
	for _, n := range result.Nodes {
		nodeIDs[n.ID] = true
	}
	assert.True(t, nodeIDs["a"])
	assert.True(t, nodeIDs["b"])
	assert.False(t, nodeIDs["c"])
}

func TestMerge_CMDBServiceNotInRepo_JaegerDiscovered(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1"}
	repo := newMockRepo(svcA)

	jaegerServer := newJaegerServer([]jaeger.DependencyEdge{
		{Parent: "x", Child: "y", CallCount: 10},
	})
	defer jaegerServer.Close()

	jaegerClient := jaeger.NewClient(jaegerServer.URL)
	merger := NewTopologyMerger(repo, jaegerClient)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Nodes, 3)
	assert.Len(t, result.Edges, 1)
	xNode := findNode(result.Nodes, "x")
	require.NotNil(t, xNode)
	assert.Equal(t, "jaeger", xNode.Source)

	aNode := findNode(result.Nodes, "a")
	require.NotNil(t, aNode)
	assert.Equal(t, "cmdb", aNode.Source)
}

func TestMerge_EmptyCMDB_EmptyJaeger(t *testing.T) {
	repo := newMockRepo()
	merger := NewTopologyMerger(repo, nil)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Nodes, 0)
	assert.Len(t, result.Edges, 0)
}

func findNode(nodes []TopologyNode, id string) *TopologyNode {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}
