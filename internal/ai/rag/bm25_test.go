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

func TestBM25Index_ReplacesExistingDocumentByID(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index()
	idx.AddDocument("doc-1", "checkoutservice timeout", nil)
	idx.AddDocument("doc-1", "frontend latency", nil)

	if idx.Size() != 1 {
		t.Fatalf("expected 1 doc after replace, got %d", idx.Size())
	}

	hits := idx.Search("frontend", 5)
	if len(hits) != 1 || hits[0].DocID != "doc-1" {
		t.Fatalf("expected replaced doc to match frontend query, got %+v", hits)
	}

	hits = idx.Search("checkoutservice", 5)
	if len(hits) != 0 {
		t.Fatalf("expected old content to be replaced, got %+v", hits)
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

func TestBM25Tokenize_Chinese(t *testing.T) {
	t.Parallel()

	tokens := bm25Tokenize("checkoutservice 超时 支付失败")
	got := make(map[string]bool)
	for _, tok := range tokens {
		got[tok] = true
	}

	if !got["checkoutservice"] {
		t.Error("expected checkoutservice token")
	}

	if !got["超"] || !got["时"] || !got["支"] || !got["付"] || !got["失"] || !got["败"] {
		t.Errorf("expected CJK unigram tokens, got %v", tokens)
	}

	if !got["超时"] {
		t.Errorf("expected bigram '超时', got %v", tokens)
	}
	if !got["支付"] {
		t.Errorf("expected bigram '支付', got %v", tokens)
	}
	if !got["失败"] {
		t.Errorf("expected bigram '失败', got %v", tokens)
	}
}

func TestBM25Index_ChineseSearch(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index()
	idx.AddDocument("case-001", "checkoutservice 超时 支付服务下游故障", nil)
	idx.AddDocument("case-002", "frontend 页面加载慢 用户体验下降", nil)
	idx.AddDocument("case-003", "recommendationservice 推荐服务响应延迟", nil)

	hits := idx.Search("超时 支付", 5)
	if len(hits) == 0 {
		t.Fatal("expected at least one hit for Chinese query")
	}
	if hits[0].DocID != "case-001" {
		t.Fatalf("expected case-001 to rank first for '超时 支付', got %s", hits[0].DocID)
	}
}

func TestBM25Hit_PreservesContent(t *testing.T) {
	t.Parallel()

	idx := NewBM25Index()
	idx.AddDocument("doc-1", "checkoutservice rrt timeout spike with paymentservice", nil)
	idx.AddDocument("doc-2", "frontend high latency", nil)

	hits := idx.Search("checkoutservice timeout", 5)
	if len(hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if hits[0].Content != "checkoutservice rrt timeout spike with paymentservice" {
		t.Fatalf("expected content preserved in hit, got %q", hits[0].Content)
	}
}
