package chat

import (
	"context"
	"encoding/json"
	"time"

	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/skills"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

// MCPToolCreate creates a new MCP tool with StatusPending.
func (c *ControllerV1) MCPToolCreate(ctx context.Context, req *v1.MCPToolCreateReq) (res *v1.MCPToolCreateRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	tool := skills.UserMCPTool{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Transport:   req.Protocol,
		EndpointURL: req.Endpoint,
		Status:      skills.StatusPending,
		CreatedAt:   time.Now(),
		CreatedBy:   g.RequestFromCtx(ctx).GetClientIp(),
	}

	// Parse optional Config JSON for extra fields.
	if req.Config != "" {
		var cfg map[string]any
		if jsonErr := json.Unmarshal([]byte(req.Config), &cfg); jsonErr == nil {
			if v, ok := cfg["tool_name"].(string); ok {
				tool.ToolName = v
			}
			if v, ok := cfg["auth_token"].(string); ok {
				tool.AuthToken = v
			}
			if v, ok := cfg["http_url"].(string); ok {
				tool.HTTPURL = v
			}
			if v, ok := cfg["timeout_ms"].(float64); ok {
				tool.TimeoutMs = int(v)
			}
		}
	}
	// Default ToolName to the tool name if not set.
	if tool.ToolName == "" {
		tool.ToolName = tool.Name
	}

	data.Tools = append(data.Tools, tool)
	if err = c.userSkillStore.Save(ctx, data); err != nil {
		return nil, err
	}

	return &v1.MCPToolCreateRes{Tool: tool}, nil
}

// MCPToolList returns all user MCP tools, optionally filtered by status.
func (c *ControllerV1) MCPToolList(ctx context.Context, req *v1.MCPToolListReq) (res *v1.MCPToolListRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	var items []interface{}
	for _, t := range data.Tools {
		if req.Status != "" && t.Status != req.Status {
			continue
		}
		items = append(items, t)
	}

	return &v1.MCPToolListRes{Items: items, Total: len(items)}, nil
}

// MCPToolUpdate updates name, description, endpoint, protocol, and config of an existing tool.
func (c *ControllerV1) MCPToolUpdate(ctx context.Context, req *v1.MCPToolUpdateReq) (res *v1.MCPToolUpdateRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i, t := range data.Tools {
		if t.ID != req.ToolId {
			continue
		}
		if req.Name != "" {
			data.Tools[i].Name = req.Name
		}
		if req.Description != "" {
			data.Tools[i].Description = req.Description
		}
		if req.Endpoint != "" {
			data.Tools[i].EndpointURL = req.Endpoint
		}
		if req.Protocol != "" {
			data.Tools[i].Transport = req.Protocol
		}
		if req.Config != "" {
			var cfg map[string]any
			if jsonErr := json.Unmarshal([]byte(req.Config), &cfg); jsonErr == nil {
				if v, ok := cfg["tool_name"].(string); ok {
					data.Tools[i].ToolName = v
				}
				if v, ok := cfg["auth_token"].(string); ok {
					data.Tools[i].AuthToken = v
				}
				if v, ok := cfg["http_url"].(string); ok {
					data.Tools[i].HTTPURL = v
				}
				if v, ok := cfg["timeout_ms"].(float64); ok {
					data.Tools[i].TimeoutMs = int(v)
				}
			}
		}
		if err = c.userSkillStore.Save(ctx, data); err != nil {
			return nil, err
		}
		return &v1.MCPToolUpdateRes{Tool: data.Tools[i]}, nil
	}

	return nil, gerror.Newf("tool %q not found", req.ToolId)
}

// MCPToolDelete deletes a tool by ID and unregisters it from the DynamicMCPRegistry.
func (c *ControllerV1) MCPToolDelete(ctx context.Context, req *v1.MCPToolDeleteReq) (res *v1.MCPToolDeleteRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	found := false
	var filtered []skills.UserMCPTool
	for _, t := range data.Tools {
		if t.ID == req.ToolId {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return nil, gerror.Newf("tool %q not found", req.ToolId)
	}

	data.Tools = filtered
	if err = c.userSkillStore.Save(ctx, data); err != nil {
		return nil, err
	}

	c.dynamicMCPReg.Unregister(req.ToolId)
	return &v1.MCPToolDeleteRes{Success: true}, nil
}

// MCPToolTest tests a tool connection by temporarily registering it.
func (c *ControllerV1) MCPToolTest(ctx context.Context, req *v1.MCPToolTestReq) (res *v1.MCPToolTestRes, err error) {
	data, loadErr := c.userSkillStore.Load(ctx)
	if loadErr != nil {
		return nil, loadErr
	}

	var tool skills.UserMCPTool
	var found bool
	for _, t := range data.Tools {
		if t.ID == req.ToolId {
			tool = t
			found = true
			break
		}
	}
	if !found {
		return nil, gerror.Newf("tool %q not found", req.ToolId)
	}

	if regErr := c.dynamicMCPReg.Register(ctx, tool); regErr != nil {
		return &v1.MCPToolTestRes{
			Success: false,
			Error:   regErr.Error(),
		}, nil
	}

	// Connection succeeded; unregister the temporary registration.
	c.dynamicMCPReg.Unregister(req.ToolId)
	return &v1.MCPToolTestRes{Success: true}, nil
}

// MCPToolApprove sets tool status to approved and registers it in the DynamicMCPRegistry.
func (c *ControllerV1) MCPToolApprove(ctx context.Context, req *v1.MCPToolApproveReq) (res *v1.MCPToolApproveRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i, t := range data.Tools {
		if t.ID != req.ToolId {
			continue
		}
		now := time.Now()
		approver := g.RequestFromCtx(ctx).GetClientIp()
		data.Tools[i].Status = skills.StatusApproved
		data.Tools[i].ApprovedAt = &now
		data.Tools[i].ApprovedBy = approver

		if err = c.userSkillStore.Save(ctx, data); err != nil {
			return nil, err
		}

		// Register the approved tool in the dynamic registry.
		if regErr := c.dynamicMCPReg.Register(ctx, data.Tools[i]); regErr != nil {
			g.Log().Warningf(ctx, "failed to register approved tool %q: %v", req.ToolId, regErr)
		}
		return &v1.MCPToolApproveRes{Success: true}, nil
	}

	return nil, gerror.Newf("tool %q not found", req.ToolId)
}

// MCPToolReject sets tool status to rejected.
func (c *ControllerV1) MCPToolReject(ctx context.Context, req *v1.MCPToolRejectReq) (res *v1.MCPToolRejectRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i, t := range data.Tools {
		if t.ID != req.ToolId {
			continue
		}
		data.Tools[i].Status = skills.StatusRejected
		if err = c.userSkillStore.Save(ctx, data); err != nil {
			return nil, err
		}
		return &v1.MCPToolRejectRes{Success: true}, nil
	}

	return nil, gerror.Newf("tool %q not found", req.ToolId)
}
