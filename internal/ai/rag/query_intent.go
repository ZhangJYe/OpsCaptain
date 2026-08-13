package rag

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

type QueryIntent struct {
	PositiveTerms  []string `json:"positive_terms,omitempty"`
	ExcludedTerms  []string `json:"excluded_terms,omitempty"`
	Rule           string   `json:"rule,omitempty"`
	ContrastClause string   `json:"contrast_clause,omitempty"`
}

type QueryIntentTrace struct {
	Parsed        bool     `json:"parsed"`
	Applied       bool     `json:"applied"`
	Rule          string   `json:"rule,omitempty"`
	PositiveTerms []string `json:"positive_terms,omitempty"`
	ExcludedTerms []string `json:"excluded_terms,omitempty"`
	PenalizedDocs int      `json:"penalized_docs"`
}

type intentDocumentScore struct {
	positiveHits []string
	excludedHits []string
	bonus        int
	penalty      int
}

var intentCueTerms = map[string]struct{}{
	"了解": {}, "查询": {}, "讨论": {}, "只想": {}, "只查": {}, "只讨论": {},
	"我只": {}, "想看": {}, "我想": {}, "暂时": {}, "please": {}, "show": {},
}

func ParseQueryIntent(query string, cfg HybridConfig) QueryIntent {
	query = strings.TrimSpace(query)
	if query == "" || len(cfg.IntentConnectors) == 0 {
		return QueryIntent{}
	}

	type connectorMatch struct {
		connector string
		position  int
	}
	lower := strings.ToLower(query)
	matches := make([]connectorMatch, 0, len(cfg.IntentConnectors))
	for _, raw := range cfg.IntentConnectors {
		connector := strings.ToLower(strings.TrimSpace(raw))
		if connector == "" {
			continue
		}
		if position := strings.Index(lower, connector); position >= 0 {
			matches = append(matches, connectorMatch{connector: connector, position: position})
		}
	}
	if len(matches) == 0 {
		return QueryIntent{}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].position == matches[j].position {
			return len(matches[i].connector) > len(matches[j].connector)
		}
		return matches[i].position < matches[j].position
	})
	chosen := matches[0]
	left := strings.TrimSpace(query[:chosen.position])
	right := strings.TrimSpace(query[chosen.position+len(chosen.connector):])
	if left == "" || right == "" || containsIntentConnector(left, cfg.IntentConnectors) || containsIntentConnector(right, cfg.IntentConnectors) {
		return QueryIntent{}
	}

	left = trimIntentCue(left)
	right = trimIntentCue(right)
	positive := intentTerms(left, cfg.IntentMaxTerms)
	excluded := intentTerms(right, cfg.IntentMaxTerms)
	positive, excluded = removeSharedIntentTerms(positive, excluded)
	if len(positive) == 0 || len(excluded) == 0 {
		return QueryIntent{}
	}
	return QueryIntent{
		PositiveTerms:  positive,
		ExcludedTerms:  excluded,
		Rule:           "contrast:" + chosen.connector,
		ContrastClause: strings.TrimSpace(right),
	}
}

func removeSharedIntentTerms(positive, excluded []string) ([]string, []string) {
	positiveSet := make(map[string]struct{}, len(positive))
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, term := range positive {
		positiveSet[term] = struct{}{}
	}
	for _, term := range excluded {
		excludedSet[term] = struct{}{}
	}
	filteredPositive := positive[:0]
	for _, term := range positive {
		if _, shared := excludedSet[term]; !shared {
			filteredPositive = append(filteredPositive, term)
		}
	}
	filteredExcluded := excluded[:0]
	for _, term := range excluded {
		if _, shared := positiveSet[term]; !shared {
			filteredExcluded = append(filteredExcluded, term)
		}
	}
	return filteredPositive, filteredExcluded
}

func containsIntentConnector(value string, connectors []string) bool {
	lower := strings.ToLower(value)
	for _, connector := range connectors {
		if connector = strings.ToLower(strings.TrimSpace(connector)); connector != "" && strings.Contains(lower, connector) {
			return true
		}
	}
	return false
}

func trimIntentCue(value string) string {
	value = strings.Trim(value, " \t\r\n，。！？；：,.!?;:")
	for _, prefix := range []string{"我只想", "我只查", "我只讨论", "只想", "只查", "只讨论", "我想", "想看", "please show"} {
		value = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(value), prefix))
	}
	for _, suffix := range []string{"暂时", "一下"} {
		value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
	}
	return strings.Trim(value, " \t\r\n，。！？；：,.!?;:")
}

func intentTerms(value string, maxTerms int) []string {
	if maxTerms <= 0 {
		return nil
	}
	tokens := BM25Tokenize(value)
	out := make([]string, 0, minInt(maxTerms, len(tokens)))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		token = normalizeValue(token)
		if token == "" || utf8.RuneCountInString(token) == 1 {
			continue
		}
		if _, stop := RetrievalStopwords[token]; stop {
			continue
		}
		if _, cue := intentCueTerms[token]; cue {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		out = append(out, token)
		if len(out) == maxTerms {
			break
		}
	}
	return out
}

func scoreDocumentIntent(intent QueryIntent, doc *schema.Document, cfg HybridConfig) intentDocumentScore {
	if intent.Rule == "" || doc == nil {
		return intentDocumentScore{}
	}
	tokens := intentDocumentTokens(doc)
	result := intentDocumentScore{
		positiveHits: matchingTerms(intent.PositiveTerms, tokens),
		excludedHits: matchingTerms(intent.ExcludedTerms, tokens),
	}
	if len(result.positiveHits) > 0 {
		result.bonus = minInt(cfg.IntentPositiveBonus, len(result.positiveHits))
	}
	if len(result.positiveHits) == 0 && len(result.excludedHits) > 0 {
		result.penalty = minInt(cfg.IntentExcludedPenalty, len(result.excludedHits)*2)
	}
	return result
}

func intentDocumentTokens(doc *schema.Document) map[string]struct{} {
	profile := buildRetrievalDocProfile(doc)
	values := []string{documentContent(doc), profile.documentID, profile.title, profile.provider}
	if doc != nil && doc.MetaData != nil {
		values = append(values, firstStringMetadata(doc.MetaData, "tags", "service", "source", "destination"))
	}
	tokens := make(map[string]struct{})
	for _, value := range values {
		for _, token := range BM25Tokenize(value) {
			if utf8.RuneCountInString(token) > 1 {
				tokens[normalizeValue(token)] = struct{}{}
			}
		}
	}
	for token := range documentCoverageTokens(doc) {
		tokens[token] = struct{}{}
	}
	return tokens
}

func matchingTerms(terms []string, tokens map[string]struct{}) []string {
	hits := make([]string, 0)
	for _, term := range terms {
		if _, ok := tokens[term]; ok {
			hits = append(hits, term)
		}
	}
	return hits
}
