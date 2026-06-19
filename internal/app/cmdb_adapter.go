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
