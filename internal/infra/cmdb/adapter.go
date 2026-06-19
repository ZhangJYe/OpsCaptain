package cmdb

import (
	"SuperBizAgent/internal/ai/cmdb"
)

// CMDBAdapter 实现 ai/cmdb.ServiceRepository interface
// 内部做 DTO → Info 转换，不泄露 infra 细节
type CMDBAdapter struct {
	loader *YAMLLoader
}

// NewCMDBAdapter 创建 adapter
func NewCMDBAdapter(loader *YAMLLoader) *CMDBAdapter {
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

func dtoToInfo(dto cmdbServiceDTO, loader *YAMLLoader) cmdb.ServiceInfo {
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

func dtosToInfos(dtos []cmdbServiceDTO, loader *YAMLLoader) []cmdb.ServiceInfo {
	result := make([]cmdb.ServiceInfo, 0, len(dtos))
	for _, dto := range dtos {
		result = append(result, dtoToInfo(dto, loader))
	}
	return result
}
