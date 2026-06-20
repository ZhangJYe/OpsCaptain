package cmdb_test

import (
	"SuperBizAgent/internal/ai/cmdb"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type refreshCountingDS struct {
	count atomic.Int32
}

func (d *refreshCountingDS) GetDependencies(ctx context.Context, lookbackHours int) ([]cmdb.DependencyEdge, error) {
	d.count.Add(1)
	return []cmdb.DependencyEdge{
		{Parent: "a", Child: "b", CallCount: 100},
	}, nil
}

func TestRefresh_BackgroundLoop(t *testing.T) {
	svcA := cmdb.ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1", Dependencies: []string{"b"}}
	svcB := cmdb.ServiceInfo{Name: "b", DisplayName: "B", Cluster: "prod", Owner: "team2"}

	repo := newRefreshMockRepo(svcA, svcB)
	ds := &refreshCountingDS{}

	merger := cmdb.NewTopologyMerger(repo, ds, 100*time.Millisecond, 24)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	merger.StartRefresh(ctx)

	// Wait for 3 refresh cycles (initial + 2 ticks)
	time.Sleep(350 * time.Millisecond)

	callCount := int(ds.count.Load())
	assert.GreaterOrEqual(t, callCount, 3, "expected at least 3 data source calls (initial + 2 ticks), got %d", callCount)

	exists, _, nodes, edges := merger.CacheInfo()
	assert.True(t, exists, "cache should be populated after refresh")
	assert.Equal(t, 2, nodes)
	assert.Equal(t, 1, edges)

	// GetTopology should use cache (no additional calls)
	beforeCalls := int(ds.count.Load())
	result := merger.GetTopology(ctx, "", "")
	afterCalls := int(ds.count.Load())
	assert.Equal(t, beforeCalls, afterCalls, "GetTopology should use cache, not call data source")
	assert.Len(t, result.Edges, 1)
}

func TestRefresh_DisabledWhenIntervalZero(t *testing.T) {
	svcA := cmdb.ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1"}
	repo := newRefreshMockRepo(svcA)
	ds := &refreshCountingDS{}

	merger := cmdb.NewTopologyMerger(repo, ds, 0, 24)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	merger.StartRefresh(ctx)
	time.Sleep(200 * time.Millisecond)

	callCount := int(ds.count.Load())
	assert.Equal(t, 0, callCount, "no calls expected when refresh disabled")
}

func TestRefresh_DisabledWhenNoDataSource(t *testing.T) {
	svcA := cmdb.ServiceInfo{Name: "a", DisplayName: "A", Cluster: "prod", Owner: "team1"}
	repo := newRefreshMockRepo(svcA)

	merger := cmdb.NewTopologyMerger(repo, nil, 100*time.Millisecond, 24)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	merger.StartRefresh(ctx)
	time.Sleep(200 * time.Millisecond)

	exists, _, _, _ := merger.CacheInfo()
	assert.False(t, exists, "no cache expected without data source")
}

type refreshMockRepo struct {
	services map[string]*cmdb.ServiceInfo
	all      []cmdb.ServiceInfo
}

func newRefreshMockRepo(services ...cmdb.ServiceInfo) *refreshMockRepo {
	m := &refreshMockRepo{
		services: make(map[string]*cmdb.ServiceInfo),
		all:      services,
	}
	for i := range services {
		m.services[services[i].Name] = &services[i]
	}
	return m
}

func (m *refreshMockRepo) GetService(name string) (*cmdb.ServiceInfo, bool) {
	s, ok := m.services[name]
	return s, ok
}
func (m *refreshMockRepo) SearchServices(keyword string, limit int) []cmdb.ServiceInfo { return nil }
func (m *refreshMockRepo) ListServicesByCluster(cluster string) []cmdb.ServiceInfo {
	var r []cmdb.ServiceInfo
	for _, s := range m.all {
		if s.Cluster == cluster {
			r = append(r, s)
		}
	}
	return r
}
func (m *refreshMockRepo) ListServicesByTeam(team string) []cmdb.ServiceInfo { return nil }
func (m *refreshMockRepo) GetDependents(name string) []string               { return nil }
func (m *refreshMockRepo) ListAll() []cmdb.ServiceInfo                      { return m.all }
func (m *refreshMockRepo) CreateService(svc cmdb.ServiceInfo) error         { return nil }
func (m *refreshMockRepo) UpdateService(name string, svc cmdb.ServiceInfo) error { return nil }
func (m *refreshMockRepo) DeleteService(name string) error                  { return nil }
