package cmdb

import (
	"context"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// DependencyEdge represents a caller→callee relationship from an external source.
type DependencyEdge struct {
	Parent    string
	Child     string
	CallCount int64
}

// TopologyDataSource provides service dependency data from external sources.
// Defined in ai/cmdb so infra implementations can satisfy it without circular imports.
type TopologyDataSource interface {
	GetDependencies(ctx context.Context, lookbackHours int) ([]DependencyEdge, error)
}

type TopologyNode struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Type    string `json:"type"`
	Cluster string `json:"cluster,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Source  string `json:"source,omitempty"`
}

type TopologyEdge struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Type       string `json:"type"`
	CallCount  int64  `json:"call_count,omitempty"`
	DataSource string `json:"data_source,omitempty"`
}

type TopologyResult struct {
	Nodes     []TopologyNode `json:"nodes"`
	Edges     []TopologyEdge `json:"edges"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type topologyCache struct {
	result    TopologyResult
	updatedAt time.Time
}

type TopologyMerger struct {
	repo       ServiceRepository
	dataSource TopologyDataSource

	mu          sync.RWMutex
	cache       *topologyCache
	refreshOnce sync.Once

	refreshInterval time.Duration
	lookbackHours   int
}

// NewTopologyMerger creates a merger with optional background refresh.
// refreshInterval=0 disables background refresh (on-demand only).
func NewTopologyMerger(repo ServiceRepository, ds TopologyDataSource, refreshInterval time.Duration, lookbackHours int) *TopologyMerger {
	if lookbackHours <= 0 {
		lookbackHours = 24
	}
	m := &TopologyMerger{
		repo:            repo,
		dataSource:      ds,
		refreshInterval: refreshInterval,
		lookbackHours:   lookbackHours,
	}
	return m
}

// StartRefresh launches the background refresh goroutine. Safe to call multiple times.
func (m *TopologyMerger) StartRefresh(ctx context.Context) {
	if m.refreshInterval <= 0 || m.dataSource == nil {
		return
	}
	m.refreshOnce.Do(func() {
		go m.refreshLoop(ctx)
	})
}

func (m *TopologyMerger) refreshLoop(ctx context.Context) {
	// Initial fetch
	m.refresh(ctx)

	ticker := time.NewTicker(m.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refresh(ctx)
		}
	}
}

func (m *TopologyMerger) refresh(ctx context.Context) {
	result := m.merge(ctx, "", "")
	m.mu.Lock()
	m.cache = &topologyCache{result: result, updatedAt: time.Now()}
	m.mu.Unlock()
	g.Log().Debugf(ctx, "cmdb: topology refreshed — %d nodes, %d edges", len(result.Nodes), len(result.Edges))
}

// GetTopology returns the topology for the given cluster/service filter.
// Uses cached result if available, otherwise merges on-demand.
func (m *TopologyMerger) GetTopology(ctx context.Context, cluster string, service string) TopologyResult {
	// Try cache first
	m.mu.RLock()
	cached := m.cache
	m.mu.RUnlock()

	if cached != nil {
		return m.filter(cached.result, cluster, service)
	}

	// No cache — merge on-demand
	return m.merge(ctx, cluster, service)
}

// GetTopologyWithForce refreshes from source and returns merged result.
func (m *TopologyMerger) GetTopologyWithForce(ctx context.Context, cluster string, service string) TopologyResult {
	result := m.merge(ctx, "", "")
	m.mu.Lock()
	m.cache = &topologyCache{result: result, updatedAt: time.Now()}
	m.mu.Unlock()
	return m.filter(result, cluster, service)
}

// CacheInfo returns cache metadata for observability.
func (m *TopologyMerger) CacheInfo() (exists bool, updatedAt time.Time, nodeCount, edgeCount int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cache == nil {
		return false, time.Time{}, 0, 0
	}
	return true, m.cache.updatedAt, len(m.cache.result.Nodes), len(m.cache.result.Edges)
}

func (m *TopologyMerger) merge(ctx context.Context, cluster string, service string) TopologyResult {
	var cmdbServices []ServiceInfo
	if service != "" {
		svc, found := m.repo.GetService(service)
		if !found {
			return TopologyResult{}
		}
		cmdbServices = []ServiceInfo{*svc}
	} else if cluster != "" {
		cmdbServices = m.repo.ListServicesByCluster(cluster)
	} else {
		cmdbServices = m.repo.ListAll()
	}

	cmdbOverrideDeps := make(map[string][]string)
	for _, svc := range cmdbServices {
		if len(svc.Dependencies) > 0 {
			cmdbOverrideDeps[svc.Name] = svc.Dependencies
		}
	}

	cmdbAllServices := make(map[string]*ServiceInfo)
	for i := range cmdbServices {
		cmdbAllServices[cmdbServices[i].Name] = &cmdbServices[i]
	}

	if cluster == "" && service == "" {
		allServices := m.repo.ListAll()
		for i := range allServices {
			cmdbAllServices[allServices[i].Name] = &allServices[i]
		}
	}

	var externalEdges []DependencyEdge
	if m.dataSource != nil {
		var err error
		externalEdges, err = m.dataSource.GetDependencies(ctx, m.lookbackHours)
		if err != nil {
			g.Log().Warningf(ctx, "cmdb: topology data source fetch failed: %v", err)
			externalEdges = nil
		}
	}

	edgeSet := make(map[string]bool)
	var resultEdges []TopologyEdge

	for _, edge := range externalEdges {
		overridden := false
		if deps, ok := cmdbOverrideDeps[edge.Parent]; ok {
			for _, d := range deps {
				if d == edge.Child {
					overridden = true
					break
				}
			}
		}
		if overridden {
			continue
		}
		key := edge.Parent + "->" + edge.Child
		if !edgeSet[key] {
			edgeSet[key] = true
			resultEdges = append(resultEdges, TopologyEdge{
				Source:     edge.Parent,
				Target:     edge.Child,
				Type:       "calls",
				CallCount:  edge.CallCount,
				DataSource: "jaeger",
			})
		}
	}

	for parent, deps := range cmdbOverrideDeps {
		for _, dep := range deps {
			key := parent + "->" + dep
			if !edgeSet[key] {
				edgeSet[key] = true
				resultEdges = append(resultEdges, TopologyEdge{
					Source:     parent,
					Target:     dep,
					Type:       "depends_on",
					DataSource: "cmdb",
				})
			}
		}
	}

	nodeSet := make(map[string]bool)
	var resultNodes []TopologyNode

	addNode := func(id, source string) {
		if nodeSet[id] {
			return
		}
		nodeSet[id] = true
		if svc, ok := cmdbAllServices[id]; ok {
			resultNodes = append(resultNodes, TopologyNode{
				ID:      svc.Name,
				Label:   svc.DisplayName,
				Type:    "service",
				Cluster: svc.Cluster,
				Owner:   svc.Owner,
				Source:  source,
			})
		} else {
			resultNodes = append(resultNodes, TopologyNode{
				ID:     id,
				Label:  id,
				Type:   "service",
				Source: "jaeger",
			})
		}
	}

	for _, edge := range resultEdges {
		addNode(edge.Source, edge.DataSource)
		addNode(edge.Target, edge.DataSource)
	}

	for _, svc := range cmdbServices {
		addNode(svc.Name, "cmdb")
		for _, dep := range svc.Dependencies {
			addNode(dep, "cmdb")
		}
	}

	return TopologyResult{
		Nodes:     resultNodes,
		Edges:     resultEdges,
		UpdatedAt: time.Now(),
	}
}

func (m *TopologyMerger) filter(result TopologyResult, cluster string, service string) TopologyResult {
	if cluster == "" && service == "" {
		return result
	}

	if service != "" {
		// Service filter: only keep the service and its direct dependencies
		depSet := make(map[string]bool)
		for _, edge := range result.Edges {
			if edge.Source == service {
				depSet[edge.Target] = true
			}
		}
		depSet[service] = true

		var filteredNodes []TopologyNode
		for _, node := range result.Nodes {
			if depSet[node.ID] {
				filteredNodes = append(filteredNodes, node)
			}
		}
		var filteredEdges []TopologyEdge
		for _, edge := range result.Edges {
			if depSet[edge.Source] && depSet[edge.Target] {
				filteredEdges = append(filteredEdges, edge)
			}
		}
		return TopologyResult{Nodes: filteredNodes, Edges: filteredEdges, UpdatedAt: result.UpdatedAt}
	}

	if cluster != "" {
		nodeIDs := make(map[string]bool)
		for _, node := range result.Nodes {
			if node.Cluster == cluster || node.Cluster == "" {
				nodeIDs[node.ID] = true
			}
		}
		var filteredNodes []TopologyNode
		for _, node := range result.Nodes {
			if nodeIDs[node.ID] {
				filteredNodes = append(filteredNodes, node)
			}
		}
		var filteredEdges []TopologyEdge
		for _, edge := range result.Edges {
			if nodeIDs[edge.Source] && nodeIDs[edge.Target] {
				filteredEdges = append(filteredEdges, edge)
			}
		}
		return TopologyResult{Nodes: filteredNodes, Edges: filteredEdges, UpdatedAt: result.UpdatedAt}
	}

	return result
}
