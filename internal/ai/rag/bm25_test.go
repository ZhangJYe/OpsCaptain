package rag

import (
	"testing"
)

func TestBM25Index_BasicSearch(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index()
	idx.AddDocument("case-001", "checkoutservice rrt timeout spike with paymentservice downstream failures", map[string]string{
		"service":     "checkoutservice",
		"destination": "paymentservice",
	})
	idx.AddDocument("case-002", "frontend high latency connecting to productcatalogservice", map[string]string{
		"service": "frontend",
	})
	idx.AddDocument("case-003", "recommendationservice cpu usage spike causing slow responses", map[string]string{
		"service": "recommendationservice",
	})

	if idx.Size() != 3 {
		t.Fatalf("expected 3 docs, got %d", idx.Size())
	}

	hits := idx.Search("checkoutservice rrt timeout paymentservice", 5)
	if len(hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if hits[0].DocID != "case-001" {
		t.Fatalf("expected case-001 to rank first, got %s", hits[0].DocID)
	}
}

func TestBM25Index_MetadataFieldsContribute(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index()
	idx.AddDocument("with-meta", "generic anomaly detected", map[string]string{
		"service":          "checkoutservice",
		"metric_names":     "rrt timeout",
		"trace_operations": "charge payment",
	})
	idx.AddDocument("no-meta", "generic anomaly detected on some service", nil)

	hits := idx.Search("checkoutservice rrt charge", 5)
	if len(hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if hits[0].DocID != "with-meta" {
		t.Fatalf("expected with-meta to rank first due to metadata, got %s", hits[0].DocID)
	}
}

func TestBM25Index_EmptyQuery(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index()
	idx.AddDocument("doc1", "some content", nil)

	hits := idx.Search("", 5)
	if len(hits) != 0 {
		t.Fatalf("expected empty result for empty query, got %d", len(hits))
	}
}

func TestBM25Index_EmptyIndex(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index()
	hits := idx.Search("something", 5)
	if len(hits) != 0 {
		t.Fatalf("expected empty result for empty index, got %d", len(hits))
	}
}

func TestBM25Tokenize_Consistency(t *testing.T) {
	t.Parallel()

	tokens := bm25Tokenize("CheckoutService RRT_timeout CPU.usage /api/v1/pay")
	expected := map[string]bool{
		"checkoutservice": true,
		"rrt_timeout":     true,
		"cpu.usage":       true,
		"/api/v1/pay":     true,
	}
	got := make(map[string]bool)
	for _, tok := range tokens {
		got[tok] = true
	}
	for k := range expected {
		if !got[k] {
			t.Errorf("expected token %q not found in %v", k, tokens)
		}
	}
}
