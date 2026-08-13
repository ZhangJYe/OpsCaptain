package rag

import (
	"sort"
	"strings"
	"unicode"

	"github.com/cloudwego/eino/schema"
)

var RetrievalStopwords = map[string]struct{}{
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
	documentID      string
	title           string
	provider        string
	titleTokens     map[string]struct{}
	tagTokens       map[string]struct{}
	providerTokens  map[string]struct{}
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
	doc          *schema.Document
	score        int
	fieldBoost   int
	fieldMatches []string
	intent       intentDocumentScore
	idx          int
}

func refineRetrievedDocs(query string, docs []*schema.Document) []*schema.Document {
	return refineRetrievedDocsWithConfig(query, docs, HybridConfig{})
}

func refineRetrievedDocsWithConfig(query string, docs []*schema.Document, cfg HybridConfig) []*schema.Document {
	docs, _ = refineRetrievedDocsWithTrace(query, docs, cfg)
	return docs
}

func refineRetrievedDocsWithTrace(query string, docs []*schema.Document, cfg HybridConfig) ([]*schema.Document, QueryIntentTrace) {
	intent := QueryIntent{}
	trace := QueryIntentTrace{}
	if cfg.IntentRefinementEnabled {
		intent = ParseQueryIntent(query, cfg)
		trace = QueryIntentTrace{
			Parsed: intent.Rule != "", Rule: intent.Rule,
			PositiveTerms: append([]string(nil), intent.PositiveTerms...),
			ExcludedTerms: append([]string(nil), intent.ExcludedTerms...),
		}
	}
	if len(docs) <= 1 {
		annotateFinalPositions(docs)
		return docs, trace
	}

	profile := buildRetrievalQueryProfile(query)
	if len(profile.tokens) == 0 && strings.TrimSpace(profile.rawLower) == "" {
		return docs, trace
	}

	scored := make([]scoredDocument, 0, len(docs))
	for idx, doc := range docs {
		fieldBoost, fieldMatches := knowledgeFieldScore(profile, doc, cfg)
		intentScore := scoreDocumentIntent(intent, doc, cfg)
		if intentScore.bonus > 0 || intentScore.penalty > 0 {
			trace.Applied = true
		}
		if intentScore.penalty > 0 {
			trace.PenalizedDocs++
		}
		scored = append(scored, scoredDocument{
			doc:          doc,
			score:        scoreRetrievedDocument(profile, doc, idx, len(docs)) + fieldBoost + intentScore.bonus - intentScore.penalty,
			fieldBoost:   fieldBoost,
			fieldMatches: fieldMatches,
			intent:       intentScore,
			idx:          idx,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].idx < scored[j].idx
		}
		return scored[i].score > scored[j].score
	})

	out := make([]*schema.Document, 0, len(scored))
	for position, item := range scored {
		ensureDocMeta(item.doc)
		item.doc.MetaData[metaKeyRefinePosition] = position + 1
		if item.fieldBoost > 0 {
			item.doc.MetaData[metaKeyFieldBoost] = float64(item.fieldBoost)
			item.doc.MetaData[metaKeyFieldMatches] = append([]string(nil), item.fieldMatches...)
		}
		if trace.Parsed {
			item.doc.MetaData[metaKeyIntentRule] = trace.Rule
			item.doc.MetaData[metaKeyIntentPositiveHits] = append([]string(nil), item.intent.positiveHits...)
			item.doc.MetaData[metaKeyIntentExcludedHits] = append([]string(nil), item.intent.excludedHits...)
			item.doc.MetaData[metaKeyIntentBonus] = float64(item.intent.bonus)
			item.doc.MetaData[metaKeyIntentPenalty] = float64(item.intent.penalty)
			item.doc.MetaData[metaKeyIntentNetScore] = float64(item.intent.bonus - item.intent.penalty)
		}
		out = append(out, item.doc)
	}
	return out, trace
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
		documentID:      normalizeValue(firstStringMetadata(meta, "knowledge_doc_id", "doc_id", "case_id")),
		title:           normalizeValue(firstStringMetadata(meta, "title", "file_name", "filename")),
		provider:        normalizeValue(stringMetadata(meta, "provider")),
		titleTokens:     tokenizeToSet(firstStringMetadata(meta, "title", "file_name", "filename")),
		tagTokens:       anySliceToSet(meta["tags"]),
		providerTokens:  tokenizeToSet(stringMetadata(meta, "provider")),
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

func knowledgeFieldScore(query retrievalQueryProfile, doc *schema.Document, cfg HybridConfig) (int, []string) {
	if !cfg.KnowledgeFieldBoostEnabled || doc == nil {
		return 0, nil
	}
	profile := buildRetrievalDocProfile(doc)
	score := 0
	matches := make([]string, 0, 4)
	if exactFieldBoost(query.rawLower, profile.documentID, cfg.KnowledgeDocIDBoost) > 0 {
		score += cfg.KnowledgeDocIDBoost
		matches = append(matches, "document_id")
	}
	titleOverlap := overlapCount(query.tokens, profile.titleTokens)
	titleBoost := minInt(cfg.KnowledgeTitleBoost, titleOverlap*2)
	if exactFieldBoost(query.rawLower, profile.title, cfg.KnowledgeTitleBoost) > 0 {
		titleBoost = cfg.KnowledgeTitleBoost
	}
	if titleBoost > 0 {
		score += titleBoost
		matches = append(matches, "title")
	}
	if overlapCount(query.tokens, profile.tagTokens) > 0 {
		score += minInt(cfg.KnowledgeTagsBoost, overlapCount(query.tokens, profile.tagTokens)*cfg.KnowledgeTagsBoost)
		matches = append(matches, "tags")
	}
	if overlapCount(query.tokens, profile.providerTokens) > 0 || exactFieldBoost(query.rawLower, profile.provider, cfg.KnowledgeProviderBoost) > 0 {
		score += cfg.KnowledgeProviderBoost
		matches = append(matches, "provider")
	}
	if cfg.KnowledgeFieldBoostCap > 0 && score > cfg.KnowledgeFieldBoostCap {
		score = cfg.KnowledgeFieldBoostCap
	}
	return score, matches
}

func selectFinalDocs(query string, docs []*schema.Document, cfg HybridConfig) []*schema.Document {
	topK := cfg.FinalTopK
	if topK <= 0 {
		topK = 10
	}
	if !cfg.CoverageEnabled || len(docs) <= 1 || len(tokenizeToSet(query)) < 2 {
		selected := trimRetrievedDocs(docs, topK)
		annotateFinalPositions(selected)
		return selected
	}

	remaining := append([]*schema.Document(nil), docs...)
	limit := minInt(topK, len(remaining))
	selected := make([]*schema.Document, 0, limit)
	covered := make(map[string]struct{})
	queryTokens := tokenizeToSet(query)
	for position := 0; position < limit; position++ {
		if position == 0 {
			selected = append(selected, remaining[0])
			addCoveredTokens(remaining[0], queryTokens, covered)
			remaining = remaining[1:]
			continue
		}
		maxIndex := minInt(cfg.CoverageMaxPositionGain, len(remaining)-1)
		bestIndex, bestGain := 0, newCoverageCount(remaining[0], queryTokens, covered)
		for candidateIndex := 1; candidateIndex <= maxIndex; candidateIndex++ {
			gain := newCoverageCount(remaining[candidateIndex], queryTokens, covered)
			if gain > bestGain {
				bestIndex, bestGain = candidateIndex, gain
			}
		}
		chosen := remaining[bestIndex]
		if bestIndex > 0 {
			ensureDocMeta(chosen)
			chosen.MetaData[metaKeyCoverageBoost] = float64(bestIndex)
		}
		selected = append(selected, chosen)
		addCoveredTokens(chosen, queryTokens, covered)
		remaining = append(remaining[:bestIndex], remaining[bestIndex+1:]...)
	}
	annotateFinalPositions(selected)
	return selected
}

func documentCoverageTokens(doc *schema.Document) map[string]struct{} {
	profile := buildRetrievalDocProfile(doc)
	return mergeTokenSets(profile.contentTokens, profile.titleTokens, profile.tagTokens, profile.providerTokens, tokenizeToSet(profile.documentID))
}

func newCoverageCount(doc *schema.Document, queryTokens, covered map[string]struct{}) int {
	count := 0
	for token := range documentCoverageTokens(doc) {
		if _, wanted := queryTokens[token]; !wanted {
			continue
		}
		if _, already := covered[token]; !already {
			count++
		}
	}
	return count
}

func addCoveredTokens(doc *schema.Document, queryTokens, covered map[string]struct{}) {
	for token := range documentCoverageTokens(doc) {
		if _, wanted := queryTokens[token]; wanted {
			covered[token] = struct{}{}
		}
	}
}

func annotateFinalPositions(docs []*schema.Document) {
	for position, doc := range docs {
		if doc == nil {
			continue
		}
		ensureDocMeta(doc)
		doc.MetaData[metaKeyFinalPosition] = position + 1
	}
}

func firstStringMetadata(meta map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringMetadata(meta, key); value != "" {
			return value
		}
	}
	return ""
}

func overlapCount(left, right map[string]struct{}) int {
	count := 0
	for token := range left {
		if _, ok := right[token]; ok {
			count++
		}
	}
	return count
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
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
		if _, stop := RetrievalStopwords[token]; stop {
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
