package rag

import (
	"fmt"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const maxSnippetLen = 300

// Citation represents a traceable reference to a source document.
type Citation struct {
	ID      string  `json:"id"`
	Source  string  `json:"source,omitempty"`
	Title   string  `json:"title,omitempty"`
	Score   float64 `json:"score,omitempty"`
	Snippet string  `json:"snippet,omitempty"`
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
	if len(content) <= maxSnippetLen {
		return content
	}
	return content[:maxSnippetLen] + "..."
}
