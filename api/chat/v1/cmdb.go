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

type CMDBServiceCreateReq struct {
	g.Meta       `path:"/cmdb/services" method:"post" summary:"新增服务"`
	Name         string   `json:"name" v:"required"`
	DisplayName  string   `json:"display_name,omitempty"`
	Owner        string   `json:"owner" v:"required"`
	Team         string   `json:"team" v:"required"`
	Cluster      string   `json:"cluster" v:"required"`
	Env          string   `json:"env" v:"required"`
	Region       string   `json:"region,omitempty"`
	Language     string   `json:"language,omitempty"`
	Port         int      `json:"port,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Description  string   `json:"description,omitempty"`
	OnCall       string   `json:"on_call,omitempty"`
	LastDeploy   string   `json:"last_deploy,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type CMDBServiceCreateRes struct {
	Success bool        `json:"success"`
	Service interface{} `json:"service,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type CMDBServiceUpdateReq struct {
	g.Meta       `path:"/cmdb/services/{name}" method:"put" summary:"更新服务"`
	Name         string   `json:"name" v:"required"`
	DisplayName  string   `json:"display_name,omitempty"`
	Owner        string   `json:"owner" v:"required"`
	Team         string   `json:"team" v:"required"`
	Cluster      string   `json:"cluster" v:"required"`
	Env          string   `json:"env" v:"required"`
	Region       string   `json:"region,omitempty"`
	Language     string   `json:"language,omitempty"`
	Port         int      `json:"port,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
	Description  string   `json:"description,omitempty"`
	OnCall       string   `json:"on_call,omitempty"`
	LastDeploy   string   `json:"last_deploy,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type CMDBServiceUpdateRes struct {
	Success bool        `json:"success"`
	Service interface{} `json:"service,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type CMDBServiceDeleteReq struct {
	g.Meta `path:"/cmdb/services/{name}" method:"delete" summary:"删除服务"`
	Name   string `json:"name" v:"required"`
}

type CMDBServiceDeleteRes struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type CMDBHostListReq struct {
	g.Meta `path:"/cmdb/hosts" method:"get" summary:"查询所有主机"`
}

type CMDBHostListRes struct {
	Items    interface{} `json:"items"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type CMDBHostGetReq struct {
	g.Meta `path:"/cmdb/hosts/{name}" method:"get" summary:"获取单个主机"`
	Name   string `json:"name" v:"required|max-length:128#主机名不能为空|主机名长度不能超过128"`
}

type CMDBHostGetRes struct {
	Host    interface{} `json:"host,omitempty"`
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type CMDBHostByServiceReq struct {
	g.Meta  `path:"/cmdb/hosts/service/{service}" method:"get" summary:"按服务查询主机"`
	Service string `json:"service" v:"required|max-length:128#服务名不能为空|服务名长度不能超过128"`
}

type CMDBHostByServiceRes struct {
	Items    interface{} `json:"items"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type CMDBHostByClusterReq struct {
	g.Meta  `path:"/cmdb/hosts/cluster/{cluster}" method:"get" summary:"按集群查询主机"`
	Cluster string `json:"cluster" v:"required|max-length:128#集群名不能为空|集群名长度不能超过128"`
}

type CMDBHostByClusterRes struct {
	Items    interface{} `json:"items"`
	Success  bool        `json:"success"`
	Error    string      `json:"error,omitempty"`
	Message  string      `json:"message,omitempty"`
}

type CMDBHostCreateReq struct {
	g.Meta       `path:"/cmdb/hosts" method:"post" summary:"新增主机"`
	Name         string   `json:"name" v:"required"`
	Service      string   `json:"service" v:"required"`
	IP           string   `json:"ip" v:"required"`
	Node         string   `json:"node,omitempty"`
	Cluster      string   `json:"cluster" v:"required"`
	Env          string   `json:"env" v:"required"`
	Status       string   `json:"status" v:"required"`
	LastRestart  string   `json:"last_restart,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type CMDBHostCreateRes struct {
	Success bool        `json:"success"`
	Host    interface{} `json:"host,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type CMDBHostUpdateReq struct {
	g.Meta       `path:"/cmdb/hosts/{name}" method:"put" summary:"更新主机"`
	Name         string   `json:"name" v:"required"`
	Service      string   `json:"service" v:"required"`
	IP           string   `json:"ip" v:"required"`
	Node         string   `json:"node,omitempty"`
	Cluster      string   `json:"cluster" v:"required"`
	Env          string   `json:"env" v:"required"`
	Status       string   `json:"status" v:"required"`
	LastRestart  string   `json:"last_restart,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

type CMDBHostUpdateRes struct {
	Success bool        `json:"success"`
	Host    interface{} `json:"host,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}

type CMDBHostDeleteReq struct {
	g.Meta `path:"/cmdb/hosts/{name}" method:"delete" summary:"删除主机"`
	Name   string `json:"name" v:"required"`
}

type CMDBHostDeleteRes struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

type CMDBTopologyReq struct {
	g.Meta  `path:"/cmdb/topology" method:"get" summary:"获取服务拓扑图"`
	Cluster string `json:"cluster,omitempty"`
	Service string `json:"service,omitempty"`
}

type CMDBTopologyRes struct {
	Success bool        `json:"success"`
	Nodes   interface{} `json:"nodes,omitempty"`
	Edges   interface{} `json:"edges,omitempty"`
	Error   string      `json:"error,omitempty"`
	Message string      `json:"message,omitempty"`
}
