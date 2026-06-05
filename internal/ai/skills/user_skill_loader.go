package skills

import (
	"context"
	"fmt"
	"log"
)

type UserSkillLoader struct {
	store      UserSkillStore
	mcpReg     MCPInvoker
	metricsR   *Registry
	logsR      *Registry
	knowledgeR *Registry
	customR    *Registry
}

func NewUserSkillLoader(store UserSkillStore, mcpReg MCPInvoker,
	metricsR, logsR, knowledgeR, customR *Registry) *UserSkillLoader {
	return &UserSkillLoader{
		store: store, mcpReg: mcpReg,
		metricsR: metricsR, logsR: logsR, knowledgeR: knowledgeR, customR: customR,
	}
}

// Reload clears all user skills from registries and re-loads approved ones from store.
func (l *UserSkillLoader) Reload(ctx context.Context) error {
	data, err := l.store.Load(ctx)
	if err != nil {
		return fmt.Errorf("load user skills: %w", err)
	}
	l.clearUserSkills()
	for _, us := range data.Skills {
		if us.Status != StatusApproved {
			continue
		}
		gs := NewGenericSkill(us, l.mcpReg)
		reg := l.resolveDomain(us.Domain)
		if err := reg.Register(gs); err != nil {
			log.Printf("[user_skill_loader] failed to register skill %q: %v", us.Name, err)
		}
	}
	return nil
}

func (l *UserSkillLoader) clearUserSkills() {
	registries := []*Registry{l.metricsR, l.logsR, l.knowledgeR, l.customR}
	for _, reg := range registries {
		if reg == nil {
			continue
		}
		for _, name := range reg.SkillNames() {
			if skill := reg.SkillByName(name); skill != nil {
				if _, ok := skill.(*GenericSkill); ok {
					reg.Unregister(name)
				}
			}
		}
	}
}

func (l *UserSkillLoader) resolveDomain(domain string) *Registry {
	switch domain {
	case DomainMetrics:
		return l.metricsR
	case DomainLogs:
		return l.logsR
	case DomainKnowledge:
		return l.knowledgeR
	default:
		if l.customR != nil {
			return l.customR
		}
		r, _ := NewRegistry("custom", nil)
		return r
	}
}
