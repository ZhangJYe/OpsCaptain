package rag

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestNeedsDecomposition(t *testing.T) {
	t.Parallel()
	cfg := DefaultPlannerConfig()

	tests := []struct {
		name     string
		query    string
		expected bool
	}{
		{"short query", "告警", false},
		{"single topic no keyword", "Prometheus 告警规则配置方法", false},
		{"keyword he", "MySQL 主从切换和 Redis 演进历史都看一下", true},
		{"keyword weishenme", "payment 服务最近为什么延迟升高", true},
		{"keyword gen", "Redis 慢查询跟网络延迟有没有关系", true},
		{"multiple keywords", "MySQL 复制延迟为什么升高，跟内存有没有关系", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := needsDecomposition(tt.query, cfg)
			if got != tt.expected {
				t.Errorf("needsDecomposition(%q) = %v, want %v", tt.query, got, tt.expected)
			}
		})
	}
}

func TestDefaultPlannerConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultPlannerConfig()

	if cfg.Enabled {
		t.Error("expected Enabled=false")
	}
	if cfg.MaxSubQueries != 4 {
		t.Errorf("expected MaxSubQueries=4, got %d", cfg.MaxSubQueries)
	}
	if cfg.TimeoutMs != 5000 {
		t.Errorf("expected TimeoutMs=5000, got %d", cfg.TimeoutMs)
	}
	if cfg.MinQueryLength != 15 {
		t.Errorf("expected MinQueryLength=15, got %d", cfg.MinQueryLength)
	}
	if len(cfg.DecompositionKeywords) == 0 {
		t.Error("expected DecompositionKeywords not empty")
	}
}

func TestMergeResults_DedupAndRRF(t *testing.T) {
	t.Parallel()

	docsA := []*schema.Document{
		{ID: "doc-1", Content: "first"},
		{ID: "doc-2", Content: "second"},
		{ID: "doc-3", Content: "third"},
	}
	docsB := []*schema.Document{
		{ID: "doc-1", Content: "first"},
		{ID: "doc-4", Content: "fourth"},
	}

	subQueryResults := [][]*schema.Document{docsA, docsB}
	subQueries := []string{"query A", "query B"}

	merged, subQueryMap := MergeResults(subQueryResults, subQueries, 10)

	if len(merged) != 4 {
		t.Fatalf("expected 4 unique docs, got %d", len(merged))
	}

	// doc-1 appears in both sub-queries, should rank first (highest RRF score)
	if merged[0].ID != "doc-1" {
		t.Errorf("expected doc-1 first, got %s", merged[0].ID)
	}

	sqs := subQueryMap["doc-1"]
	if len(sqs) != 2 {
		t.Errorf("expected doc-1 linked to 2 sub-queries, got %d", len(sqs))
	}
}

func TestMergeResults_Empty(t *testing.T) {
	t.Parallel()

	docs, subQueryMap := MergeResults(nil, nil, 10)
	if len(docs) != 0 {
		t.Errorf("expected empty docs, got %d", len(docs))
	}
	if len(subQueryMap) != 0 {
		t.Errorf("expected empty subQueryMap, got %d", len(subQueryMap))
	}

	docs2, subQueryMap2 := MergeResults([][]*schema.Document{}, []string{}, 5)
	if len(docs2) != 0 {
		t.Errorf("expected empty docs from empty slices, got %d", len(docs2))
	}
	if len(subQueryMap2) != 0 {
		t.Errorf("expected empty subQueryMap from empty slices, got %d", len(subQueryMap2))
	}
}

func TestAnalyze_Disabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultPlannerConfig()
	cfg.Enabled = false

	result := Analyze(context.Background(), "MySQL 和 Redis 的性能对比分析", cfg)

	if result.Decomposed {
		t.Error("expected Decomposed=false when planner is disabled")
	}
	if result.Reason != "planner_disabled" {
		t.Errorf("expected reason planner_disabled, got %s", result.Reason)
	}
}

func TestAnalyze_NoDecompositionNeeded(t *testing.T) {
	t.Parallel()

	cfg := DefaultPlannerConfig()
	cfg.Enabled = true

	result := Analyze(context.Background(), "Prometheus 告警规则配置方法", cfg)

	if result.Decomposed {
		t.Error("expected Decomposed=false for single-topic query")
	}
	if result.Reason != "no_decomposition_needed" {
		t.Errorf("expected reason no_decomposition_needed, got %s", result.Reason)
	}
}
