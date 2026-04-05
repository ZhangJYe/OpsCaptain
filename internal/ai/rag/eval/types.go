package eval

import "context"

type RetrievedDoc struct {
	ID      string
	Title   string
	Content string
	Score   float64
}

type EvalCase struct {
	ID          string
	Query       string
	RelevantIDs []string
	Notes       string
}

type Searcher interface {
	Search(context.Context, string, int) ([]RetrievedDoc, error)
}

type CaseResult struct {
	CaseID      string
	Query       string
	RelevantIDs []string
	RankedIDs   []string
	HitIDsByK   map[int][]string
	RecallAtK   map[int]float64
}

type Summary struct {
	Cases         int
	AvgRecallAtK  map[int]float64
	HitRateAtK    map[int]float64
	FullRecallAtK map[int]int
}
