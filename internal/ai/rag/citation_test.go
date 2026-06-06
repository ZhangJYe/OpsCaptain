package rag

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

func TestCitationFromDocument_Nil(t *testing.T) {
	c := CitationFromDocument(nil, "ctx-doc-1")
	if c.ID != "ctx-doc-1" {
		t.Fatalf("expected ID=ctx-doc-1, got %s", c.ID)
	}
	if c.Title != "ctx-doc-1" {
		t.Fatalf("expected fallback title=ctx-doc-1, got %s", c.Title)
	}
}

func TestCitationFromDocument_FullMetadata(t *testing.T) {
	doc := &schema.Document{
		ID:      "doc-abc",
		Content: "some content here",
		MetaData: map[string]any{
			"_source": "/path/to/doc.md",
			"title":   "Runbook: Redis Failover",
		},
	}
	c := CitationFromDocument(doc, "kb-doc-1")
	if c.ID != "kb-doc-1" {
		t.Fatalf("ID: got %s", c.ID)
	}
	if c.Source != "/path/to/doc.md" {
		t.Fatalf("Source: got %s", c.Source)
	}
	if c.Title != "Runbook: Redis Failover" {
		t.Fatalf("Title: got %s", c.Title)
	}
	if c.Snippet != "some content here" {
		t.Fatalf("Snippet: got %s", c.Snippet)
	}
}

func TestCitationFromDocument_FallbackSource(t *testing.T) {
	doc := &schema.Document{
		ID:      "doc-xyz",
		Content: "content",
		MetaData: map[string]any{
			"file_name": "deploy.md",
		},
	}
	c := CitationFromDocument(doc, "ctx-doc-2")
	if c.Source != "deploy.md" {
		t.Fatalf("Source fallback to file_name: got %s", c.Source)
	}
	if c.Title != "deploy.md" {
		t.Fatalf("Title fallback to file_name: got %s", c.Title)
	}
}

func TestCitationFromDocument_FallbackToID(t *testing.T) {
	doc := &schema.Document{
		ID:      "doc-123",
		Content: "content",
	}
	c := CitationFromDocument(doc, "ctx-doc-3")
	if c.Source != "doc-123" {
		t.Fatalf("Source fallback to doc.ID: got %s", c.Source)
	}
	if c.Title != "doc-123" {
		t.Fatalf("Title fallback to doc.ID: got %s", c.Title)
	}
}

func TestCitationFromDocument_SnippetTruncation(t *testing.T) {
	long := strings.Repeat("x", 400)
	doc := &schema.Document{ID: "d1", Content: long}
	c := CitationFromDocument(doc, "ctx-doc-1")
	if len(c.Snippet) != 303 { // 300 + "..."
		t.Fatalf("expected snippet len 303, got %d", len(c.Snippet))
	}
	if !strings.HasSuffix(c.Snippet, "...") {
		t.Fatal("expected ... suffix")
	}
}

func TestCitationFromDocument_SnippetTruncationKeepsUTF8(t *testing.T) {
	content := strings.Repeat("支付服务延迟升高", 80)
	doc := &schema.Document{ID: "d1", Content: content}
	c := CitationFromDocument(doc, "ctx-doc-1")
	if !utf8.ValidString(c.Snippet) {
		t.Fatalf("expected valid utf8 snippet, got %q", c.Snippet)
	}
	if !strings.HasSuffix(c.Snippet, "...") {
		t.Fatal("expected ... suffix")
	}
	if utf8.RuneCountInString(strings.TrimSuffix(c.Snippet, "...")) != maxSnippetLen {
		t.Fatalf("expected %d runes before suffix, got %d", maxSnippetLen, utf8.RuneCountInString(strings.TrimSuffix(c.Snippet, "...")))
	}
}

func TestBuildCitations(t *testing.T) {
	docs := []*schema.Document{
		{ID: "d1", Content: "first doc", MetaData: map[string]any{"title": "Doc One"}},
		{ID: "d2", Content: "second doc"},
	}
	citations, evidence := BuildCitations(docs, "kb-doc")
	if len(citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(citations))
	}
	if citations[0].ID != "kb-doc-1" || citations[1].ID != "kb-doc-2" {
		t.Fatalf("unexpected IDs: %s, %s", citations[0].ID, citations[1].ID)
	}
	if citations[0].Title != "Doc One" {
		t.Fatalf("title: got %s", citations[0].Title)
	}
	if len(evidence) != 2 {
		t.Fatalf("expected 2 evidence, got %d", len(evidence))
	}
	if evidence[0].CitationID != "kb-doc-1" {
		t.Fatalf("evidence citation_id: got %s", evidence[0].CitationID)
	}
}

func TestBuildCitations_Empty(t *testing.T) {
	citations, evidence := BuildCitations(nil, "ctx-doc")
	if citations != nil || evidence != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestCitationFromDocument_WithTrace(t *testing.T) {
	doc := &schema.Document{
		ID:      "doc-1",
		Content: "test content",
		MetaData: map[string]any{
			"title":              "Test Doc",
			metaKeyDenseRank:     2,
			metaKeyLexicalRank:   1,
			metaKeyFusionScore:   0.045,
			metaKeyMetadataBoost: 3.0,
			metaKeyRerankScore:   0.87,
		},
	}
	c := CitationFromDocument(doc, "kb-doc-1")
	if c.Trace == nil {
		t.Fatal("expected non-nil trace")
	}
	if c.Trace.DenseRank != 2 {
		t.Fatalf("DenseRank: got %d", c.Trace.DenseRank)
	}
	if c.Trace.LexicalRank != 1 {
		t.Fatalf("LexicalRank: got %d", c.Trace.LexicalRank)
	}
	if c.Trace.FusionScore != 0.045 {
		t.Fatalf("FusionScore: got %f", c.Trace.FusionScore)
	}
	if c.Trace.MetadataBoost != 3.0 {
		t.Fatalf("MetadataBoost: got %f", c.Trace.MetadataBoost)
	}
	if c.Trace.RerankScore != 0.87 {
		t.Fatalf("RerankScore: got %f", c.Trace.RerankScore)
	}
}

func TestCitationFromDocument_NoTraceMeta(t *testing.T) {
	doc := &schema.Document{
		ID:      "doc-1",
		Content: "test content",
		MetaData: map[string]any{
			"title": "Test Doc",
		},
	}
	c := CitationFromDocument(doc, "kb-doc-1")
	if c.Trace != nil {
		t.Fatalf("expected nil trace when no trace metadata, got %+v", c.Trace)
	}
}

func TestBuildCitations_WithTrace(t *testing.T) {
	docs := []*schema.Document{
		{
			ID:      "d1",
			Content: "first doc",
			MetaData: map[string]any{
				"title":            "Doc One",
				metaKeyDenseRank:   1,
				metaKeyFusionScore: 0.05,
			},
		},
		{
			ID:      "d2",
			Content: "second doc",
			MetaData: map[string]any{
				metaKeyLexicalRank: 1,
				metaKeyRerankScore: 0.9,
			},
		},
	}
	citations, _ := BuildCitations(docs, "kb-doc")
	if citations[0].Trace == nil {
		t.Fatal("expected trace for first citation")
	}
	if citations[0].Trace.DenseRank != 1 {
		t.Fatalf("DenseRank: got %d", citations[0].Trace.DenseRank)
	}
	if citations[1].Trace == nil {
		t.Fatal("expected trace for second citation")
	}
	if citations[1].Trace.RerankScore != 0.9 {
		t.Fatalf("RerankScore: got %f", citations[1].Trace.RerankScore)
	}
}
