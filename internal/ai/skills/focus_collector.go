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
		skill, err := registry.Resolve(task)
		if err != nil || skill == nil {
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
