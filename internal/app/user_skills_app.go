package app

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

type UserSkillApp struct {
	store  UserSkillStore
	loader *UserSkillLoader
}

func NewUserSkillApp(store UserSkillStore, loader *UserSkillLoader) *UserSkillApp {
	return &UserSkillApp{store: store, loader: loader}
}

type UserSkillCreateInput struct {
	Name        string
	Description string
	Domain      string
	CreatedBy   string
}

func (a *UserSkillApp) Create(ctx context.Context, input *UserSkillCreateInput) (UserSkill, error) {
	data, err := a.store.Load(ctx)
	if err != nil {
		return UserSkill{}, err
	}

	for _, s := range data.Skills {
		if s.Name == input.Name {
			return UserSkill{}, gerror.Newf("skill name %q already exists", input.Name)
		}
	}

	skill := UserSkill{
		ID:          uuid.New().String(),
		Name:        input.Name,
		Description: input.Description,
		Domain:      input.Domain,
		Status:      StatusApproved,
		CreatedAt:   time.Now(),
		CreatedBy:   input.CreatedBy,
	}

	data.Skills = append(data.Skills, skill)
	if err = a.store.Save(ctx, data); err != nil {
		return UserSkill{}, err
	}

	return skill, nil
}

type UserSkillListInput struct {
	Status string
}

func (a *UserSkillApp) List(ctx context.Context, input *UserSkillListInput) ([]interface{}, error) {
	data, err := a.store.Load(ctx)
	if err != nil {
		return nil, err
	}

	var items []interface{}
	for _, s := range data.Skills {
		if input.Status != "" && s.Status != input.Status {
			continue
		}
		items = append(items, s)
	}
	return items, nil
}

type UserSkillUpdateInput struct {
	SkillID     string
	Name        string
	Description string
	Content     string
	Domain      string
}

func (a *UserSkillApp) Update(ctx context.Context, input *UserSkillUpdateInput) (UserSkill, error) {
	data, err := a.store.Load(ctx)
	if err != nil {
		return UserSkill{}, err
	}

	for i, s := range data.Skills {
		if s.ID != input.SkillID {
			continue
		}
		if input.Name != "" {
			for _, other := range data.Skills {
				if other.ID != input.SkillID && other.Name == input.Name {
					return UserSkill{}, gerror.Newf("skill name %q already exists", input.Name)
				}
			}
			data.Skills[i].Name = input.Name
		}
		if input.Description != "" {
			data.Skills[i].Description = input.Description
		}
		if input.Content != "" {
			data.Skills[i].Focus = input.Content
		}
		if input.Domain != "" {
			data.Skills[i].Domain = input.Domain
		}
		if err = a.store.Save(ctx, data); err != nil {
			return UserSkill{}, err
		}
		return data.Skills[i], nil
	}

	return UserSkill{}, gerror.Newf("skill %q not found", input.SkillID)
}

func (a *UserSkillApp) Delete(ctx context.Context, skillID string) error {
	data, err := a.store.Load(ctx)
	if err != nil {
		return err
	}

	found := false
	var filtered []UserSkill
	for _, s := range data.Skills {
		if s.ID == skillID {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}
	if !found {
		return gerror.Newf("skill %q not found", skillID)
	}

	data.Skills = filtered
	if err = a.store.Save(ctx, data); err != nil {
		return err
	}

	if reloadErr := a.loader.Reload(ctx); reloadErr != nil {
		g.Log().Warningf(ctx, "hot reload after skill delete failed: %v", reloadErr)
	}

	return nil
}

func (a *UserSkillApp) Approve(ctx context.Context, skillID, approver string) error {
	data, err := a.store.Load(ctx)
	if err != nil {
		return err
	}

	for i, s := range data.Skills {
		if s.ID != skillID {
			continue
		}
		now := time.Now()
		data.Skills[i].Status = StatusApproved
		data.Skills[i].ApprovedAt = &now
		data.Skills[i].ApprovedBy = approver

		if err = a.store.Save(ctx, data); err != nil {
			return err
		}

		if reloadErr := a.loader.Reload(ctx); reloadErr != nil {
			g.Log().Warningf(ctx, "hot reload after skill approve failed: %v", reloadErr)
		}
		return nil
	}

	return gerror.Newf("skill %q not found", skillID)
}

func (a *UserSkillApp) Reject(ctx context.Context, skillID string) error {
	data, err := a.store.Load(ctx)
	if err != nil {
		return err
	}

	for i, s := range data.Skills {
		if s.ID != skillID {
			continue
		}
		data.Skills[i].Status = StatusRejected
		if err = a.store.Save(ctx, data); err != nil {
			return err
		}
		return nil
	}

	return gerror.Newf("skill %q not found", skillID)
}
