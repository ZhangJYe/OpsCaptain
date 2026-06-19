package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/cmdb"
	"context"
)

func (c *ControllerV1) CMDBServiceList(ctx context.Context, req *v1.CMDBServiceListReq) (res *v1.CMDBServiceListRes, err error) {
	result := c.cmdbApp.ListAll()
	return &v1.CMDBServiceListRes{
		Items:   result["items"],
		Success: result["success"].(bool),
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceSearch(ctx context.Context, req *v1.CMDBServiceSearchReq) (res *v1.CMDBServiceSearchRes, err error) {
	result := c.cmdbApp.Search(req.Q, req.Limit)
	return &v1.CMDBServiceSearchRes{
		Items:   result["items"],
		Success: result["success"].(bool),
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceGet(ctx context.Context, req *v1.CMDBServiceGetReq) (res *v1.CMDBServiceGetRes, err error) {
	result := c.cmdbApp.GetService(req.Name)
	return &v1.CMDBServiceGetRes{
		Service: result["service"],
		Success: result["success"].(bool),
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceDeps(ctx context.Context, req *v1.CMDBServiceDepsReq) (res *v1.CMDBServiceDepsRes, err error) {
	result := c.cmdbApp.GetDependencies(req.Name)
	return &v1.CMDBServiceDepsRes{
		Dependencies: result["dependencies"],
		Dependents:   result["dependents"],
		Success:      result["success"].(bool),
		Error:        getString(result, "error"),
		Message:      getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceByCluster(ctx context.Context, req *v1.CMDBServiceByClusterReq) (res *v1.CMDBServiceByClusterRes, err error) {
	result := c.cmdbApp.ListByCluster(req.Cluster)
	return &v1.CMDBServiceByClusterRes{
		Items:   result["items"],
		Success: result["success"].(bool),
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceByTeam(ctx context.Context, req *v1.CMDBServiceByTeamReq) (res *v1.CMDBServiceByTeamRes, err error) {
	result := c.cmdbApp.ListByTeam(req.Team)
	return &v1.CMDBServiceByTeamRes{
		Items:   result["items"],
		Success: result["success"].(bool),
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceCreate(ctx context.Context, req *v1.CMDBServiceCreateReq) (res *v1.CMDBServiceCreateRes, err error) {
	svc := cmdb.ServiceInfo{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Owner:        req.Owner,
		Team:         req.Team,
		Cluster:      req.Cluster,
		Env:          req.Env,
		Region:       req.Region,
		Language:     req.Language,
		Port:         req.Port,
		Dependencies: req.Dependencies,
		Description:  req.Description,
		OnCall:       req.OnCall,
		LastDeploy:   req.LastDeploy,
		Tags:         req.Tags,
	}
	result := c.cmdbApp.CreateService(svc)
	return &v1.CMDBServiceCreateRes{
		Success: result["success"].(bool),
		Service: result["service"],
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceUpdate(ctx context.Context, req *v1.CMDBServiceUpdateReq) (res *v1.CMDBServiceUpdateRes, err error) {
	svc := cmdb.ServiceInfo{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		Owner:        req.Owner,
		Team:         req.Team,
		Cluster:      req.Cluster,
		Env:          req.Env,
		Region:       req.Region,
		Language:     req.Language,
		Port:         req.Port,
		Dependencies: req.Dependencies,
		Description:  req.Description,
		OnCall:       req.OnCall,
		LastDeploy:   req.LastDeploy,
		Tags:         req.Tags,
	}
	result := c.cmdbApp.UpdateService(req.Name, svc)
	return &v1.CMDBServiceUpdateRes{
		Success: result["success"].(bool),
		Service: result["service"],
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func (c *ControllerV1) CMDBServiceDelete(ctx context.Context, req *v1.CMDBServiceDeleteReq) (res *v1.CMDBServiceDeleteRes, err error) {
	result := c.cmdbApp.DeleteService(req.Name)
	return &v1.CMDBServiceDeleteRes{
		Success: result["success"].(bool),
		Error:   getString(result, "error"),
		Message: getString(result, "message"),
	}, nil
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
