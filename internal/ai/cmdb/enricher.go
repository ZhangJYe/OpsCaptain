package cmdb

import (
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/contextengine"
)

type CMDBEnricherImpl struct {
	repo ServiceRepository
}

func NewCMDBEnricher(repo ServiceRepository) *CMDBEnricherImpl {
	return &CMDBEnricherImpl{repo: repo}
}

func (e *CMDBEnricherImpl) Enrich(ctx context.Context, query string) []contextengine.ContextItem {
	if e.repo == nil {
		return nil
	}

	allServices := e.repo.ListAll()
	if len(allServices) == 0 {
		return nil
	}

	queryLower := strings.ToLower(query)
	var items []contextengine.ContextItem

	for _, svc := range allServices {
		if !strings.Contains(queryLower, strings.ToLower(svc.Name)) {
			continue
		}

		content := fmt.Sprintf("Service: %s (%s)\nOwner: %s | Team: %s\nCluster: %s | Env: %s\nDescription: %s",
			svc.Name, svc.DisplayName, svc.Owner, svc.Team, svc.Cluster, svc.Env, svc.Description)
		if len(svc.Dependencies) > 0 {
			content += fmt.Sprintf("\nDependencies: %s", strings.Join(svc.Dependencies, ", "))
		}

		items = append(items, contextengine.ContextItem{
			ID:         fmt.Sprintf("cmdb-%s", svc.Name),
			SourceType: "cmdb",
			SourceID:   svc.Name,
			Title:      fmt.Sprintf("CMDB: %s", svc.Name),
			Content:    content,
			Score:      1.0,
			TrustLevel: "high",
		})
	}

	return items
}
