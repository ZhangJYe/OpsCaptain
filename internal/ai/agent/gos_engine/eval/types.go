package eval

import (
	"time"

	"SuperBizAgent/internal/ai/protocol"
)

type EvalCase struct {
	ID          string `json:"id"`
	Symptom     string `json:"symptom"`
	GroundTruth string `json:"ground_truth"`
	Notes       string `json:"notes,omitempty"`
}

type EvalResult struct {
	CaseID            string        `json:"case_id"`
	Symptom           string        `json:"symptom"`
	Prediction        string        `json:"prediction"`
	GroundTruth       string        `json:"ground_truth"`
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
	Accuracy         float64        `json:"accuracy"`
	EvidenceCoverage float64        `json:"evidence_coverage"`
	AvgLatency       time.Duration  `json:"avg_latency"`
	AvgLLMCalls      float64        `json:"avg_llm_calls"`
	DegradationRate  float64        `json:"degradation_rate"`
	Traceability     float64        `json:"traceability"`
	PerStatus        map[string]int `json:"per_status"`
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

	if r.EvidenceCount > 0 {
		m.EvidenceCoverage++
	}

	if r.TraceComplete {
		m.Traceability++
	}
}

func (m *EvalMetrics) Finalize() {
	if m.TotalCases == 0 {
		return
	}

	m.Accuracy = float64(m.Succeeded) / float64(m.TotalCases)
	m.EvidenceCoverage = m.EvidenceCoverage / float64(m.TotalCases)
	m.DegradationRate = float64(m.Degraded) / float64(m.TotalCases)
	m.Traceability = m.Traceability / float64(m.TotalCases)
}
