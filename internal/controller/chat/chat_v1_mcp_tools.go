package chat

import (
	"context"

	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"

	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) MCPToolCreate(ctx context.Context, req *v1.MCPToolCreateReq) (res *v1.MCPToolCreateRes, err error) {
	tool, err := c.mcpToolApp.Create(ctx, &app.MCPToolCreateInput{
		Name:        req.Name,
		Description: req.Description,
		Protocol:    req.Protocol,
		Endpoint:    req.Endpoint,
		Config:      req.Config,
		CreatedBy:   g.RequestFromCtx(ctx).GetClientIp(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.MCPToolCreateRes{Tool: tool}, nil
}

func (c *ControllerV1) MCPToolList(ctx context.Context, req *v1.MCPToolListReq) (res *v1.MCPToolListRes, err error) {
	items, err := c.mcpToolApp.List(ctx, &app.MCPToolListInput{Status: req.Status})
	if err != nil {
		return nil, err
	}
	return &v1.MCPToolListRes{Items: items, Total: len(items)}, nil
}

func (c *ControllerV1) MCPToolUpdate(ctx context.Context, req *v1.MCPToolUpdateReq) (res *v1.MCPToolUpdateRes, err error) {
	tool, err := c.mcpToolApp.Update(ctx, &app.MCPToolUpdateInput{
		ToolID:      req.ToolId,
		Name:        req.Name,
		Description: req.Description,
		Endpoint:    req.Endpoint,
		Protocol:    req.Protocol,
		Config:      req.Config,
	})
	if err != nil {
		return nil, err
	}
	return &v1.MCPToolUpdateRes{Tool: tool}, nil
}

func (c *ControllerV1) MCPToolDelete(ctx context.Context, req *v1.MCPToolDeleteReq) (res *v1.MCPToolDeleteRes, err error) {
	if err := c.mcpToolApp.Delete(ctx, req.ToolId); err != nil {
		return nil, err
	}
	return &v1.MCPToolDeleteRes{Success: true}, nil
}

func (c *ControllerV1) MCPToolTest(ctx context.Context, req *v1.MCPToolTestReq) (res *v1.MCPToolTestRes, err error) {
	ok, errMsg := c.mcpToolApp.Test(ctx, req.ToolId)
	return &v1.MCPToolTestRes{Success: ok, Error: errMsg}, nil
}

func (c *ControllerV1) MCPToolApprove(ctx context.Context, req *v1.MCPToolApproveReq) (res *v1.MCPToolApproveRes, err error) {
	approver := g.RequestFromCtx(ctx).GetClientIp()
	if err := c.mcpToolApp.Approve(ctx, req.ToolId, approver); err != nil {
		return nil, err
	}
	return &v1.MCPToolApproveRes{Success: true}, nil
}

func (c *ControllerV1) MCPToolReject(ctx context.Context, req *v1.MCPToolRejectReq) (res *v1.MCPToolRejectRes, err error) {
	if err := c.mcpToolApp.Reject(ctx, req.ToolId); err != nil {
		return nil, err
	}
	return &v1.MCPToolRejectRes{Success: true}, nil
}
