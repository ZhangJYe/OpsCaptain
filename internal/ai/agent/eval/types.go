package eval

import (
	"context"
	"time"
)

type DiagCase struct {
	ID              string   `json:"id"`
	Query           string   `json:"query"`
	ExpectedIntent  string   `json:"expected_intent"`
	ExpectedDomains []string `json:"expected_domains"`
	MustMention     []string `json:"must_mention"`
	MustNotMention  []string `json:"must_not_mention"`
	ExpectedAction  string   `json:"expected_action"`
	Severity        string   `json:"severity"`
}

type DiagScores struct {
	Correctness   int    `json:"correctness"`
	Completeness  int    `json:"completeness"`
	Coherence     int    `json:"coherence"`
	Actionability int    `json:"actionability"`
	Overall       int    `json:"overall"`
	Comments      string `json:"comments"`
}

type DiagScoreAverages struct {
	Correctness   float64 `json:"correctness"`
	Completeness  float64 `json:"completeness"`
	Coherence     float64 `json:"coherence"`
	Actionability float64 `json:"actionability"`
	Overall       float64 `json:"overall"`
}

type JudgeResult struct {
	CaseID          string     `json:"case_id"`
	Query           string     `json:"query"`
	BaselineScores  DiagScores `json:"baseline_scores"`
	CandidateScores DiagScores `json:"candidate_scores"`
	Delta           DiagScores `json:"delta"`
}

type Runner interface {
	Run(context.Context, string) (*RunResult, error)
}

type RunResult struct {
	Summary       string         `json:"summary"`
	Intent        string         `json:"intent,omitempty"`
	Domains       []string       `json:"domains,omitempty"`
	Status        string         `json:"status,omitempty"`
	Latency       time.Duration  `json:"-"`
	LatencyMillis int64          `json:"latency_ms,omitempty"`
	TokensUsed    int64          `json:"tokens_used,omitempty"`
	LLMCalls      int            `json:"llm_calls,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type GoldenCaseResult struct {
	CaseID   string     `json:"case_id"`
	Query    string     `json:"query"`
	Passed   bool       `json:"passed"`
	Failures []string   `json:"failures,omitempty"`
	Result   *RunResult `json:"result,omitempty"`
}

type GoldenSummary struct {
	Cases    int     `json:"cases"`
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	PassRate float64 `json:"pass_rate"`
}

type RoutingCaseResult struct {
	CaseID          string   `json:"case_id"`
	Query           string   `json:"query"`
	ExpectedIntent  string   `json:"expected_intent,omitempty"`
	ActualIntent    string   `json:"actual_intent,omitempty"`
	IntentMatched   bool     `json:"intent_matched"`
	ExpectedDomains []string `json:"expected_domains,omitempty"`
	ActualDomains   []string `json:"actual_domains,omitempty"`
	DomainPrecision float64  `json:"domain_precision"`
	DomainRecall    float64  `json:"domain_recall"`
	DomainF1        float64  `json:"domain_f1"`
	Fallback        bool     `json:"fallback"`
	Failure         string   `json:"failure,omitempty"`
}

type RoutingSummary struct {
	Cases           int     `json:"cases"`
	IntentCorrect   int     `json:"intent_correct"`
	IntentAccuracy  float64 `json:"intent_accuracy"`
	DomainPrecision float64 `json:"domain_precision"`
	DomainRecall    float64 `json:"domain_recall"`
	DomainF1        float64 `json:"domain_f1"`
	FallbackCount   int     `json:"fallback_count"`
	FallbackRate    float64 `json:"fallback_rate"`
	Failed          int     `json:"failed"`
}

type RoutingReport struct {
	Summary RoutingSummary      `json:"summary"`
	Results []RoutingCaseResult `json:"results"`
}

type Judge interface {
	Score(context.Context, string, string) (DiagScores, error)
}

type JudgeFunc func(context.Context, string, string) (DiagScores, error)

func (f JudgeFunc) Score(ctx context.Context, query, report string) (DiagScores, error) {
	return f(ctx, query, report)
}

type ABCaseResult struct {
	CaseID          string     `json:"case_id"`
	Query           string     `json:"query"`
	Baseline        *RunResult `json:"baseline"`
	Candidate       *RunResult `json:"candidate"`
	BaselineScores  DiagScores `json:"baseline_scores,omitempty"`
	CandidateScores DiagScores `json:"candidate_scores,omitempty"`
	DeltaScores     DiagScores `json:"delta_scores,omitempty"`
	Failures        []string   `json:"failures,omitempty"`
}

type ABReport struct {
	Name                     string            `json:"name,omitempty"`
	Cases                    int               `json:"cases"`
	Results                  []ABCaseResult    `json:"results"`
	BaselineAverageScores    DiagScoreAverages `json:"baseline_average_scores,omitempty"`
	CandidateAverageScores   DiagScoreAverages `json:"candidate_average_scores,omitempty"`
	DeltaAverageScores       DiagScoreAverages `json:"delta_average_scores,omitempty"`
	BaselineMedianLatencyMs  int64             `json:"baseline_median_latency_ms,omitempty"`
	CandidateMedianLatencyMs int64             `json:"candidate_median_latency_ms,omitempty"`
	BaselineAverageTokens    float64           `json:"baseline_average_tokens,omitempty"`
	CandidateAverageTokens   float64           `json:"candidate_average_tokens,omitempty"`
	BaselineAverageLLMCalls  float64           `json:"baseline_average_llm_calls,omitempty"`
	CandidateAverageLLMCalls float64           `json:"candidate_average_llm_calls,omitempty"`
}

type CalibrationCase struct {
	ID             string     `json:"id"`
	Query          string     `json:"query"`
	Report         string     `json:"report"`
	HumanScore     int        `json:"human_score,omitempty"`
	ExpectedScores DiagScores `json:"expected_scores,omitempty"`
	Notes          string     `json:"notes,omitempty"`
}
