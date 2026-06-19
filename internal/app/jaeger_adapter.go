package app

import (
	"SuperBizAgent/internal/ai/cmdb"
	infrajaeger "SuperBizAgent/internal/infra/jaeger"
	"context"
)

// JaegerTopologyAdapter wraps infra/jaeger.Client and implements cmdb.TopologyDataSource.
// Lives in app/ to avoid ai/cmdb → infra/jaeger import violation.
type JaegerTopologyAdapter struct {
	client *infrajaeger.Client
}

func NewJaegerTopologyAdapter(client *infrajaeger.Client) *JaegerTopologyAdapter {
	return &JaegerTopologyAdapter{client: client}
}

func (a *JaegerTopologyAdapter) GetDependencies(ctx context.Context, lookbackHours int) ([]cmdb.DependencyEdge, error) {
	edges, err := a.client.GetDependencies(ctx, lookbackHours)
	if err != nil {
		return nil, err
	}
	result := make([]cmdb.DependencyEdge, len(edges))
	for i, e := range edges {
		result[i] = cmdb.DependencyEdge{
			Parent:    e.Parent,
			Child:     e.Child,
			CallCount: e.CallCount,
		}
	}
	return result, nil
}
