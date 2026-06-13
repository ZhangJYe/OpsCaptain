package rag

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestDefaultAgentConfig(t *testing.T) {
	cfg := DefaultAgentConfig()
	if cfg.Enabled {
		t.Error("expected Enabled=false")
	}
	if cfg.Model != "chat_model_fast" {
		t.Errorf("expected Model=chat_model_fast, got %s", cfg.Model)
	}
	if cfg.MaxRounds != 3 {
		t.Errorf("expected MaxRounds=3, got %d", cfg.MaxRounds)
	}
	if cfg.ConfidenceThreshold != 0.7 {
		t.Errorf("expected ConfidenceThreshold=0.7, got %f", cfg.ConfidenceThreshold)
	}
	if cfg.EvalTimeoutMs != 10000 {
		t.Errorf("expected EvalTimeoutMs=10000, got %d", cfg.EvalTimeoutMs)
	}
	if cfg.PlanTimeoutMs != 10000 {
		t.Errorf("expected PlanTimeoutMs=10000, got %d", cfg.PlanTimeoutMs)
	}
	if cfg.TotalTimeoutMs != 30000 {
		t.Errorf("expected TotalTimeoutMs=30000, got %d", cfg.TotalTimeoutMs)
	}
	if cfg.MaxTotalTokens != 8000 {
		t.Errorf("expected MaxTotalTokens=8000, got %d", cfg.MaxTotalTokens)
	}
}

func TestFilterNewDocs(t *testing.T) {
	docs := []*schema.Document{
		{ID: "doc-1", Content: "c1"},
		{ID: "doc-2", Content: "c2"},
		{ID: "doc-3", Content: "c3"},
	}
	seen := map[string]struct{}{
		"doc-1": {},
	}
	result := filterNewDocs(docs, seen)
	if len(result) != 2 {
		t.Fatalf("expected 2 docs, got %d", len(result))
	}
	if result[0].ID != "doc-2" {
		t.Errorf("expected doc-2, got %s", result[0].ID)
	}
	if result[1].ID != "doc-3" {
		t.Errorf("expected doc-3, got %s", result[1].ID)
	}
	_, ok := seen["doc-2"]
	if !ok {
		t.Error("doc-2 should be added to seen")
	}
}

func TestFilterNewDocs_Empty(t *testing.T) {
	result := filterNewDocs(nil, map[string]struct{}{})
	if len(result) != 0 {
		t.Errorf("expected 0 docs, got %d", len(result))
	}
}

func TestMergeAndDedup(t *testing.T) {
	docs := []*schema.Document{
		{ID: "a", Content: "first-a"},
		{ID: "b", Content: "b"},
		{ID: "a", Content: "second-a"},
		{ID: "c", Content: "c"},
		{ID: "b", Content: "second-b"},
	}
	result := mergeAndDedup(docs)
	if len(result) != 3 {
		t.Fatalf("expected 3 docs, got %d", len(result))
	}
	if result[0].ID != "a" || result[0].Content != "first-a" {
		t.Errorf("expected first-a, got %s/%s", result[0].ID, result[0].Content)
	}
	if result[1].ID != "b" || result[1].Content != "b" {
		t.Errorf("expected b, got %s/%s", result[1].ID, result[1].Content)
	}
	if result[2].ID != "c" || result[2].Content != "c" {
		t.Errorf("expected c, got %s/%s", result[2].ID, result[2].Content)
	}
}

func TestMergeAndDedup_Empty(t *testing.T) {
	result := mergeAndDedup(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 docs, got %d", len(result))
	}
}

func TestCanonicalDocID(t *testing.T) {
	tests := []struct {
		name     string
		doc      *schema.Document
		expected string
	}{
		{
			name:     "nil doc",
			doc:      nil,
			expected: "",
		},
		{
			name:     "case_id in metadata",
			doc:      &schema.Document{ID: "id1", MetaData: map[string]any{"case_id": "case-123"}},
			expected: "case-123",
		},
		{
			name:     "doc_id in metadata",
			doc:      &schema.Document{ID: "id1", MetaData: map[string]any{"doc_id": "doc-456"}},
			expected: "doc-456",
		},
		{
			name:     "fallback to doc ID",
			doc:      &schema.Document{ID: "  fallback-id  ", MetaData: map[string]any{"other": "val"}},
			expected: "fallback-id",
		},
		{
			name:     "empty doc ID",
			doc:      &schema.Document{ID: "", MetaData: map[string]any{"other": "val"}},
			expected: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := canonicalDocID(tt.doc)
			if got != tt.expected {
				t.Errorf("canonicalDocID() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestParseEvalResult(t *testing.T) {
	raw := `{"confidence": 0.85, "sufficient": true, "missing_info": ["missing1"], "next_strategy": "expand_scope", "reason": "good"}`
	result := parseEvalResult(raw)
	if result.Confidence != 0.85 {
		t.Errorf("expected confidence=0.85, got %f", result.Confidence)
	}
	if !result.Sufficient {
		t.Error("expected sufficient=true")
	}
	if result.NextStrategy != "expand_scope" {
		t.Errorf("expected next_strategy=expand_scope, got %s", result.NextStrategy)
	}
	if len(result.MissingInfo) != 1 || result.MissingInfo[0] != "missing1" {
		t.Errorf("unexpected missing_info: %v", result.MissingInfo)
	}
}

func TestParseEvalResult_Invalid(t *testing.T) {
	result := parseEvalResult("not json")
	if result.Sufficient {
		t.Error("expected sufficient=false for invalid input")
	}
	if result.NextStrategy != "none" {
		t.Errorf("expected next_strategy=none, got %s", result.NextStrategy)
	}
}

func TestParseRetrievalPlan(t *testing.T) {
	raw := `{"sub_queries": ["q1", "q2"], "strategy": "expand_scope", "reason": "need more"}`
	result := parseRetrievalPlan(raw)
	if len(result.SubQueries) != 2 || result.SubQueries[0] != "q1" || result.SubQueries[1] != "q2" {
		t.Errorf("unexpected sub_queries: %v", result.SubQueries)
	}
	if result.Strategy != "expand_scope" {
		t.Errorf("expected strategy=expand_scope, got %s", result.Strategy)
	}
	if result.Reason != "need more" {
		t.Errorf("expected reason='need more', got %s", result.Reason)
	}
}

func TestParseRetrievalPlan_Invalid(t *testing.T) {
	result := parseRetrievalPlan("not json")
	if result.Strategy != "none" {
		t.Errorf("expected strategy=none for invalid input, got %s", result.Strategy)
	}
	if len(result.SubQueries) != 0 {
		t.Errorf("expected empty sub_queries, got %v", result.SubQueries)
	}
}
