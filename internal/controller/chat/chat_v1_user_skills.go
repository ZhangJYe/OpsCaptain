package chat

import (
	"context"
	"time"

	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/skills"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

// UserSkillCreate creates a new user skill with StatusApproved (no approval flow).
func (c *ControllerV1) UserSkillCreate(ctx context.Context, req *v1.UserSkillCreateReq) (res *v1.UserSkillCreateRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	// Check name uniqueness.
	for _, s := range data.Skills {
		if s.Name == req.Name {
			return nil, gerror.Newf("skill name %q already exists", req.Name)
		}
	}

	skill := skills.UserSkill{
		ID:          uuid.New().String(),
		Name:        req.Name,
		Description: req.Description,
		Domain:      req.Domain,
		Status:      skills.StatusApproved,
		CreatedAt:   time.Now(),
		CreatedBy:   g.RequestFromCtx(ctx).GetClientIp(),
	}

	data.Skills = append(data.Skills, skill)
	if err = c.userSkillStore.Save(ctx, data); err != nil {
		return nil, err
	}

	return &v1.UserSkillCreateRes{Skill: skill}, nil
}

// UserSkillList returns all user skills, optionally filtered by status.
func (c *ControllerV1) UserSkillList(ctx context.Context, req *v1.UserSkillListReq) (res *v1.UserSkillListRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	var items []interface{}
	for _, s := range data.Skills {
		if req.Status != "" && s.Status != req.Status {
			continue
		}
		items = append(items, s)
	}

	return &v1.UserSkillListRes{Items: items, Total: len(items)}, nil
}

// UserSkillUpdate updates fields of an existing user skill.
func (c *ControllerV1) UserSkillUpdate(ctx context.Context, req *v1.UserSkillUpdateReq) (res *v1.UserSkillUpdateRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i, s := range data.Skills {
		if s.ID != req.SkillId {
			continue
		}
		if req.Name != "" {
			// Check name uniqueness against other skills.
			for _, other := range data.Skills {
				if other.ID != req.SkillId && other.Name == req.Name {
					return nil, gerror.Newf("skill name %q already exists", req.Name)
				}
			}
			data.Skills[i].Name = req.Name
		}
		if req.Description != "" {
			data.Skills[i].Description = req.Description
		}
		if req.Content != "" {
			data.Skills[i].Focus = req.Content
		}
		if req.Domain != "" {
			data.Skills[i].Domain = req.Domain
		}
		if err = c.userSkillStore.Save(ctx, data); err != nil {
			return nil, err
		}
		return &v1.UserSkillUpdateRes{Skill: data.Skills[i]}, nil
	}

	return nil, gerror.Newf("skill %q not found", req.SkillId)
}

// UserSkillDelete deletes a skill by ID and triggers a hot reload of user skills.
func (c *ControllerV1) UserSkillDelete(ctx context.Context, req *v1.UserSkillDeleteReq) (res *v1.UserSkillDeleteRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	found := false
	var filtered []skills.UserSkill
	for _, s := range data.Skills {
		if s.ID == req.SkillId {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}
	if !found {
		return nil, gerror.Newf("skill %q not found", req.SkillId)
	}

	data.Skills = filtered
	if err = c.userSkillStore.Save(ctx, data); err != nil {
		return nil, err
	}

	// Hot reload to remove the skill from active registries.
	if reloadErr := c.userSkillLoader.Reload(ctx); reloadErr != nil {
		g.Log().Warningf(ctx, "hot reload after skill delete failed: %v", reloadErr)
	}

	return &v1.UserSkillDeleteRes{Success: true}, nil
}

// UserSkillApprove sets skill status to approved and triggers a hot reload.
func (c *ControllerV1) UserSkillApprove(ctx context.Context, req *v1.UserSkillApproveReq) (res *v1.UserSkillApproveRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i, s := range data.Skills {
		if s.ID != req.SkillId {
			continue
		}
		now := time.Now()
		approver := g.RequestFromCtx(ctx).GetClientIp()
		data.Skills[i].Status = skills.StatusApproved
		data.Skills[i].ApprovedAt = &now
		data.Skills[i].ApprovedBy = approver

		if err = c.userSkillStore.Save(ctx, data); err != nil {
			return nil, err
		}

		// Hot reload to register the approved skill.
		if reloadErr := c.userSkillLoader.Reload(ctx); reloadErr != nil {
			g.Log().Warningf(ctx, "hot reload after skill approve failed: %v", reloadErr)
		}
		return &v1.UserSkillApproveRes{Success: true}, nil
	}

	return nil, gerror.Newf("skill %q not found", req.SkillId)
}

// UserSkillReject sets skill status to rejected.
func (c *ControllerV1) UserSkillReject(ctx context.Context, req *v1.UserSkillRejectReq) (res *v1.UserSkillRejectRes, err error) {
	data, err := c.userSkillStore.Load(ctx)
	if err != nil {
		return nil, err
	}

	for i, s := range data.Skills {
		if s.ID != req.SkillId {
			continue
		}
		data.Skills[i].Status = skills.StatusRejected
		if err = c.userSkillStore.Save(ctx, data); err != nil {
			return nil, err
		}
		return &v1.UserSkillRejectRes{Success: true}, nil
	}

	return nil, gerror.Newf("skill %q not found", req.SkillId)
}
