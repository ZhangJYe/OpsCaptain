package app

import (
	"SuperBizAgent/internal/ai/cmdb"
)

type CMDBApp struct {
	repo cmdb.ServiceRepository
}

func NewCMDBApp(repo cmdb.ServiceRepository) *CMDBApp {
	return &CMDBApp{repo: repo}
}

func (a *CMDBApp) ListAll() map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	items := a.repo.ListAll()
	return map[string]interface{}{"success": true, "items": items}
}

func (a *CMDBApp) GetService(name string) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	svc, found := a.repo.GetService(name)
	if !found {
		return map[string]interface{}{"success": false, "error": "service not found"}
	}
	return map[string]interface{}{"success": true, "service": svc}
}

func (a *CMDBApp) GetDependencies(name string) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	svc, found := a.repo.GetService(name)
	if !found {
		return map[string]interface{}{"success": false, "error": "service not found"}
	}
	deps := a.repo.GetDependents(name)
	return map[string]interface{}{
		"success":      true,
		"dependencies": svc.Dependencies,
		"dependents":   deps,
	}
}

func (a *CMDBApp) Search(keyword string, limit int) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	items := a.repo.SearchServices(keyword, limit)
	return map[string]interface{}{"success": true, "items": items}
}

func (a *CMDBApp) ListByCluster(cluster string) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	items := a.repo.ListServicesByCluster(cluster)
	return map[string]interface{}{"success": true, "items": items}
}

func (a *CMDBApp) ListByTeam(team string) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	items := a.repo.ListServicesByTeam(team)
	return map[string]interface{}{"success": true, "items": items}
}
