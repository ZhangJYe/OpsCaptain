package eval

import (
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
)

type EvalCase struct {
	ID               string   `json:"id"`
	Symptom          string   `json:"symptom"`
	GroundTruth      string   `json:"ground_truth"`
	ExpectedKeywords []string `json:"expected_keywords,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

type EvalResult struct {
	CaseID            string        `json:"case_id"`
	Symptom           string        `json:"symptom"`
	Prediction        string        `json:"prediction"`
	GroundTruth       string        `json:"ground_truth"`
	Matched           bool          `json:"matched"`
	Latency           time.Duration `json:"latency"`
	LLMCalls          int           `json:"llm_calls"`
	Status            string        `json:"status"`
	EvidenceCount     int           `json:"evidence_count"`
	TraceComplete     bool          `json:"trace_complete"`
	DegradationReason string        `json:"degradation_reason,omitempty"`
}

type EvalMetrics struct {
	TotalCases       int            `json:"total_cases"`
	Succeeded        int            `json:"succeeded"`
	Failed           int            `json:"failed"`
	Degraded         int            `json:"degraded"`
	Matched          int            `json:"matched"`
	Accuracy         float64        `json:"accuracy"`
	EvidenceCoverage float64        `json:"evidence_coverage"`
	AvgLatency       time.Duration  `json:"avg_latency"`
	AvgLLMCalls      float64        `json:"avg_llm_calls"`
	DegradationRate  float64        `json:"degradation_rate"`
	Traceability     float64        `json:"traceability"`
	PerStatus        map[string]int `json:"per_status"`
	totalLatency     time.Duration
	totalLLMCalls    int
	traceComplete    int
	evidencePresent  int
}

type GateResult struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

type GateReport struct {
	AllPassed bool         `json:"all_passed"`
	Gates     []GateResult `json:"gates"`
}

func NewEvalMetrics() *EvalMetrics {
	return &EvalMetrics{
		PerStatus: make(map[string]int),
	}
}

func (m *EvalMetrics) AddResult(r *EvalResult) {
	m.TotalCases++
	m.PerStatus[r.Status]++

	switch r.Status {
	case string(protocol.ResultStatusSucceeded):
		m.Succeeded++
	case string(protocol.ResultStatusFailed):
		m.Failed++
	case string(protocol.ResultStatusDegraded):
		m.Degraded++
	}

	if r.Matched {
		m.Matched++
	}

	m.totalLatency += r.Latency
	m.totalLLMCalls += r.LLMCalls

	if r.EvidenceCount > 0 {
		m.evidencePresent++
	}

	if r.TraceComplete {
		m.traceComplete++
	}
}

func (m *EvalMetrics) Finalize() {
	if m.TotalCases == 0 {
		return
	}

	m.Accuracy = float64(m.Matched) / float64(m.TotalCases)
	m.EvidenceCoverage = float64(m.evidencePresent) / float64(m.TotalCases)
	m.DegradationRate = float64(m.Degraded) / float64(m.TotalCases)
	m.Traceability = float64(m.traceComplete) / float64(m.TotalCases)

	m.AvgLatency = m.totalLatency / time.Duration(m.TotalCases)
	m.AvgLLMCalls = float64(m.totalLLMCalls) / float64(m.TotalCases)
}

func MatchPrediction(prediction, groundTruth string, expectedKeywords []string) bool {
	predLower := strings.ToLower(prediction)

	if len(expectedKeywords) > 0 {
		matched := 0
		for _, kw := range expectedKeywords {
			if strings.Contains(predLower, strings.ToLower(kw)) {
				matched++
			}
		}
		threshold := float64(matched) / float64(len(expectedKeywords))
		return threshold >= 0.5
	}

	gtLower := strings.ToLower(groundTruth)
	gtKeywords := extractKeywords(gtLower)
	if len(gtKeywords) == 0 {
		return strings.Contains(predLower, gtLower)
	}

	matched := 0
	for _, kw := range gtKeywords {
		if strings.Contains(predLower, kw) {
			matched++
		}
	}

	threshold := float64(matched) / float64(len(gtKeywords))
	return threshold >= 0.5
}

func extractKeywords(s string) []string {
	words := strings.Fields(s)
	var keywords []string
	stopWords := map[string]bool{
		"的": true, "了": true, "在": true, "是": true, "和": true,
		"与": true, "导致": true, "造成": true, "引起": true,
		"a": true, "an": true, "the": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "shall": true, "can": true,
		"due": true, "to": true, "of": true, "in": true, "on": true,
		"at": true, "for": true, "with": true, "by": true, "from": true,
	}

	for _, w := range words {
		w = strings.Trim(w, ".,;:!?()[]{}\"'")
		if len(w) >= 2 && !stopWords[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}
