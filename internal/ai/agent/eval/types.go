// Package eval provides evaluation infrastructure for the multi-agent pipeline.
// It supports Golden Case regression tests, LLM-as-Judge quality scoring, and A/B comparison.
package eval

// DiagCase represents a single diagnostic test case with expected outcomes.
type DiagCase struct {
	ID              string   `json:"id"`
	Query           string   `json:"query"`
	ExpectedIntent  string   `json:"expected_intent"`
	ExpectedDomains []string `json:"expected_domains"`
	MustMention     []string `json:"must_mention"`
	MustNotMention  []string `json:"must_not_mention"`
	ExpectedAction  string   `json:"expected_action"`
	Severity        string   `json:"severity"` // high | medium | low
}

// DiagScores represents LLM-as-Judge quality scores for a diagnostic report.
// Each dimension is scored 1-5.
type DiagScores struct {
	Correctness   int    `json:"correctness"`
	Completeness  int    `json:"completeness"`
	Coherence     int    `json:"coherence"`
	Actionability int    `json:"actionability"`
	Overall       int    `json:"overall"`
	Comments      string `json:"comments"`
}

// JudgeResult holds the A/B comparison result for a single case.
type JudgeResult struct {
	CaseID          string     `json:"case_id"`
	Query           string     `json:"query"`
	BaselineScores  DiagScores `json:"baseline_scores"`
	CandidateScores DiagScores `json:"candidate_scores"`
	Delta           DiagScores `json:"delta"`
}

// Runner executes a diagnostic query through the multi-agent pipeline and returns the report.
type Runner interface {
	Run(query string) (*RunResult, error)
}

// RunResult holds the output of a single Runner execution.
type RunResult struct {
	Summary  string
	Intent   string
	Domains  []string
	Metadata map[string]any
}
