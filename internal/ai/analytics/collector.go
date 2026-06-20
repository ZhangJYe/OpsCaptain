package analytics

import (
	"sort"
	"sync"
	"time"
)

type queryRecord struct {
	query      string
	model      string
	toolsUsed  []string
	durationMs int64
	timestamp  time.Time
}

type Collector struct {
	mu            sync.RWMutex
	queries       []queryRecord
	toolCalls     map[string]int
	modelCalls    map[string]int
	totalMs       int64
	totalCount    int
	feedbackScore float64
}

var defaultCollector *Collector

func init() {
	defaultCollector = NewCollector()
}

func DefaultCollector() *Collector {
	return defaultCollector
}

func NewCollector() *Collector {
	return &Collector{
		toolCalls:  make(map[string]int),
		modelCalls: make(map[string]int),
	}
}

func (c *Collector) RecordQuery(query, model string, tools []string, durationMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.queries = append(c.queries, queryRecord{
		query:      query,
		model:      model,
		toolsUsed:  tools,
		durationMs: durationMs,
		timestamp:  time.Now(),
	})
	c.toolCalls[model]++
	for _, t := range tools {
		c.toolCalls[t]++
	}
	c.totalMs += durationMs
	c.totalCount++
}

func (c *Collector) SetFeedbackScore(score float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.feedbackScore = score
}

func (c *Collector) Stats() DashboardStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := len(c.queries)
	if total == 0 {
		return DashboardStats{
			ToolUsage:  make(map[string]int),
			ModelUsage: make(map[string]int),
			TopQueries: []QueryCount{},
			QueryTrend: c.queryTrend(7),
		}
	}

	today := time.Now().Truncate(24 * time.Hour)
	toolUsage := make(map[string]int)
	modelUsage := make(map[string]int)
	var queriesToday int
	var totalMs int64

	for _, q := range c.queries {
		if q.timestamp.After(today) || q.timestamp.Equal(today) {
			queriesToday++
		}
		totalMs += q.durationMs
		if q.model != "" {
			modelUsage[q.model]++
		}
		for _, t := range q.toolsUsed {
			toolUsage[t]++
		}
	}

	var avgMs int64
	if total > 0 {
		avgMs = totalMs / int64(total)
	}

	topQueries := c.topQueries(10)
	queryTrend := c.queryTrend(7)

	return DashboardStats{
		TotalQueries:  total,
		QueriesToday:  queriesToday,
		ToolUsage:     toolUsage,
		ModelUsage:    modelUsage,
		AvgResponseMs: avgMs,
		FeedbackScore: c.feedbackScore,
		TopQueries:    topQueries,
		QueryTrend:    queryTrend,
	}
}

func (c *Collector) topQueries(n int) []QueryCount {
	counts := make(map[string]int)
	for _, q := range c.queries {
		counts[q.query]++
	}

	type qc struct {
		query string
		count int
	}
	var sorted []qc
	for query, count := range counts {
		sorted = append(sorted, qc{query: query, count: count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	if len(sorted) > n {
		sorted = sorted[:n]
	}

	result := make([]QueryCount, len(sorted))
	for i, s := range sorted {
		result[i] = QueryCount{Query: s.query, Count: s.count}
	}
	return result
}

func (c *Collector) queryTrend(days int) []DayCount {
	counts := make(map[string]int)
	now := time.Now()
	cutoff := now.AddDate(0, 0, -(days - 1)).Truncate(24 * time.Hour)

	for _, q := range c.queries {
		if q.timestamp.After(cutoff) || q.timestamp.Equal(cutoff) {
			date := q.timestamp.Format("2006-01-02")
			counts[date]++
		}
	}

	result := make([]DayCount, days)
	for i := days - 1; i >= 0; i-- {
		d := now.AddDate(0, 0, -i).Format("2006-01-02")
		result[days-1-i] = DayCount{Date: d, Count: counts[d]}
	}
	return result
}
