package knowledge

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/rag"
)

type parsedKnowledgeOutput struct {
	evidence      []protocol.EvidenceItem
	highlights    []string
	documentCount int
	degraded      bool
	message       string
}

func parseKnowledgeLookupOutput(output string) (parsedKnowledgeOutput, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return parsedKnowledgeOutput{}, fmt.Errorf("empty payload")
	}
	if strings.HasPrefix(trimmed, "[") {
		return parseLegacyKnowledgeDocs(trimmed)
	}
	if strings.HasPrefix(trimmed, "{") {
		return parseStructuredKnowledgeOutput(trimmed)
	}
	return parsedKnowledgeOutput{}, fmt.Errorf("unsupported payload shape")
}

func parseStructuredKnowledgeOutput(output string) (parsedKnowledgeOutput, error) {
	var payload rag.KnowledgeSearchOutput
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return parsedKnowledgeOutput{}, err
	}
	documentCount := len(payload.Citations)
	if documentCount == 0 {
		documentCount = len(payload.Evidence)
	}
	if !payload.Success || payload.Degraded {
		return parsedKnowledgeOutput{
			documentCount: documentCount,
			degraded:      true,
			message:       firstNonEmpty(payload.Message, payload.Error, payload.Answer),
		}, nil
	}
	evidence, highlights := evidenceFromKnowledgeSearchOutput(payload, knowledgeEvidenceLimit())
	return parsedKnowledgeOutput{
		evidence:      evidence,
		highlights:    highlights,
		documentCount: documentCount,
	}, nil
}

func parseLegacyKnowledgeDocs(output string) (parsedKnowledgeOutput, error) {
	var docs []map[string]any
	if err := json.Unmarshal([]byte(output), &docs); err != nil {
		return parsedKnowledgeOutput{}, err
	}
	limit := knowledgeEvidenceLimit()
	evidence := make([]protocol.EvidenceItem, 0, min(limit, len(docs)))
	highlights := make([]string, 0, min(limit, len(docs)))
	for idx, doc := range docs {
		if idx >= limit {
			break
		}
		content := firstNonEmptyString(doc, "content", "page_content", "text")
		title := firstNonEmptyString(doc, "id", "title", "source")
		if title == "" {
			title = fmt.Sprintf("doc-%d", idx+1)
		}
		snippet := shorten(content, 160)
		if snippet != "" {
			highlights = append(highlights, snippet)
		}
		evidence = append(evidence, protocol.EvidenceItem{
			SourceType: "knowledge",
			SourceID:   title,
			Title:      title,
			Snippet:    snippet,
			Score:      confidenceKnowledgeEvidence - float64(idx)*0.08,
		})
	}
	return parsedKnowledgeOutput{
		evidence:      evidence,
		highlights:    highlights,
		documentCount: len(docs),
	}, nil
}

func evidenceFromKnowledgeSearchOutput(payload rag.KnowledgeSearchOutput, limit int) ([]protocol.EvidenceItem, []string) {
	if limit <= 0 {
		return nil, nil
	}
	citations := make(map[string]rag.Citation, len(payload.Citations))
	for _, citation := range payload.Citations {
		citations[citation.ID] = citation
	}
	evidence := make([]protocol.EvidenceItem, 0, min(limit, max(len(payload.Evidence), len(payload.Citations))))
	highlights := make([]string, 0, limit)
	if len(payload.Evidence) > 0 {
		for idx, item := range payload.Evidence {
			if idx >= limit {
				break
			}
			citation := citations[item.CitationID]
			if citation.ID == "" {
				citation.ID = item.CitationID
			}
			ev := evidenceItemFromCitation(citation, item.Text, idx)
			evidence = append(evidence, ev)
			if ev.Snippet != "" {
				highlights = append(highlights, ev.Snippet)
			}
		}
		return evidence, highlights
	}
	for idx, citation := range payload.Citations {
		if idx >= limit {
			break
		}
		ev := evidenceItemFromCitation(citation, citation.Snippet, idx)
		evidence = append(evidence, ev)
		if ev.Snippet != "" {
			highlights = append(highlights, ev.Snippet)
		}
	}
	return evidence, highlights
}

func evidenceItemFromCitation(citation rag.Citation, text string, idx int) protocol.EvidenceItem {
	sourceID := firstNonEmpty(citation.ID, citation.Source, fmt.Sprintf("doc-%d", idx+1))
	title := firstNonEmpty(citation.Title, citation.ID, sourceID)
	snippet := shorten(firstNonEmpty(text, citation.Snippet), 160)
	score := citation.Score
	if score == 0 {
		score = confidenceKnowledgeEvidence - float64(idx)*0.08
	}
	return protocol.EvidenceItem{
		SourceType: "knowledge",
		SourceID:   sourceID,
		Title:      title,
		Snippet:    snippet,
		Score:      score,
		URI:        citation.Source,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

func shorten(input string, max int) string {
	input = strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if utf8.RuneCountInString(input) <= max {
		return input
	}
	runes := []rune(input)
	return string(runes[:max]) + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}