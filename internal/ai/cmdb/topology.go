package cmdb

import "context"

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
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

type TopologyMerger struct {
	repo       ServiceRepository
	dataSource TopologyDataSource
}

func NewTopologyMerger(repo ServiceRepository, ds TopologyDataSource) *TopologyMerger {
	return &TopologyMerger{
		repo:       repo,
		dataSource: ds,
	}
}

func (m *TopologyMerger) GetTopology(ctx context.Context, cluster string, service string) TopologyResult {
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
		externalEdges, err = m.dataSource.GetDependencies(ctx, 24)
		if err != nil {
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

	if cluster != "" {
		filteredNodes := make(map[string]bool)
		for _, node := range resultNodes {
			if node.Cluster == cluster || node.Cluster == "" {
				filteredNodes[node.ID] = true
			}
		}
		var filteredResultNodes []TopologyNode
		for _, node := range resultNodes {
			if filteredNodes[node.ID] {
				filteredResultNodes = append(filteredResultNodes, node)
			}
		}
		var filteredResultEdges []TopologyEdge
		for _, edge := range resultEdges {
			if filteredNodes[edge.Source] && filteredNodes[edge.Target] {
				filteredResultEdges = append(filteredResultEdges, edge)
			}
		}
		return TopologyResult{Nodes: filteredResultNodes, Edges: filteredResultEdges}
	}

	return TopologyResult{Nodes: resultNodes, Edges: resultEdges}
}
