package v1

import (
	"github.com/gogf/gf/v2/frame/g"
)

// ==================== MCP Tools ====================

type MCPToolCreateReq struct {
	g.Meta      `path:"/mcp_tools" method:"post" summary:"创建MCP工具"`
	Name        string `json:"Name" v:"required|max-length:128#工具名称不能为空|工具名称长度不能超过128"`
	Description string `json:"Description,omitempty" v:"max-length:512#工具描述长度不能超过512"`
	Endpoint    string `json:"Endpoint" v:"required|max-length:256#工具Endpoint不能为空|Endpoint长度不能超过256"`
	Protocol    string `json:"Protocol,omitempty" v:"in:mcp,openapi,sse#协议类型不合法"`
	Config      string `json:"Config,omitempty"`
}

type MCPToolCreateRes struct {
	Tool interface{} `json:"tool"`
}

type MCPToolListReq struct {
	g.Meta `path:"/mcp_tools" method:"get" summary:"查询MCP工具列表"`
	Status string `json:"status,omitempty" v:"in:draft,pending,approved,rejected,disabled#工具状态不合法"`
	Page   int    `json:"page,omitempty"`
	Size   int    `json:"size,omitempty"`
}

type MCPToolListRes struct {
	Items []interface{} `json:"items"`
	Total int           `json:"total"`
}

type MCPToolUpdateReq struct {
	g.Meta      `path:"/mcp_tools/{ToolId}" method:"put" summary:"更新MCP工具"`
	ToolId      string `json:"ToolId" v:"required|max-length:128#工具ID不能为空|工具ID长度不能超过128"`
	Name        string `json:"Name,omitempty" v:"max-length:128#工具名称长度不能超过128"`
	Description string `json:"Description,omitempty" v:"max-length:512#工具描述长度不能超过512"`
	Endpoint    string `json:"Endpoint,omitempty" v:"max-length:256#Endpoint长度不能超过256"`
	Protocol    string `json:"Protocol,omitempty" v:"in:mcp,openapi,sse#协议类型不合法"`
	Config      string `json:"Config,omitempty"`
}

type MCPToolUpdateRes struct {
	Tool interface{} `json:"tool"`
}

type MCPToolDeleteReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}" method:"delete" summary:"删除MCP工具"`
	ToolId string `json:"ToolId" v:"required|max-length:128#工具ID不能为空|工具ID长度不能超过128"`
}

type MCPToolDeleteRes struct {
	Success bool `json:"success"`
}

type MCPToolTestReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}/test" method:"post" summary:"测试MCP工具"`
	ToolId string `json:"ToolId" v:"required|max-length:128#工具ID不能为空|工具ID长度不能超过128"`
}

type MCPToolTestRes struct {
	Success bool        `json:"success"`
	Result  interface{} `json:"result,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type MCPToolApproveReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}/approve" method:"post" summary:"审批通过MCP工具"`
	ToolId string `json:"ToolId" v:"required|max-length:128#工具ID不能为空|工具ID长度不能超过128"`
}

type MCPToolApproveRes struct {
	Success bool `json:"success"`
}

type MCPToolRejectReq struct {
	g.Meta `path:"/mcp_tools/{ToolId}/reject" method:"post" summary:"拒绝MCP工具"`
	ToolId string `json:"ToolId" v:"required|max-length:128#工具ID不能为空|工具ID长度不能超过128"`
	Reason string `json:"Reason,omitempty" v:"max-length:512#拒绝原因长度不能超过512"`
}

type MCPToolRejectRes struct {
	Success bool `json:"success"`
}

// ==================== User Skills ====================

type UserSkillCreateReq struct {
	g.Meta      `path:"/skills" method:"post" summary:"创建用户技能"`
	Name        string `json:"Name" v:"required|max-length:128#技能名称不能为空|技能名称长度不能超过128"`
	Description string `json:"Description,omitempty" v:"max-length:512#技能描述长度不能超过512"`
	Content     string `json:"Content" v:"required|max-length:8000#技能内容不能为空|技能内容长度不能超过8000"`
	Domain      string `json:"Domain,omitempty" v:"max-length:64#领域长度不能超过64"`
}

type UserSkillCreateRes struct {
	Skill interface{} `json:"skill"`
}

type UserSkillListReq struct {
	g.Meta `path:"/skills" method:"get" summary:"查询用户技能列表"`
	Status string `json:"status,omitempty" v:"in:draft,pending,approved,rejected,disabled#技能状态不合法"`
	Page   int    `json:"page,omitempty"`
	Size   int    `json:"size,omitempty"`
}

type UserSkillListRes struct {
	Items []interface{} `json:"items"`
	Total int           `json:"total"`
}

type UserSkillUpdateReq struct {
	g.Meta      `path:"/skills/{SkillId}" method:"put" summary:"更新用户技能"`
	SkillId     string `json:"SkillId" v:"required|max-length:128#技能ID不能为空|技能ID长度不能超过128"`
	Name        string `json:"Name,omitempty" v:"max-length:128#技能名称长度不能超过128"`
	Description string `json:"Description,omitempty" v:"max-length:512#技能描述长度不能超过512"`
	Content     string `json:"Content,omitempty" v:"max-length:8000#技能内容长度不能超过8000"`
	Domain      string `json:"Domain,omitempty" v:"max-length:64#领域长度不能超过64"`
}

type UserSkillUpdateRes struct {
	Skill interface{} `json:"skill"`
}

type UserSkillDeleteReq struct {
	g.Meta  `path:"/skills/{SkillId}" method:"delete" summary:"删除用户技能"`
	SkillId string `json:"SkillId" v:"required|max-length:128#技能ID不能为空|技能ID长度不能超过128"`
}

type UserSkillDeleteRes struct {
	Success bool `json:"success"`
}

type UserSkillApproveReq struct {
	g.Meta  `path:"/skills/{SkillId}/approve" method:"post" summary:"审批通过用户技能"`
	SkillId string `json:"SkillId" v:"required|max-length:128#技能ID不能为空|技能ID长度不能超过128"`
}

type UserSkillApproveRes struct {
	Success bool `json:"success"`
}

type UserSkillRejectReq struct {
	g.Meta  `path:"/skills/{SkillId}/reject" method:"post" summary:"拒绝用户技能"`
	SkillId string `json:"SkillId" v:"required|max-length:128#技能ID不能为空|技能ID长度不能超过128"`
	Reason  string `json:"Reason,omitempty" v:"max-length:512#拒绝原因长度不能超过512"`
}

type UserSkillRejectRes struct {
	Success bool `json:"success"`
}
