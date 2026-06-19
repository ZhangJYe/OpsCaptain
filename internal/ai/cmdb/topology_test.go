package cmdb

import (
	"context"
	"testing"

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

type mockTopologyDataSource struct {
	edges []DependencyEdge
	err   error
}

func (m *mockTopologyDataSource) GetDependencies(ctx context.Context, lookbackHours int) ([]DependencyEdge, error) {
	return m.edges, m.err
}

func TestMerge_JaegerEdges_NoOverride(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1"}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	ds := &mockTopologyDataSource{
		edges: []DependencyEdge{
			{Parent: "a", Child: "b", CallCount: 100},
		},
	}
	merger := NewTopologyMerger(repo, ds)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Edges, 1)
	assert.Equal(t, "a", result.Edges[0].Source)
	assert.Equal(t, "b", result.Edges[0].Target)
	assert.Equal(t, "jaeger", result.Edges[0].DataSource)
	assert.Equal(t, int64(100), result.Edges[0].CallCount)
}

func TestMerge_CMDBOverride(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	ds := &mockTopologyDataSource{
		edges: []DependencyEdge{
			{Parent: "a", Child: "b", CallCount: 50},
			{Parent: "a", Child: "c", CallCount: 30},
		},
	}
	merger := NewTopologyMerger(repo, ds)

	result := merger.GetTopology(context.Background(), "", "")

	edgeKeys := make(map[string]string)
	for _, e := range result.Edges {
		edgeKeys[e.Source+"->"+e.Target] = e.DataSource
	}
	assert.Equal(t, "cmdb", edgeKeys["a->b"])
	assert.Equal(t, "jaeger", edgeKeys["a->c"])
}

func TestMerge_CMDBOnly_NoDataSource(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	merger := NewTopologyMerger(repo, nil)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Edges, 1)
	assert.Equal(t, "cmdb", result.Edges[0].DataSource)
}

func TestMerge_DataSourceError_FallbackToCMDB(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}
	repo := newMockRepo(svcA, svcB)

	ds := &mockTopologyDataSource{
		err: assert.AnError,
	}
	merger := NewTopologyMerger(repo, ds)

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

func TestMerge_ServiceNotFound(t *testing.T) {
	repo := newMockRepo()
	merger := NewTopologyMerger(repo, nil)

	result := merger.GetTopology(context.Background(), "", "nonexistent")

	assert.Empty(t, result.Nodes)
	assert.Empty(t, result.Edges)
}

func TestMerge_NodesHaveCorrectSource(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1"}
	repo := newMockRepo(svcA)

	ds := &mockTopologyDataSource{
		edges: []DependencyEdge{
			{Parent: "x", Child: "y", CallCount: 10},
		},
	}
	merger := NewTopologyMerger(repo, ds)

	result := merger.GetTopology(context.Background(), "", "")

	xNode := findNode(result.Nodes, "x")
	require.NotNil(t, xNode)
	assert.Equal(t, "jaeger", xNode.Source)

	yNode := findNode(result.Nodes, "y")
	require.NotNil(t, yNode)
	assert.Equal(t, "jaeger", yNode.Source)

	aNode := findNode(result.Nodes, "a")
	require.NotNil(t, aNode)
	assert.Equal(t, "cmdb", aNode.Source)
}

func TestMerge_JaegerEdgeNotInCMDB(t *testing.T) {
	svcA := ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1"}
	repo := newMockRepo(svcA)

	ds := &mockTopologyDataSource{
		edges: []DependencyEdge{
			{Parent: "x", Child: "y", CallCount: 10},
		},
	}
	merger := NewTopologyMerger(repo, ds)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Len(t, result.Nodes, 3)
	assert.Len(t, result.Edges, 1)
}

func TestMerge_Empty(t *testing.T) {
	repo := newMockRepo()
	merger := NewTopologyMerger(repo, nil)

	result := merger.GetTopology(context.Background(), "", "")

	assert.Empty(t, result.Nodes)
	assert.Empty(t, result.Edges)
}

func findNode(nodes []TopologyNode, id string) *TopologyNode {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
	}
	return nil
}
