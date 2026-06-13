package chat

import (
	"context"

	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"

	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) UserSkillCreate(ctx context.Context, req *v1.UserSkillCreateReq) (res *v1.UserSkillCreateRes, err error) {
	skill, err := c.userSkillApp.Create(ctx, &app.UserSkillCreateInput{
		Name:        req.Name,
		Description: req.Description,
		Domain:      req.Domain,
		CreatedBy:   g.RequestFromCtx(ctx).GetClientIp(),
	})
	if err != nil {
		return nil, err
	}
	return &v1.UserSkillCreateRes{Skill: skill}, nil
}

func (c *ControllerV1) UserSkillList(ctx context.Context, req *v1.UserSkillListReq) (res *v1.UserSkillListRes, err error) {
	items, err := c.userSkillApp.List(ctx, &app.UserSkillListInput{Status: req.Status})
	if err != nil {
		return nil, err
	}
	return &v1.UserSkillListRes{Items: items, Total: len(items)}, nil
}

func (c *ControllerV1) UserSkillUpdate(ctx context.Context, req *v1.UserSkillUpdateReq) (res *v1.UserSkillUpdateRes, err error) {
	skill, err := c.userSkillApp.Update(ctx, &app.UserSkillUpdateInput{
		SkillID:     req.SkillId,
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Domain:      req.Domain,
	})
	if err != nil {
		return nil, err
	}
	return &v1.UserSkillUpdateRes{Skill: skill}, nil
}

func (c *ControllerV1) UserSkillDelete(ctx context.Context, req *v1.UserSkillDeleteReq) (res *v1.UserSkillDeleteRes, err error) {
	if err := c.userSkillApp.Delete(ctx, req.SkillId); err != nil {
		return nil, err
	}
	return &v1.UserSkillDeleteRes{Success: true}, nil
}

func (c *ControllerV1) UserSkillApprove(ctx context.Context, req *v1.UserSkillApproveReq) (res *v1.UserSkillApproveRes, err error) {
	approver := g.RequestFromCtx(ctx).GetClientIp()
	if err := c.userSkillApp.Approve(ctx, req.SkillId, approver); err != nil {
		return nil, err
	}
	return &v1.UserSkillApproveRes{Success: true}, nil
}

func (c *ControllerV1) UserSkillReject(ctx context.Context, req *v1.UserSkillRejectReq) (res *v1.UserSkillRejectRes, err error) {
	if err := c.userSkillApp.Reject(ctx, req.SkillId); err != nil {
		return nil, err
	}
	return &v1.UserSkillRejectRes{Success: true}, nil
}
