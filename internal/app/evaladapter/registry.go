package evaladapter

import (
	"context"
	"fmt"

	"SuperBizAgent/internal/ai/evalharness"
	aitools "SuperBizAgent/internal/ai/tools"

	"github.com/cloudwego/eino/components/tool"
)

func NewDefaultRegistry(ctx context.Context) (*evalharness.Registry, error) {
	registry := evalharness.NewRegistry()
	toolIndex := make(map[string]tool.InvokableTool)
	if currentTime := aitools.NewGetCurrentTimeTool(); currentTime != nil {
		info, err := currentTime.Info(ctx)
		if err == nil && info != nil && info.Name != "" {
			toolIndex[info.Name] = currentTime
		}
	}
	if unavailableLogs := aitools.NewUnavailableLogQueryTool("deterministic evaluation fixture"); unavailableLogs != nil {
		info, err := unavailableLogs.Info(ctx)
		if err == nil && info != nil && info.Name != "" {
			toolIndex[info.Name] = unavailableLogs
		}
	}
	adapters := []evalharness.Adapter{
		NewRouteAdapter(), evalharness.NewRAGAdapter(), evalharness.NewPlanAdapter(), evalharness.NewGoSAdapter(),
		evalharness.NewToolAdapter(func(name string) (tool.InvokableTool, bool) { value, ok := toolIndex[name]; return value, ok }),
		evalharness.NewEvidenceAdapter(),
	}
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			return nil, fmt.Errorf("register %s adapter: %w", adapter.Name(), err)
		}
	}
	return registry, nil
}
