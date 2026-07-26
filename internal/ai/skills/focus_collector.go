package skills

import (
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

type FocusHint struct {
	Domain      string
	SkillName   string
	Description string
	Focus       string
}

type FocusProvider interface {
	Name() string
	Description() string
	Focus() string
	Match(task *protocol.TaskEnvelope) bool
}

type FocusCollector struct {
	registries []*Registry
}

func NewFocusCollector(registries ...*Registry) *FocusCollector {
	return &FocusCollector{registries: registries}
}

func (c *FocusCollector) Collect(query string) []FocusHint {
	if strings.TrimSpace(query) == "" || len(c.registries) == 0 {
		return nil
	}

	task := &protocol.TaskEnvelope{Goal: query}
	var hints []FocusHint

	for _, registry := range c.registries {
		if registry == nil {
			continue
		}
		skill := registry.Match(task)
		if skill == nil {
			continue
		}

		fp, ok := skill.(FocusProvider)
		if !ok {
			continue
		}
		focus := strings.TrimSpace(fp.Focus())
		if focus == "" {
			continue
		}

		hints = append(hints, FocusHint{
			Domain:      registry.Domain(),
			SkillName:   skill.Name(),
			Description: skill.Description(),
			Focus:       focus,
		})
	}
	return hints
}

func (fc *FocusCollector) ResolveSelected(selectedSkillIDs []string) []FocusHint {
	if len(selectedSkillIDs) == 0 {
		return nil
	}
	var hints []FocusHint
	for _, id := range selectedSkillIDs {
		name := id
		if strings.HasPrefix(id, "user-skill:") {
			name = strings.TrimPrefix(id, "user-skill:")
		}
		for _, reg := range fc.registries {
			if reg == nil {
				continue
			}
			if skill := reg.SkillByName(name); skill != nil {
				if fp, ok := skill.(FocusProvider); ok {
					hints = append(hints, FocusHint{
						Domain:    reg.Domain(),
						SkillName: skill.Name(),
						Focus:     fp.Focus(),
					})
				}
				break
			}
		}
	}
	return hints
}

func FormatFocusHints(hints []FocusHint) string {
	if len(hints) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, h := range hints {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("- [")
		sb.WriteString(h.Domain)
		sb.WriteString("] ")
		sb.WriteString(h.Focus)
	}
	return sb.String()
}
