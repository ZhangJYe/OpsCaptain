package rag

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

const maxSnippetLen = 300

const (
	metaKeyDenseRank          = "_trace_dense_rank"
	metaKeyLexicalRank        = "_trace_lexical_rank"
	metaKeyFusionScore        = "_trace_fusion_score"
	metaKeyMetadataBoost      = "_trace_metadata_boost"
	metaKeyRerankScore        = "_trace_rerank_score"
	metaKeyFusionPosition     = "_trace_fusion_position"
	metaKeyRefinePosition     = "_trace_refine_position"
	metaKeyFinalPosition      = "_trace_final_position"
	metaKeyFieldBoost         = "_trace_field_boost"
	metaKeyFieldMatches       = "_trace_field_matches"
	metaKeyCoverageBoost      = "_trace_coverage_boost"
	metaKeyIntentRule         = "_trace_intent_rule"
	metaKeyIntentPositiveHits = "_trace_intent_positive_hits"
	metaKeyIntentExcludedHits = "_trace_intent_excluded_hits"
	metaKeyIntentBonus        = "_trace_intent_bonus"
	metaKeyIntentPenalty      = "_trace_intent_penalty"
	metaKeyIntentNetScore     = "_trace_intent_net_score"
)

type CitationTrace struct {
	DenseRank          int      `json:"dense_rank,omitempty"`
	LexicalRank        int      `json:"lexical_rank,omitempty"`
	FusionScore        float64  `json:"fusion_score,omitempty"`
	MetadataBoost      float64  `json:"metadata_boost,omitempty"`
	RerankScore        float64  `json:"rerank_score,omitempty"`
	FusionPosition     int      `json:"fusion_position,omitempty"`
	RefinePosition     int      `json:"refine_position,omitempty"`
	FinalPosition      int      `json:"final_position,omitempty"`
	FieldBoost         float64  `json:"field_boost,omitempty"`
	FieldMatches       []string `json:"field_matches,omitempty"`
	CoverageBoost      float64  `json:"coverage_boost,omitempty"`
	IntentRule         string   `json:"intent_rule,omitempty"`
	IntentPositiveHits []string `json:"intent_positive_hits,omitempty"`
	IntentExcludedHits []string `json:"intent_excluded_hits,omitempty"`
	IntentBonus        float64  `json:"intent_bonus,omitempty"`
	IntentPenalty      float64  `json:"intent_penalty,omitempty"`
	IntentNetScore     float64  `json:"intent_net_score,omitempty"`
}

type Citation struct {
	ID      string         `json:"id"`
	Source  string         `json:"source,omitempty"`
	Title   string         `json:"title,omitempty"`
	Score   float64        `json:"score,omitempty"`
	Snippet string         `json:"snippet,omitempty"`
	Trace   *CitationTrace `json:"trace,omitempty"`
}

// Evidence links a citation to the specific text used in an answer.
type Evidence struct {
	CitationID string `json:"citation_id"`
	Text       string `json:"text"`
}

// KnowledgeSearchOutput is the normalized schema for query_internal_docs tool results.
type KnowledgeSearchOutput struct {
	Success   bool       `json:"success"`
	Degraded  bool       `json:"degraded,omitempty"`
	Message   string     `json:"message,omitempty"`
	Error     string     `json:"error,omitempty"`
	Answer    string     `json:"answer,omitempty"`
	Citations []Citation `json:"citations,omitempty"`
	Evidence  []Evidence `json:"evidence,omitempty"`
}

// sourceKeys is the priority order for extracting a source identifier from document metadata.
var sourceKeys = []string{"_source", "source", "source_uri", "file_path", "file_name", "filename"}

// titleKeys is the priority order for extracting a title from document metadata.
var titleKeys = []string{"title", "file_name", "filename", "source"}

// CitationFromDocument creates a Citation from a schema.Document.
// The id parameter is the caller-assigned citation ID (e.g. "ctx-doc-1", "kb-doc-3").
func CitationFromDocument(doc *schema.Document, id string) Citation {
	if doc == nil {
		return Citation{ID: id, Title: id}
	}

	c := Citation{ID: id}
	c.Source = extractMetaField(doc, sourceKeys)
	c.Title = extractMetaField(doc, titleKeys)
	c.Score = doc.Score()
	c.Snippet = truncateSnippet(doc.Content)
	c.Trace = citationTraceFromMeta(doc.MetaData)

	// Fallbacks
	if c.Source == "" && doc.ID != "" {
		c.Source = doc.ID
	}
	if c.Title == "" && doc.ID != "" {
		c.Title = doc.ID
	}
	if c.Title == "" {
		c.Title = id
	}

	return c
}

// EvidenceFromCitation creates an Evidence entry from a Citation, using its snippet as text.
func EvidenceFromCitation(c Citation) Evidence {
	return Evidence{
		CitationID: c.ID,
		Text:       c.Snippet,
	}
}

// BuildCitations creates Citation and Evidence slices from a list of documents.
// prefix is the ID prefix, e.g. "ctx-doc" or "kb-doc".
func BuildCitations(docs []*schema.Document, prefix string) ([]Citation, []Evidence) {
	if len(docs) == 0 {
		return nil, nil
	}
	citations := make([]Citation, 0, len(docs))
	evidence := make([]Evidence, 0, len(docs))
	for i, doc := range docs {
		id := fmt.Sprintf("%s-%d", prefix, i+1)
		c := CitationFromDocument(doc, id)
		citations = append(citations, c)
		evidence = append(evidence, EvidenceFromCitation(c))
	}
	return citations, evidence
}

func extractMetaField(doc *schema.Document, keys []string) string {
	if doc == nil || doc.MetaData == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := doc.MetaData[key].(string); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func truncateSnippet(content string) string {
	content = strings.TrimSpace(content)
	if utf8.RuneCountInString(content) <= maxSnippetLen {
		return content
	}
	runes := []rune(content)
	return string(runes[:maxSnippetLen]) + "..."
}

func citationTraceFromMeta(meta map[string]any) *CitationTrace {
	if meta == nil {
		return nil
	}
	t := &CitationTrace{}
	hasAny := false
	if v, ok := meta[metaKeyDenseRank].(int); ok {
		t.DenseRank = v
		hasAny = true
	}
	if v, ok := meta[metaKeyLexicalRank].(int); ok {
		t.LexicalRank = v
		hasAny = true
	}
	if v, ok := meta[metaKeyFusionScore].(float64); ok {
		t.FusionScore = v
		hasAny = true
	}
	if v, ok := meta[metaKeyMetadataBoost].(float64); ok {
		t.MetadataBoost = v
		hasAny = true
	}
	if v, ok := meta[metaKeyRerankScore].(float64); ok {
		t.RerankScore = v
		hasAny = true
	}
	if v, ok := meta[metaKeyFusionPosition].(int); ok {
		t.FusionPosition = v
		hasAny = true
	}
	if v, ok := meta[metaKeyRefinePosition].(int); ok {
		t.RefinePosition = v
		hasAny = true
	}
	if v, ok := meta[metaKeyFinalPosition].(int); ok {
		t.FinalPosition = v
		hasAny = true
	}
	if v, ok := meta[metaKeyFieldBoost].(float64); ok {
		t.FieldBoost = v
		hasAny = true
	}
	if v, ok := meta[metaKeyFieldMatches].([]string); ok {
		t.FieldMatches = append([]string(nil), v...)
		hasAny = true
	}
	if v, ok := meta[metaKeyCoverageBoost].(float64); ok {
		t.CoverageBoost = v
		hasAny = true
	}
	if v, ok := meta[metaKeyIntentRule].(string); ok && v != "" {
		t.IntentRule = v
		hasAny = true
	}
	if v, ok := meta[metaKeyIntentPositiveHits].([]string); ok {
		t.IntentPositiveHits = append([]string(nil), v...)
		hasAny = true
	}
	if v, ok := meta[metaKeyIntentExcludedHits].([]string); ok {
		t.IntentExcludedHits = append([]string(nil), v...)
		hasAny = true
	}
	if v, ok := meta[metaKeyIntentBonus].(float64); ok {
		t.IntentBonus = v
		hasAny = true
	}
	if v, ok := meta[metaKeyIntentPenalty].(float64); ok {
		t.IntentPenalty = v
		hasAny = true
	}
	if v, ok := meta[metaKeyIntentNetScore].(float64); ok {
		t.IntentNetScore = v
		hasAny = true
	}
	if !hasAny {
		return nil
	}
	return t
}
