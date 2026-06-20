package analytics

type DashboardStats struct {
	TotalQueries  int            `json:"total_queries"`
	QueriesToday  int            `json:"queries_today"`
	ToolUsage     map[string]int `json:"tool_usage"`
	ModelUsage    map[string]int `json:"model_usage"`
	AvgResponseMs int64          `json:"avg_response_ms"`
	FeedbackScore float64        `json:"feedback_score"`
	TopQueries    []QueryCount   `json:"top_queries"`
	QueryTrend    []DayCount     `json:"query_trend"`
}

type QueryCount struct {
	Query string `json:"query"`
	Count int    `json:"count"`
}

type DayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}
