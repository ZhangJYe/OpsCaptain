package eval

import "context"

type RetrievedDoc struct {
	ID      string  `json:"id"`
	Title   string  `json:"title,omitempty"`
	Content string  `json:"content,omitempty"`
	Score   float64 `json:"score"`
}

type EvalCase struct {
	ID          string   `json:"id"`
	Query       string   `json:"query"`
	RelevantIDs []string `json:"relevant_ids"`
	Notes       string   `json:"notes,omitempty"`
}

type Searcher interface {
	Search(context.Context, string, int) ([]RetrievedDoc, error)
}

type CaseResult struct {
	CaseID      string           `json:"case_id"`
	Query       string           `json:"query"`
	RelevantIDs []string         `json:"relevant_ids"`
	RankedIDs   []string         `json:"ranked_ids"`
	HitIDsByK   map[int][]string `json:"hit_ids_by_k"`
	RecallAtK   map[int]float64  `json:"recall_at_k"`
}

type CaseFailure struct {
	CaseID string `json:"case_id"`
	Error  string `json:"error"`
}

type Summary struct {
	Cases         int             `json:"cases"`
	Succeeded     int             `json:"succeeded"`
	Failed        int             `json:"failed"`
	AvgRecallAtK  map[int]float64 `json:"avg_recall_at_k"`
	HitRateAtK    map[int]float64 `json:"hit_rate_at_k"`
	FullRecallAtK map[int]int     `json:"full_recall_at_k"`
	Failures      []CaseFailure   `json:"failures,omitempty"`
}

type RunOptions struct {
	ContinueOnError bool
}

func defaultRunOptions() RunOptions {
	return RunOptions{ContinueOnError: false}
}
