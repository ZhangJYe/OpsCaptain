package skills

import (
	"context"
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

type Skill interface {
	Name() string
	Description() string
	Match(task *protocol.TaskEnvelope) bool
	Run(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error)
}

type Execution struct {
	Skill  Skill
	Result *protocol.TaskResult
}

type Registry struct {
	domain string
	skills []Skill
}

func NewRegistry(domain string, skills ...Skill) (*Registry, error) {
	r := &Registry{domain: strings.TrimSpace(domain)}
	for _, skill := range skills {
		if err := r.Register(skill); err != nil {
			return nil, err
		}
	}
	if len(r.skills) == 0 {
		return nil, fmt.Errorf("skills registry %q has no skills", domain)
	}
	return r, nil
}

func (r *Registry) Register(skill Skill) error {
	if skill == nil {
		return fmt.Errorf("skill is nil")
	}
	name := strings.TrimSpace(skill.Name())
	if name == "" {
		return fmt.Errorf("skill name is empty")
	}
	for _, existing := range r.skills {
		if existing.Name() == name {
			return fmt.Errorf("skill %q already registered", name)
		}
	}
	r.skills = append(r.skills, skill)
	return nil
}

func (r *Registry) Resolve(task *protocol.TaskEnvelope) (Skill, error) {
	if r == nil || len(r.skills) == 0 {
		return nil, fmt.Errorf("skills registry is empty")
	}
	for _, skill := range r.skills {
		if skill.Match(task) {
			return skill, nil
		}
	}
	return r.skills[0], nil
}

func (r *Registry) Execute(ctx context.Context, task *protocol.TaskEnvelope) (*Execution, error) {
	skill, err := r.Resolve(task)
	if err != nil {
		return nil, err
	}
	result, err := skill.Run(ctx, task)
	if err != nil {
		return nil, err
	}
	result = AttachMetadata(result, r.domain, skill)
	return &Execution{
		Skill:  skill,
		Result: result,
	}, nil
}

func (r *Registry) SkillNames() []string {
	if r == nil || len(r.skills) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.skills))
	for _, skill := range r.skills {
		names = append(names, skill.Name())
	}
	return names
}

func AttachMetadata(result *protocol.TaskResult, domain string, skill Skill) *protocol.TaskResult {
	if result == nil || skill == nil {
		return result
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]any, 3)
	}
	if strings.TrimSpace(domain) != "" {
		result.Metadata["skill_domain"] = domain
	}
	result.Metadata["skill_name"] = skill.Name()
	result.Metadata["skill_description"] = skill.Description()
	return result
}

func PrefixedCapabilities(base []string, skillNames []string) []string {
	out := make([]string, 0, len(base)+len(skillNames))
	out = append(out, base...)
	for _, name := range skillNames {
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, "skill:"+name)
	}
	return out
}

func ContainsAny(goal string, keywords ...string) bool {
	goal = strings.ToLower(strings.TrimSpace(goal))
	for _, keyword := range keywords {
		if keyword == "" {
			continue
		}
		if strings.Contains(goal, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}
