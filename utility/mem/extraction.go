package mem

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type MemoryCandidate struct {
	Type    MemoryType
	Content string
	Source  string
}

type DroppedMemoryCandidate struct {
	Candidate MemoryCandidate
	Reason    string
}

type MemoryExtractionReport struct {
	Candidates []MemoryCandidate
	StoredIDs  []string
	Dropped    []DroppedMemoryCandidate
}

func ExtractMemories(ctx context.Context, sessionID string, userMsg string, assistantMsg string) {
	_ = ExtractMemoriesWithReport(ctx, sessionID, userMsg, assistantMsg)
}

func ExtractMemoriesWithReport(ctx context.Context, sessionID string, userMsg string, assistantMsg string) *MemoryExtractionReport {
	if ctx != nil && ctx.Err() != nil {
		return &MemoryExtractionReport{}
	}
	ltm := GetLongTermMemory()
	report := &MemoryExtractionReport{}

	for _, candidate := range ExtractMemoryCandidates(userMsg, assistantMsg) {
		if ctx != nil && ctx.Err() != nil {
			return report
		}
		report.Candidates = append(report.Candidates, candidate)
		if ok, reason := ValidateMemoryCandidate(candidate); !ok {
			report.Dropped = append(report.Dropped, DroppedMemoryCandidate{
				Candidate: candidate,
				Reason:    reason,
			})
			continue
		}
		id := ltm.Store(ctx, sessionID, candidate.Type, candidate.Content, candidate.Source)
		report.StoredIDs = append(report.StoredIDs, id)
	}
	return report
}

func ExtractMemoryCandidates(userMsg string, assistantMsg string) []MemoryCandidate {
	var candidates []MemoryCandidate

	facts := extractFacts(userMsg, assistantMsg)
	for _, fact := range facts {
		candidates = append(candidates, MemoryCandidate{
			Type:    MemoryTypeFact,
			Content: fact,
			Source:  "conversation",
		})
	}

	prefs := extractPreferences(userMsg)
	for _, pref := range prefs {
		candidates = append(candidates, MemoryCandidate{
			Type:    MemoryTypePreference,
			Content: pref,
			Source:  "user_input",
		})
	}

	if len(assistantMsg) > 100 {
		episode := fmt.Sprintf("用户问: %s -> 系统回答了关于此问题的详细信息",
			truncate(userMsg, 100))
		candidates = append(candidates, MemoryCandidate{
			Type:    MemoryTypeEpisode,
			Content: episode,
			Source:  "conversation",
		})
	}

	return candidates
}

func ValidateMemoryCandidate(candidate MemoryCandidate) (bool, string) {
	content := strings.TrimSpace(candidate.Content)
	if content == "" {
		return false, "empty"
	}
	if len(content) < 4 {
		return false, "too_short"
	}
	if len(content) > 500 {
		return false, "too_long"
	}
	if strings.Count(content, "\n") > 3 {
		return false, "too_many_lines"
	}
	if strings.Contains(content, "```") {
		return false, "contains_code_block"
	}
	lower := strings.ToLower(content)
	for _, marker := range []string{
		"作为ai", "作为一个ai", "抱歉", "对不起", "请提供更多信息", "无法直接",
	} {
		if strings.Contains(lower, marker) {
			return false, "assistant_boilerplate"
		}
	}
	return true, ""
}

func BuildEnrichedContext(ctx context.Context, sessionID string, query string, shortTermMsgs []*schema.Message) []*schema.Message {
	ltm := GetLongTermMemory()
	memories := ltm.Retrieve(ctx, sessionID, query, 5)

	var result []*schema.Message

	if len(memories) > 0 {
		var memParts []string
		for _, m := range memories {
			memParts = append(memParts, fmt.Sprintf("- [%s] %s", m.Type, m.Content))
		}
		memoryContext := strings.Join(memParts, "\n")
		result = append(result, &schema.Message{
			Role:    schema.User,
			Content: fmt.Sprintf("[关键记忆]\n%s", memoryContext),
		})
		result = append(result, schema.AssistantMessage("好的，我已了解这些背景信息。", nil))
		g.Log().Debugf(ctx, "[memory] Injected %d long-term memories for session %s", len(memories), sessionID)
	}

	result = append(result, shortTermMsgs...)
	return result
}

func extractFacts(userMsg string, assistantMsg string) []string {
	var facts []string

	indicators := []string{
		"服务名", "应用名", "IP地址", "端口", "数据库名", "集群名",
		"版本号", "负责人", "告警规则", "阈值", "SLA", "域名",
	}
	combined := userMsg + " " + assistantMsg
	for _, indicator := range indicators {
		if strings.Contains(combined, indicator) {
			sentences := splitSentences(combined)
			for _, s := range sentences {
				if strings.Contains(s, indicator) && len(s) > 5 && len(s) < 500 {
					facts = append(facts, strings.TrimSpace(s))
				}
			}
		}
	}
	return dedup(facts)
}

func extractPreferences(userMsg string) []string {
	var prefs []string
	prefIndicators := []string{
		"我喜欢", "我希望", "请用", "不要用", "我倾向", "我习惯",
		"帮我", "我需要", "每次都", "总是",
	}
	for _, indicator := range prefIndicators {
		if strings.Contains(userMsg, indicator) {
			sentences := splitSentences(userMsg)
			for _, s := range sentences {
				if strings.Contains(s, indicator) && len(s) > 3 && len(s) < 300 {
					prefs = append(prefs, strings.TrimSpace(s))
				}
			}
		}
	}
	return dedup(prefs)
}

func splitSentences(text string) []string {
	var result []string
	separators := []string{"。", "；", "！", "？", "\n", ". ", "; "}
	parts := []string{text}
	for _, sep := range separators {
		var newParts []string
		for _, p := range parts {
			splits := strings.Split(p, sep)
			newParts = append(newParts, splits...)
		}
		parts = newParts
	}
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func dedup(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
