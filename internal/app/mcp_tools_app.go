package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

type MCPToolApp struct {
	store  UserSkillStore
	dynReg *DynamicMCPRegistry
}

func NewMCPToolApp(store UserSkillStore, dynReg *DynamicMCPRegistry) *MCPToolApp {
	return &MCPToolApp{store: store, dynReg: dynReg}
}

type MCPToolCreateInput struct {
	Name        string
	Description string
	Protocol    string
	Endpoint    string
	Config      string
	CreatedBy   string
}

func (a *MCPToolApp) Create(ctx context.Context, input *MCPToolCreateInput) (UserMCPTool, error) {
	data, err := a.store.Load(ctx)
	if err != nil {
		return UserMCPTool{}, err
	}

	tool := UserMCPTool{
		ID:          uuid.New().String(),
		Name:        input.Name,
		Description: input.Description,
		Transport:   input.Protocol,
		EndpointURL: input.Endpoint,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
		CreatedBy:   input.CreatedBy,
	}

	if input.Config != "" {
		var cfg map[string]any
		if jsonErr := json.Unmarshal([]byte(input.Config), &cfg); jsonErr == nil {
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
	if tool.ToolName == "" {
		tool.ToolName = tool.Name
	}

	data.Tools = append(data.Tools, tool)
	if err = a.store.Save(ctx, data); err != nil {
		return UserMCPTool{}, err
	}

	return tool, nil
}

type MCPToolListInput struct {
	Status string
}

func (a *MCPToolApp) List(ctx context.Context, input *MCPToolListInput) ([]interface{}, error) {
	data, err := a.store.Load(ctx)
	if err != nil {
		return nil, err
	}

	var items []interface{}
	for _, t := range data.Tools {
		if input.Status != "" && t.Status != input.Status {
			continue
		}
		sanitized := t
		if sanitized.AuthToken != "" {
			sanitized.AuthToken = "***REDACTED***"
		}
		items = append(items, sanitized)
	}
	return items, nil
}

type MCPToolUpdateInput struct {
	ToolID      string
	Name        string
	Description string
	Endpoint    string
	Protocol    string
	Config      string
}

func (a *MCPToolApp) Update(ctx context.Context, input *MCPToolUpdateInput) (UserMCPTool, error) {
	data, err := a.store.Load(ctx)
	if err != nil {
		return UserMCPTool{}, err
	}

	for i, t := range data.Tools {
		if t.ID != input.ToolID {
			continue
		}
		if input.Name != "" {
			data.Tools[i].Name = input.Name
		}
		if input.Description != "" {
			data.Tools[i].Description = input.Description
		}
		if input.Endpoint != "" {
			data.Tools[i].EndpointURL = input.Endpoint
		}
		if input.Protocol != "" {
			data.Tools[i].Transport = input.Protocol
		}
		if input.Config != "" {
			var cfg map[string]any
			if jsonErr := json.Unmarshal([]byte(input.Config), &cfg); jsonErr == nil {
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
		if err = a.store.Save(ctx, data); err != nil {
			return UserMCPTool{}, err
		}
		return data.Tools[i], nil
	}

	return UserMCPTool{}, gerror.Newf("tool %q not found", input.ToolID)
}

func (a *MCPToolApp) Delete(ctx context.Context, toolID string) error {
	data, err := a.store.Load(ctx)
	if err != nil {
		return err
	}

	found := false
	var filtered []UserMCPTool
	for _, t := range data.Tools {
		if t.ID == toolID {
			found = true
			continue
		}
		filtered = append(filtered, t)
	}
	if !found {
		return gerror.Newf("tool %q not found", toolID)
	}

	data.Tools = filtered
	if err = a.store.Save(ctx, data); err != nil {
		return err
	}

	a.dynReg.Unregister(toolID)
	return nil
}

func (a *MCPToolApp) Test(ctx context.Context, toolID string) (bool, string) {
	data, loadErr := a.store.Load(ctx)
	if loadErr != nil {
		return false, loadErr.Error()
	}

	var tool UserMCPTool
	var found bool
	for _, t := range data.Tools {
		if t.ID == toolID {
			tool = t
			found = true
			break
		}
	}
	if !found {
		return false, gerror.Newf("tool %q not found", toolID).Error()
	}

	if regErr := a.dynReg.Register(ctx, tool); regErr != nil {
		return false, regErr.Error()
	}

	a.dynReg.Unregister(toolID)
	return true, ""
}

func (a *MCPToolApp) Approve(ctx context.Context, toolID, approver string) error {
	data, err := a.store.Load(ctx)
	if err != nil {
		return err
	}

	for i, t := range data.Tools {
		if t.ID != toolID {
			continue
		}
		now := time.Now()
		data.Tools[i].Status = StatusApproved
		data.Tools[i].ApprovedAt = &now
		data.Tools[i].ApprovedBy = approver

		if err = a.store.Save(ctx, data); err != nil {
			return err
		}

		if regErr := a.dynReg.Register(ctx, data.Tools[i]); regErr != nil {
			g.Log().Warningf(ctx, "failed to register approved tool %q: %v", toolID, regErr)
		}
		return nil
	}

	return gerror.Newf("tool %q not found", toolID)
}

func (a *MCPToolApp) Reject(ctx context.Context, toolID string) error {
	data, err := a.store.Load(ctx)
	if err != nil {
		return err
	}

	for i, t := range data.Tools {
		if t.ID != toolID {
			continue
		}
		data.Tools[i].Status = StatusRejected
		if err = a.store.Save(ctx, data); err != nil {
			return err
		}
		return nil
	}

	return gerror.Newf("tool %q not found", toolID)
}
