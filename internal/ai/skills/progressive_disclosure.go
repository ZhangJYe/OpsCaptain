package skills

import (
	"strings"

	"SuperBizAgent/internal/ai/protocol"

	"github.com/cloudwego/eino/components/tool"
)

type ToolTier int

const (
	TierAlwaysOn  ToolTier = 0
	TierSkillGate ToolTier = 1
	TierOnDemand  ToolTier = 2
)

type TieredTool struct {
	Tool    tool.BaseTool
	Tier    ToolTier
	Domains []string
}

type ProgressiveDisclosure struct {
	tools      []TieredTool
	registries []*Registry
}

func NewProgressiveDisclosure(registries []*Registry, tieredTools []TieredTool) *ProgressiveDisclosure {
	return &ProgressiveDisclosure{
		tools:      tieredTools,
		registries: registries,
	}
}

type DisclosureResult struct {
	Tools          []tool.BaseTool
	DisclosedTier  map[ToolTier]int
	MatchedSkills  []string
	MatchedDomains []string
	SelectedSkills []SelectedSkill
}

type SelectedSkill struct {
	Name        string
	Domain      string
	Description string
}

func (pd *ProgressiveDisclosure) Disclose(query string, selectedSkillIDs []string) DisclosureResult {
	result := DisclosureResult{
		DisclosedTier: make(map[ToolTier]int),
	}

	selectedSkills := pd.ResolveSelectedSkills(selectedSkillIDs)
	matchedDomains := mergeDomains(pd.matchDomains(query), domainsFromSelectedSkills(selectedSkills))
	result.SelectedSkills = selectedSkills
	result.MatchedDomains = matchedDomains
	result.MatchedSkills = matchedDomains

	for _, tt := range pd.tools {
		switch tt.Tier {
		case TierAlwaysOn:
			result.Tools = append(result.Tools, tt.Tool)
			result.DisclosedTier[TierAlwaysOn]++

		case TierSkillGate:
			if domainOverlap(matchedDomains, tt.Domains) {
				result.Tools = append(result.Tools, tt.Tool)
				result.DisclosedTier[TierSkillGate]++
			}
		}
	}

	return result
}

func (pd *ProgressiveDisclosure) ResolveSelectedSkills(selectedSkillIDs []string) []SelectedSkill {
	selectedSkillIDs = normalizeSelectedSkillIDs(selectedSkillIDs)
	if len(selectedSkillIDs) == 0 {
		return nil
	}
	selected := make([]SelectedSkill, 0, len(selectedSkillIDs))
	for _, id := range selectedSkillIDs {
		for _, reg := range pd.registries {
			if reg == nil {
				continue
			}
			skill := reg.SkillByName(id)
			if skill == nil {
				continue
			}
			selected = append(selected, SelectedSkill{
				Name:        skill.Name(),
				Domain:      reg.Domain(),
				Description: skill.Description(),
			})
			break
		}
	}
	return selected
}

func (pd *ProgressiveDisclosure) ExpandDomain(current []tool.BaseTool, domain string) []tool.BaseTool {
	expanded := make([]tool.BaseTool, len(current))
	copy(expanded, current)

	existing := make(map[string]bool)
	for _, t := range current {
		info, _ := t.Info(nil)
		existing[info.Name] = true
	}

	for _, tt := range pd.tools {
		if tt.Tier == TierOnDemand || tt.Tier == TierSkillGate {
			if containsDomain(tt.Domains, domain) {
				info, _ := tt.Tool.Info(nil)
				if !existing[info.Name] {
					expanded = append(expanded, tt.Tool)
					existing[info.Name] = true
				}
			}
		}
	}
	return expanded
}

func (pd *ProgressiveDisclosure) AllTools() []tool.BaseTool {
	all := make([]tool.BaseTool, 0, len(pd.tools))
	for _, tt := range pd.tools {
		all = append(all, tt.Tool)
	}
	return all
}

func (pd *ProgressiveDisclosure) matchDomains(query string) []string {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	task := &protocol.TaskEnvelope{Goal: query}
	var domains []string
	seen := make(map[string]bool)

	for _, reg := range pd.registries {
		if reg == nil {
			continue
		}
		skill := reg.Match(task)
		if skill == nil {
			continue
		}
		d := reg.Domain()
		if !seen[d] {
			seen[d] = true
			domains = append(domains, d)
		}
	}
	return domains
}

func domainOverlap(matched []string, toolDomains []string) bool {
	if len(toolDomains) == 0 {
		return true
	}
	for _, m := range matched {
		for _, d := range toolDomains {
			if strings.EqualFold(m, d) {
				return true
			}
		}
	}
	return false
}

func containsDomain(domains []string, target string) bool {
	for _, d := range domains {
		if strings.EqualFold(d, target) {
			return true
		}
	}
	return false
}

func domainsFromSelectedSkills(selected []SelectedSkill) []string {
	if len(selected) == 0 {
		return nil
	}
	domains := make([]string, 0, len(selected))
	for _, skill := range selected {
		if strings.TrimSpace(skill.Domain) == "" {
			continue
		}
		domains = append(domains, skill.Domain)
	}
	return mergeDomains(domains)
}

func mergeDomains(groups ...[]string) []string {
	var merged []string
	seen := make(map[string]bool)
	for _, group := range groups {
		for _, domain := range group {
			trimmed := strings.TrimSpace(domain)
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, trimmed)
		}
	}
	return merged
}
