package app

import (
	"SuperBizAgent/internal/ai/cmdb"
	infracmdb "SuperBizAgent/internal/infra/cmdb"
)

// CMDBAdapter wraps infra YAMLLoader and implements ai/cmdb.ServiceRepository
// Lives in app/ (not infra/) so infra/ doesn't import ai/
type CMDBAdapter struct {
	loader *infracmdb.YAMLLoader
}

func NewCMDBAdapter(loader *infracmdb.YAMLLoader) *CMDBAdapter {
	return &CMDBAdapter{loader: loader}
}

func (a *CMDBAdapter) GetService(name string) (*cmdb.ServiceInfo, bool) {
	dto, ok := a.loader.GetService(name)
	if !ok {
		return nil, false
	}
	info := dtoToInfo(dto, a.loader)
	return &info, true
}

func (a *CMDBAdapter) SearchServices(keyword string, limit int) []cmdb.ServiceInfo {
	dtos := a.loader.SearchServices(keyword, limit)
	return dtosToInfos(dtos, a.loader)
}

func (a *CMDBAdapter) ListServicesByCluster(cluster string) []cmdb.ServiceInfo {
	dtos := a.loader.ListServicesByCluster(cluster)
	return dtosToInfos(dtos, a.loader)
}

func (a *CMDBAdapter) ListServicesByTeam(team string) []cmdb.ServiceInfo {
	dtos := a.loader.ListServicesByTeam(team)
	return dtosToInfos(dtos, a.loader)
}

func (a *CMDBAdapter) GetDependents(name string) []string {
	return a.loader.GetDependents(name)
}

func (a *CMDBAdapter) ListAll() []cmdb.ServiceInfo {
	dtos := a.loader.ListAll()
	return dtosToInfos(dtos, a.loader)
}

func dtoToInfo(dto infracmdb.CMDBServiceDTO, loader *infracmdb.YAMLLoader) cmdb.ServiceInfo {
	return cmdb.ServiceInfo{
		Name:         dto.Name,
		DisplayName:  dto.DisplayName,
		Owner:        dto.Owner,
		Team:         dto.Team,
		Cluster:      dto.Cluster,
		Env:          dto.Env,
		Region:       dto.Region,
		Language:     dto.Language,
		Port:         dto.Port,
		Dependencies: dto.Dependencies,
		Dependents:   loader.GetDependents(dto.Name),
		Description:  dto.Description,
		OnCall:       dto.OnCall,
		LastDeploy:   dto.LastDeploy,
		Tags:         dto.Tags,
	}
}

func dtosToInfos(dtos []infracmdb.CMDBServiceDTO, loader *infracmdb.YAMLLoader) []cmdb.ServiceInfo {
	result := make([]cmdb.ServiceInfo, 0, len(dtos))
	for _, dto := range dtos {
		result = append(result, dtoToInfo(dto, loader))
	}
	return result
}

func infoToDTO(info cmdb.ServiceInfo) infracmdb.CMDBServiceDTO {
	return infracmdb.CMDBServiceDTO{
		Name:         info.Name,
		DisplayName:  info.DisplayName,
		Owner:        info.Owner,
		Team:         info.Team,
		Cluster:      info.Cluster,
		Env:          info.Env,
		Region:       info.Region,
		Language:     info.Language,
		Port:         info.Port,
		Dependencies: info.Dependencies,
		Description:  info.Description,
		OnCall:       info.OnCall,
		LastDeploy:   info.LastDeploy,
		Tags:         info.Tags,
	}
}

func (a *CMDBAdapter) CreateService(svc cmdb.ServiceInfo) error {
	return a.loader.CreateService(infoToDTO(svc))
}

func (a *CMDBAdapter) UpdateService(name string, svc cmdb.ServiceInfo) error {
	return a.loader.UpdateService(name, infoToDTO(svc))
}

func (a *CMDBAdapter) DeleteService(name string) error {
	return a.loader.DeleteService(name)
}

func hostDTOToInfo(dto infracmdb.HostDTO) cmdb.HostInfo {
	return cmdb.HostInfo{
		Name:        dto.Name,
		Service:     dto.Service,
		IP:          dto.IP,
		Node:        dto.Node,
		Cluster:     dto.Cluster,
		Env:         dto.Env,
		Status:      dto.Status,
		LastRestart: dto.LastRestart,
		Tags:        dto.Tags,
	}
}

func hostInfoToDTO(info cmdb.HostInfo) infracmdb.HostDTO {
	return infracmdb.HostDTO{
		Name:        info.Name,
		Service:     info.Service,
		IP:          info.IP,
		Node:        info.Node,
		Cluster:     info.Cluster,
		Env:         info.Env,
		Status:      info.Status,
		LastRestart: info.LastRestart,
		Tags:        info.Tags,
	}
}

func (a *CMDBAdapter) GetHost(name string) (*cmdb.HostInfo, bool) {
	dto, ok := a.loader.GetHost(name)
	if !ok {
		return nil, false
	}
	info := hostDTOToInfo(dto)
	return &info, true
}

func (a *CMDBAdapter) ListHostsByService(service string) []cmdb.HostInfo {
	dtos := a.loader.ListHostsByService(service)
	result := make([]cmdb.HostInfo, 0, len(dtos))
	for _, dto := range dtos {
		result = append(result, hostDTOToInfo(dto))
	}
	return result
}

func (a *CMDBAdapter) ListHostsByCluster(cluster string) []cmdb.HostInfo {
	dtos := a.loader.ListHostsByCluster(cluster)
	result := make([]cmdb.HostInfo, 0, len(dtos))
	for _, dto := range dtos {
		result = append(result, hostDTOToInfo(dto))
	}
	return result
}

func (a *CMDBAdapter) ListAllHosts() []cmdb.HostInfo {
	dtos := a.loader.ListAllHosts()
	result := make([]cmdb.HostInfo, 0, len(dtos))
	for _, dto := range dtos {
		result = append(result, hostDTOToInfo(dto))
	}
	return result
}

func (a *CMDBAdapter) CreateHost(host cmdb.HostInfo) error {
	return a.loader.CreateHost(hostInfoToDTO(host))
}

func (a *CMDBAdapter) UpdateHost(name string, host cmdb.HostInfo) error {
	return a.loader.UpdateHost(name, hostInfoToDTO(host))
}

func (a *CMDBAdapter) DeleteHost(name string) error {
	return a.loader.DeleteHost(name)
}
