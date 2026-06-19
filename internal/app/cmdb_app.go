package app

import (
	"SuperBizAgent/internal/ai/cmdb"
	"SuperBizAgent/internal/ai/tools"
)

type CMDBApp struct {
	repo     cmdb.ServiceRepository
	hostRepo cmdb.HostRepository
}

func NewCMDBApp(repo cmdb.ServiceRepository) *CMDBApp {
	return &CMDBApp{repo: repo}
}

func NewCMDBAppWithHost(repo cmdb.ServiceRepository, hostRepo cmdb.HostRepository) *CMDBApp {
	return &CMDBApp{repo: repo, hostRepo: hostRepo}
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
		limit = tools.LoadCMDBMaxResults()
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

func (a *CMDBApp) CreateService(svc cmdb.ServiceInfo) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	if err := validateService(svc); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	if err := a.repo.CreateService(svc); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	created, _ := a.repo.GetService(svc.Name)
	return map[string]interface{}{"success": true, "service": created, "message": "service created"}
}

func (a *CMDBApp) UpdateService(name string, svc cmdb.ServiceInfo) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	if err := validateService(svc); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	if err := a.repo.UpdateService(name, svc); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	updated, _ := a.repo.GetService(name)
	return map[string]interface{}{"success": true, "service": updated, "message": "service updated"}
}

func (a *CMDBApp) DeleteService(name string) map[string]interface{} {
	if a.repo == nil {
		return map[string]interface{}{"success": false, "error": "CMDB repository not configured"}
	}
	if err := a.repo.DeleteService(name); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "message": "service deleted"}
}

func (a *CMDBApp) GetHost(name string) map[string]interface{} {
	if a.hostRepo == nil {
		return map[string]interface{}{"success": false, "error": "host repository not configured"}
	}
	host, found := a.hostRepo.GetHost(name)
	if !found {
		return map[string]interface{}{"success": false, "error": "host not found"}
	}
	return map[string]interface{}{"success": true, "host": host}
}

func (a *CMDBApp) ListHostsByService(service string) map[string]interface{} {
	if a.hostRepo == nil {
		return map[string]interface{}{"success": false, "error": "host repository not configured"}
	}
	items := a.hostRepo.ListHostsByService(service)
	return map[string]interface{}{"success": true, "items": items}
}

func (a *CMDBApp) ListHostsByCluster(cluster string) map[string]interface{} {
	if a.hostRepo == nil {
		return map[string]interface{}{"success": false, "error": "host repository not configured"}
	}
	items := a.hostRepo.ListHostsByCluster(cluster)
	return map[string]interface{}{"success": true, "items": items}
}

func (a *CMDBApp) ListAllHosts() map[string]interface{} {
	if a.hostRepo == nil {
		return map[string]interface{}{"success": false, "error": "host repository not configured"}
	}
	items := a.hostRepo.ListAllHosts()
	return map[string]interface{}{"success": true, "items": items}
}

func (a *CMDBApp) CreateHost(host cmdb.HostInfo) map[string]interface{} {
	if a.hostRepo == nil {
		return map[string]interface{}{"success": false, "error": "host repository not configured"}
	}
	if err := validateHost(host); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	if err := a.hostRepo.CreateHost(host); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	created, _ := a.hostRepo.GetHost(host.Name)
	return map[string]interface{}{"success": true, "host": created, "message": "host created"}
}

func (a *CMDBApp) UpdateHost(name string, host cmdb.HostInfo) map[string]interface{} {
	if a.hostRepo == nil {
		return map[string]interface{}{"success": false, "error": "host repository not configured"}
	}
	if err := validateHost(host); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	if err := a.hostRepo.UpdateHost(name, host); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	updated, _ := a.hostRepo.GetHost(name)
	return map[string]interface{}{"success": true, "host": updated, "message": "host updated"}
}

func (a *CMDBApp) DeleteHost(name string) map[string]interface{} {
	if a.hostRepo == nil {
		return map[string]interface{}{"success": false, "error": "host repository not configured"}
	}
	if err := a.hostRepo.DeleteHost(name); err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}
	return map[string]interface{}{"success": true, "message": "host deleted"}
}
