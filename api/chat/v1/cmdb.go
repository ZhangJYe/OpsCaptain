package v1

import (
	"github.com/gogf/gf/v2/frame/g"
)

type CMDBServiceListReq struct {
	g.Meta `path:"/cmdb/services" method:"get" summary:"查询所有服务"`
}

type CMDBServiceListRes struct {
	Items    interface{} `json:"items"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type CMDBServiceSearchReq struct {
	g.Meta `path:"/cmdb/services/search" method:"get" summary:"搜索服务"`
	Q      string `json:"q" v:"required|max-length:200#搜索关键词不能为空|关键词长度不能超过200"`
	Limit  int    `json:"limit,omitempty" d:"20"`
}

type CMDBServiceSearchRes struct {
	Items    interface{} `json:"items"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type CMDBServiceGetReq struct {
	g.Meta `path:"/cmdb/services/{name}" method:"get" summary:"获取单个服务"`
	Name   string `json:"name" v:"required|max-length:128#服务名不能为空|服务名长度不能超过128"`
}

type CMDBServiceGetRes struct {
	Service  interface{} `json:"service,omitempty"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type CMDBServiceDepsReq struct {
	g.Meta `path:"/cmdb/services/{name}/dependencies" method:"get" summary:"获取服务依赖"`
	Name   string `json:"name" v:"required|max-length:128#服务名不能为空|服务名长度不能超过128"`
}

type CMDBServiceDepsRes struct {
	Dependencies interface{} `json:"dependencies,omitempty"`
	Dependents   interface{} `json:"dependents,omitempty"`
	Success      bool        `json:"success"`
	Error        string      `json:"error,omitempty"`
	Message      string      `json:"message,omitempty"`
}

type CMDBServiceByClusterReq struct {
	g.Meta  `path:"/cmdb/services/cluster/{cluster}" method:"get" summary:"按集群查询服务"`
	Cluster string `json:"cluster" v:"required|max-length:128#集群名不能为空|集群名长度不能超过128"`
}

type CMDBServiceByClusterRes struct {
	Items    interface{} `json:"items"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type CMDBServiceByTeamReq struct {
	g.Meta `path:"/cmdb/services/team/{team}" method:"get" summary:"按团队查询服务"`
	Team   string `json:"team" v:"required|max-length:128#团队名不能为空|团队名长度不能超过128"`
}

type CMDBServiceByTeamRes struct {
	Items    interface{} `json:"items"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}
