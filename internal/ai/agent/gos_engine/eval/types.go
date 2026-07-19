package eval

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"SuperBizAgent/internal/ai/protocol"
)

type EvalCase struct {
	ID                       string   `json:"id"`
	Domain                   string   `json:"domain"`
	Scenario                 string   `json:"scenario"`
	Symptom                  string   `json:"symptom"`
	GroundTruth              string   `json:"ground_truth"`
	ExpectedKeywords         []string `json:"expected_keywords,omitempty"`
	ExpectedCauseKeywords    []string `json:"expected_cause_keywords,omitempty"`
	ExpectedEntityKeywords   []string `json:"expected_entity_keywords,omitempty"`
	ExpectedEvidenceKeywords []string `json:"expected_evidence_keywords,omitempty"`
	ExpectedStatus           string   `json:"expected_status,omitempty"`
	ExpectedFailurePhase     string   `json:"expected_failure_phase,omitempty"`
	RequireRefine            bool     `json:"require_refine,omitempty"`
	RequireBacktrack         bool     `json:"require_backtrack,omitempty"`
	Notes                    string   `json:"notes,omitempty"`
}

type EvalResult struct {
	CaseID               string         `json:"case_id"`
	Symptom              string         `json:"symptom"`
	Prediction           string         `json:"prediction"`
	GroundTruth          string         `json:"ground_truth"`
	Matched              bool           `json:"matched"`
	Latency              time.Duration  `json:"latency"`
	LLMCalls             int            `json:"llm_calls"`
	ToolCalls            int            `json:"tool_calls"`
	RAGCalls             int            `json:"rag_calls"`
	Status               string         `json:"status"`
	ExpectedStatus       string         `json:"expected_status,omitempty"`
	StatusMatched        bool           `json:"status_matched"`
	EvidenceCount        int            `json:"evidence_count"`
	RawEvidenceCount     int            `json:"raw_evidence_count"`
	EvidenceSourceCounts map[string]int `json:"evidence_source_counts,omitempty"`
	RelevantEvidence     int            `json:"relevant_evidence"`
	ExpectedEvidence     int            `json:"expected_evidence"`
	CoveredEvidence      int            `json:"covered_evidence"`
	TraceComplete        bool           `json:"trace_complete"`
	GraphValid           bool           `json:"graph_valid"`
	Refined              bool           `json:"refined"`
	Backtracked          bool           `json:"backtracked"`
	BacktrackRequired    bool           `json:"backtrack_required"`
	PrematureStop        bool           `json:"premature_stop"`
	FailurePhase         string         `json:"failure_phase,omitempty"`
	ExpectedFailurePhase string         `json:"expected_failure_phase,omitempty"`
	FailurePhaseMatched  bool           `json:"failure_phase_matched"`
	ContractMatched      bool           `json:"contract_matched"`
	Scenario             string         `json:"scenario,omitempty"`
	DegradationReason    string         `json:"degradation_reason,omitempty"`
}

type EvalMetrics struct {
	TotalCases         int            `json:"total_cases"`
	Succeeded          int            `json:"succeeded"`
	Failed             int            `json:"failed"`
	Degraded           int            `json:"degraded"`
	Matched            int            `json:"matched"`
	Accuracy           float64        `json:"accuracy"`
	RootCauseAccuracy  float64        `json:"root_cause_accuracy"`
	EvidencePrecision  float64        `json:"evidence_precision"`
	EvidenceCoverage   float64        `json:"evidence_coverage"`
	BacktrackSuccess   float64        `json:"backtrack_success"`
	PrematureStopRate  float64        `json:"premature_stop_rate"`
	GraphValidity      float64        `json:"graph_validity"`
	AvgLatency         time.Duration  `json:"avg_latency"`
	P50Latency         time.Duration  `json:"p50_latency"`
	P95Latency         time.Duration  `json:"p95_latency"`
	AvgLLMCalls        float64        `json:"avg_llm_calls"`
	AvgToolCalls       float64        `json:"avg_tool_calls"`
	AvgRAGCalls        float64        `json:"avg_rag_calls"`
	DegradationRate    float64        `json:"degradation_rate"`
	Traceability       float64        `json:"traceability"`
	ContractCompliance float64        `json:"contract_compliance"`
	PerStatus          map[string]int `json:"per_status"`
	FailuresByPhase    map[string]int `json:"failures_by_phase"`
	totalLatency       time.Duration
	totalLLMCalls      int
	totalToolCalls     int
	totalRAGCalls      int
	traceComplete      int
	evidencePresent    int
	totalEvidence      int
	relevantEvidence   int
	expectedEvidence   int
	coveredEvidence    int
	backtrackRequired  int
	backtrackSucceeded int
	prematureStops     int
	graphValid         int
	contractsMatched   int
	latencies          []time.Duration
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
		PerStatus:       make(map[string]int),
		FailuresByPhase: make(map[string]int),
	}
}

// IsPrematureStop measures an invalid early terminal decision, not diagnosis accuracy.
// A wrong diagnosis is accounted for by root-cause accuracy and report-phase failure.
func IsPrematureStop(
	status string,
	statusMatches bool,
	requireRefine bool,
	refined bool,
	requireBacktrack bool,
	backtracked bool,
) bool {
	return status == string(protocol.ResultStatusSucceeded) &&
		(!statusMatches || requireRefine && !refined || requireBacktrack && !backtracked)
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
	m.totalToolCalls += r.ToolCalls
	m.totalRAGCalls += r.RAGCalls
	m.latencies = append(m.latencies, r.Latency)
	m.totalEvidence += r.EvidenceCount
	m.relevantEvidence += r.RelevantEvidence
	m.expectedEvidence += r.ExpectedEvidence
	m.coveredEvidence += r.CoveredEvidence

	if r.EvidenceCount > 0 {
		m.evidencePresent++
	}

	if r.TraceComplete {
		m.traceComplete++
	}
	if r.ContractMatched {
		m.contractsMatched++
	}
	if r.GraphValid {
		m.graphValid++
	}
	if r.BacktrackRequired {
		m.backtrackRequired++
		if r.Backtracked && r.Matched && r.Status == string(protocol.ResultStatusSucceeded) && r.GraphValid {
			m.backtrackSucceeded++
		}
	}
	if r.PrematureStop {
		m.prematureStops++
	}
	if r.FailurePhase != "" {
		m.FailuresByPhase[r.FailurePhase]++
	}
}

func (m *EvalMetrics) Finalize() {
	if m.TotalCases == 0 {
		return
	}

	m.Accuracy = float64(m.Matched) / float64(m.TotalCases)
	m.RootCauseAccuracy = m.Accuracy
	if m.totalEvidence > 0 {
		m.EvidencePrecision = float64(m.relevantEvidence) / float64(m.totalEvidence)
	}
	if m.expectedEvidence > 0 {
		m.EvidenceCoverage = float64(m.coveredEvidence) / float64(m.expectedEvidence)
	} else {
		m.EvidenceCoverage = float64(m.evidencePresent) / float64(m.TotalCases)
	}
	if m.backtrackRequired > 0 {
		m.BacktrackSuccess = float64(m.backtrackSucceeded) / float64(m.backtrackRequired)
	} else {
		m.BacktrackSuccess = 1
	}
	m.PrematureStopRate = float64(m.prematureStops) / float64(m.TotalCases)
	m.GraphValidity = float64(m.graphValid) / float64(m.TotalCases)
	m.DegradationRate = float64(m.Degraded) / float64(m.TotalCases)
	m.Traceability = float64(m.traceComplete) / float64(m.TotalCases)
	m.ContractCompliance = float64(m.contractsMatched) / float64(m.TotalCases)

	m.AvgLatency = m.totalLatency / time.Duration(m.TotalCases)
	m.AvgLLMCalls = float64(m.totalLLMCalls) / float64(m.TotalCases)
	m.AvgToolCalls = float64(m.totalToolCalls) / float64(m.TotalCases)
	m.AvgRAGCalls = float64(m.totalRAGCalls) / float64(m.TotalCases)
	latencies := append([]time.Duration(nil), m.latencies...)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	m.P50Latency = percentileDuration(latencies, 0.50)
	m.P95Latency = percentileDuration(latencies, 0.95)
}

func percentileDuration(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(sorted))*percentile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
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

func MatchCasePrediction(prediction string, evalCase EvalCase) bool {
	if len(evalCase.ExpectedCauseKeywords) == 0 && len(evalCase.ExpectedEntityKeywords) == 0 {
		return MatchPrediction(prediction, evalCase.GroundTruth, evalCase.ExpectedKeywords)
	}
	if len(evalCase.ExpectedCauseKeywords) == 0 || len(evalCase.ExpectedEntityKeywords) == 0 {
		return false
	}
	normalizedPrediction := normalizeMatchText(prediction)
	return containsAnyNormalized(normalizedPrediction, evalCase.ExpectedCauseKeywords) &&
		containsAnyNormalized(normalizedPrediction, evalCase.ExpectedEntityKeywords)
}

func containsAnyNormalized(normalizedText string, values []string) bool {
	for _, value := range values {
		normalized := normalizeMatchText(value)
		if normalized != "" && strings.Contains(normalizedText, normalized) {
			return true
		}
	}
	return false
}

func normalizeMatchText(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
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
