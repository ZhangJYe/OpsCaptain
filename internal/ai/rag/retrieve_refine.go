package rag

import (
	"sort"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/schema"
)

var retrievalStopwords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "by": {},
	"for": {}, "from": {}, "in": {}, "into": {}, "is": {}, "of": {}, "on": {}, "or": {},
	"the": {}, "to": {}, "with": {}, "without": {}, "service": {}, "instance": {}, "type": {},
}

type retrievalQueryProfile struct {
	rawLower string
	tokens   map[string]struct{}
}

type retrievalDocProfile struct {
	contentTokens   map[string]struct{}
	service         string
	instanceType    string
	source          string
	destination     string
	serviceTokens   map[string]struct{}
	podTokens       map[string]struct{}
	nodeTokens      map[string]struct{}
	namespaceTokens map[string]struct{}
	metricNames     map[string]struct{}
	traceServices   map[string]struct{}
	traceOperations map[string]struct{}
}

type scoredDocument struct {
	doc   *schema.Document
	score int
	idx   int
}

func refineRetrievedDocs(query string, docs []*schema.Document) []*schema.Document {
	if len(docs) <= 1 {
		return docs
	}

	profile := buildRetrievalQueryProfile(query)
	if len(profile.tokens) == 0 && strings.TrimSpace(profile.rawLower) == "" {
		return docs
	}

	scored := make([]scoredDocument, 0, len(docs))
	for idx, doc := range docs {
		scored = append(scored, scoredDocument{
			doc:   doc,
			score: scoreRetrievedDocument(profile, doc, idx, len(docs)),
			idx:   idx,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].idx < scored[j].idx
		}
		return scored[i].score > scored[j].score
	})

	out := make([]*schema.Document, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.doc)
	}
	return out
}

func trimRetrievedDocs(docs []*schema.Document, topK int) []*schema.Document {
	if topK <= 0 || len(docs) <= topK {
		return docs
	}
	return docs[:topK]
}

func scoreRetrievedDocument(query retrievalQueryProfile, doc *schema.Document, idx, total int) int {
	score := (total - idx) * 2
	if doc == nil {
		return score
	}

	profile := buildRetrievalDocProfile(doc)

	score += overlapScore(query.tokens, profile.contentTokens, 1, 6)
	score += overlapScore(query.tokens, profile.metricNames, 3, 9)
	score += overlapScore(query.tokens, profile.traceOperations, 3, 9)
	score += overlapScore(query.tokens, profile.traceServices, 3, 6)
	score += overlapScore(query.tokens, profile.serviceTokens, 4, 12)
	score += overlapScore(query.tokens, profile.podTokens, 4, 12)
	score += overlapScore(query.tokens, profile.nodeTokens, 4, 12)
	score += overlapScore(query.tokens, profile.namespaceTokens, 2, 4)

	score += exactFieldBoost(query.rawLower, profile.service, 8)
	score += exactFieldBoost(query.rawLower, profile.instanceType, 5)
	score += exactFieldBoost(query.rawLower, profile.source, 6)
	score += exactFieldBoost(query.rawLower, profile.destination, 6)

	return score
}

func buildRetrievalQueryProfile(query string) retrievalQueryProfile {
	return retrievalQueryProfile{
		rawLower: strings.ToLower(strings.TrimSpace(query)),
		tokens:   tokenizeToSet(query),
	}
}

func buildRetrievalDocProfile(doc *schema.Document) retrievalDocProfile {
	meta := map[string]any{}
	if doc != nil && doc.MetaData != nil {
		meta = doc.MetaData
	}

	return retrievalDocProfile{
		contentTokens:   tokenizeToSet(documentContent(doc)),
		service:         normalizeValue(stringMetadata(meta, "service")),
		instanceType:    normalizeValue(stringMetadata(meta, "instance_type")),
		source:          normalizeValue(stringMetadata(meta, "source")),
		destination:     normalizeValue(stringMetadata(meta, "destination")),
		serviceTokens:   mergeTokenSets(tokenizeToSet(stringMetadata(meta, "service")), anySliceToSet(meta["service_tokens"])),
		podTokens:       anySliceToSet(meta["pod_tokens"]),
		nodeTokens:      anySliceToSet(meta["node_tokens"]),
		namespaceTokens: anySliceToSet(meta["namespace_tokens"]),
		metricNames:     anySliceToSet(meta["metric_names"]),
		traceServices:   anySliceToSet(meta["trace_services"]),
		traceOperations: anySliceToSet(meta["trace_operations"]),
	}
}

func documentContent(doc *schema.Document) string {
	if doc == nil {
		return ""
	}
	return doc.Content
}

func stringMetadata(meta map[string]any, key string) string {
	if meta == nil {
		return ""
	}
	raw, ok := meta[key]
	if !ok || raw == nil {
		return ""
	}
	if value, ok := raw.(string); ok {
		return value
	}
	return ""
}

func anySliceToSet(value any) map[string]struct{} {
	out := map[string]struct{}{}
	switch items := value.(type) {
	case []string:
		for _, item := range items {
			for token := range tokenizeToSet(item) {
				out[token] = struct{}{}
			}
		}
	case []any:
		for _, item := range items {
			if s, ok := item.(string); ok {
				for token := range tokenizeToSet(s) {
					out[token] = struct{}{}
				}
			}
		}
	}
	return out
}

func tokenizeToSet(value string) map[string]struct{} {
	out := map[string]struct{}{}
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		token := normalizeValue(b.String())
		b.Reset()
		if len(token) < 2 {
			return
		}
		if _, stop := retrievalStopwords[token]; stop {
			return
		}
		out[token] = struct{}{}
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == '/' || r == ':' {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}

func mergeTokenSets(sets ...map[string]struct{}) map[string]struct{} {
	out := map[string]struct{}{}
	for _, set := range sets {
		for token := range set {
			out[token] = struct{}{}
		}
	}
	return out
}

func overlapScore(queryTokens, docTokens map[string]struct{}, weight, capScore int) int {
	if len(queryTokens) == 0 || len(docTokens) == 0 || weight <= 0 || capScore <= 0 {
		return 0
	}
	score := 0
	for token := range queryTokens {
		if _, ok := docTokens[token]; ok {
			score += weight
			if score >= capScore {
				return capScore
			}
		}
	}
	return score
}

func exactFieldBoost(queryLower, value string, weight int) int {
	if queryLower == "" || value == "" || weight <= 0 {
		return 0
	}
	if strings.Contains(queryLower, value) {
		return weight
	}
	return 0
}

func normalizeValue(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
