package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"context"
)

func (c *ControllerV1) CMDBServiceList(ctx context.Context, req *v1.CMDBServiceListReq) (res *v1.CMDBServiceListRes, err error) {
	result := c.cmdbApp.ListAll()
	return &v1.CMDBServiceListRes{
		Items:   result["items"],
		Success: result["success"].(bool),
	}, nil
}

func (c *ControllerV1) CMDBServiceSearch(ctx context.Context, req *v1.CMDBServiceSearchReq) (res *v1.CMDBServiceSearchRes, err error) {
	result := c.cmdbApp.Search(req.Q, req.Limit)
	return &v1.CMDBServiceSearchRes{
		Items:   result["items"],
		Success: result["success"].(bool),
	}, nil
}

func (c *ControllerV1) CMDBServiceGet(ctx context.Context, req *v1.CMDBServiceGetReq) (res *v1.CMDBServiceGetRes, err error) {
	result := c.cmdbApp.GetService(req.Name)
	return &v1.CMDBServiceGetRes{
		Service: result["service"],
		Success: result["success"].(bool),
	}, nil
}

func (c *ControllerV1) CMDBServiceDeps(ctx context.Context, req *v1.CMDBServiceDepsReq) (res *v1.CMDBServiceDepsRes, err error) {
	result := c.cmdbApp.GetDependencies(req.Name)
	return &v1.CMDBServiceDepsRes{
		Dependencies: result["dependencies"],
		Dependents:   result["dependents"],
		Success:      result["success"].(bool),
	}, nil
}

func (c *ControllerV1) CMDBServiceByCluster(ctx context.Context, req *v1.CMDBServiceByClusterReq) (res *v1.CMDBServiceByClusterRes, err error) {
	result := c.cmdbApp.ListByCluster(req.Cluster)
	return &v1.CMDBServiceByClusterRes{
		Items:   result["items"],
		Success: result["success"].(bool),
	}, nil
}

func (c *ControllerV1) CMDBServiceByTeam(ctx context.Context, req *v1.CMDBServiceByTeamReq) (res *v1.CMDBServiceByTeamRes, err error) {
	result := c.cmdbApp.ListByTeam(req.Team)
	return &v1.CMDBServiceByTeamRes{
		Items:   result["items"],
		Success: result["success"].(bool),
	}, nil
}
