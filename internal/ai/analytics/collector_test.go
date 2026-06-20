package analytics

import (
	"testing"
)

func TestRecordQuery(t *testing.T) {
	c := NewCollector()
	c.RecordQuery("test query", "gpt-4", []string{"search", "web"}, 100)

	stats := c.Stats()
	if stats.TotalQueries != 1 {
		t.Errorf("expected TotalQueries=1, got %d", stats.TotalQueries)
	}
	if stats.QueriesToday != 1 {
		t.Errorf("expected QueriesToday=1, got %d", stats.QueriesToday)
	}
	if stats.AvgResponseMs != 100 {
		t.Errorf("expected AvgResponseMs=100, got %d", stats.AvgResponseMs)
	}
	if stats.ToolUsage["search"] != 1 {
		t.Errorf("expected search tool count=1, got %d", stats.ToolUsage["search"])
	}
	if stats.ToolUsage["web"] != 1 {
		t.Errorf("expected web tool count=1, got %d", stats.ToolUsage["web"])
	}
	if stats.ModelUsage["gpt-4"] != 1 {
		t.Errorf("expected gpt-4 model count=1, got %d", stats.ModelUsage["gpt-4"])
	}
}

func TestStats(t *testing.T) {
	c := NewCollector()
	c.RecordQuery("q1", "gpt-4", []string{"search"}, 100)
	c.RecordQuery("q2", "gpt-4", []string{"search", "web"}, 200)
	c.RecordQuery("q1", "gpt-4", []string{"search"}, 150)

	stats := c.Stats()
	if stats.TotalQueries != 3 {
		t.Errorf("expected TotalQueries=3, got %d", stats.TotalQueries)
	}
	if stats.AvgResponseMs != 150 {
		t.Errorf("expected AvgResponseMs=150, got %d", stats.AvgResponseMs)
	}
	if stats.ModelUsage["gpt-4"] != 3 {
		t.Errorf("expected gpt-4 count=3, got %d", stats.ModelUsage["gpt-4"])
	}
	if stats.ToolUsage["search"] != 3 {
		t.Errorf("expected search count=3, got %d", stats.ToolUsage["search"])
	}
	if stats.ToolUsage["web"] != 1 {
		t.Errorf("expected web count=1, got %d", stats.ToolUsage["web"])
	}
	if len(stats.TopQueries) == 0 || stats.TopQueries[0].Query != "q1" {
		t.Errorf("expected top query to be q1, got %v", stats.TopQueries)
	}
	if len(stats.QueryTrend) != 7 {
		t.Errorf("expected 7 days of trend, got %d", len(stats.QueryTrend))
	}
}

func TestStatsEmpty(t *testing.T) {
	c := NewCollector()
	stats := c.Stats()
	if stats.TotalQueries != 0 {
		t.Errorf("expected TotalQueries=0, got %d", stats.TotalQueries)
	}
	if stats.ToolUsage == nil {
		t.Error("expected ToolUsage to be non-nil map")
	}
	if stats.ModelUsage == nil {
		t.Error("expected ModelUsage to be non-nil map")
	}
	if len(stats.QueryTrend) != 7 {
		t.Errorf("expected 7 days of trend, got %d", len(stats.QueryTrend))
	}
}
