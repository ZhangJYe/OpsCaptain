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

type selectedSkillsContextKey struct{}

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
	if skill := r.Match(task); skill != nil {
		return skill, nil
	}
	return r.skills[0], nil
}

func (r *Registry) Match(task *protocol.TaskEnvelope) Skill {
	if r == nil || len(r.skills) == 0 {
		return nil
	}
	for _, skill := range r.skills {
		if skill.Match(task) {
			return skill
		}
	}
	return nil
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

func (r *Registry) Domain() string {
	return r.domain
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

func (r *Registry) SkillByName(name string) Skill {
	if r == nil || len(r.skills) == 0 {
		return nil
	}
	target := strings.TrimSpace(name)
	if target == "" {
		return nil
	}
	for _, skill := range r.skills {
		if strings.EqualFold(skill.Name(), target) {
			return skill
		}
	}
	return nil
}

func WithSelectedSkillIDs(ctx context.Context, selectedSkillIDs []string) context.Context {
	normalized := normalizeSelectedSkillIDs(selectedSkillIDs)
	if len(normalized) == 0 {
		return ctx
	}
	return context.WithValue(ctx, selectedSkillsContextKey{}, normalized)
}

func SelectedSkillIDsFromContext(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	selected, _ := ctx.Value(selectedSkillsContextKey{}).([]string)
	return append([]string(nil), selected...)
}

func normalizeSelectedSkillIDs(selectedSkillIDs []string) []string {
	if len(selectedSkillIDs) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(selectedSkillIDs))
	normalized := make([]string, 0, len(selectedSkillIDs))
	for _, raw := range selectedSkillIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, id)
	}
	return normalized
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
