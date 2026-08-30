package evalharness

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

const EvidencePayloadSchema = "evidence-eval/v1"

type EvidenceAdapter struct{}

type EvidenceClaim struct {
	Text             string   `json:"text"`
	CitationIDs      []string `json:"citation_ids"`
	RequiresEvidence bool     `json:"requires_evidence"`
}
type EvidenceCitation struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	TraceID string `json:"trace_id"`
	Text    string `json:"text"`
}
type EvidenceLink struct {
	CitationID string `json:"citation_id"`
	Text       string `json:"text"`
}
type EvidencePayload struct {
	Claims           []EvidenceClaim    `json:"claims"`
	Citations        []EvidenceCitation `json:"citations"`
	Evidence         []EvidenceLink     `json:"evidence"`
	ExpectedKeywords []string           `json:"expected_keywords,omitempty"`
}
type evidenceCaseDomain struct {
	Diagnostic         bool     `json:"diagnostic"`
	RequiresEvidence   bool     `json:"requires_evidence"`
	Claims             int      `json:"claims"`
	SupportedClaims    int      `json:"supported_claims"`
	ValidCitations     int      `json:"valid_citations"`
	TraceableCitations int      `json:"traceable_citations"`
	RelevantEvidence   int      `json:"relevant_evidence"`
	UnsupportedClaims  []string `json:"unsupported_claims,omitempty"`
}

func NewEvidenceAdapter() *EvidenceAdapter       { return &EvidenceAdapter{} }
func (a *EvidenceAdapter) Name() SuiteName       { return SuiteEvidence }
func (a *EvidenceAdapter) PayloadSchema() string { return EvidencePayloadSchema }
func (a *EvidenceAdapter) Validate(_ SuiteConfig, _ DatasetRole, profile Profile) error {
	return RejectLiveProfile(profile)
}
func (a *EvidenceAdapter) RunCase(_ context.Context, evalCase CaseEnvelope) CaseResult {
	start := time.Now()
	var payload EvidencePayload
	if err := json.Unmarshal(evalCase.Payload, &payload); err != nil {
		return failedCase(evalCase.ID, "evidence", err)
	}
	citations := make(map[string]EvidenceCitation, len(payload.Citations))
	links := make(map[string][]EvidenceLink)
	domain := evidenceCaseDomain{Diagnostic: true, Claims: len(payload.Claims)}
	for _, citation := range payload.Citations {
		citations[citation.ID] = citation
		if citation.ID != "" && citation.Source != "" {
			domain.ValidCitations++
		}
		if citation.ID != "" && citation.Source != "" && citation.TraceID != "" {
			domain.TraceableCitations++
		}
	}
	for _, link := range payload.Evidence {
		links[link.CitationID] = append(links[link.CitationID], link)
	}
	for _, claim := range payload.Claims {
		if claim.RequiresEvidence {
			domain.RequiresEvidence = true
		}
		supported := !claim.RequiresEvidence
		for _, id := range claim.CitationIDs {
			citation, ok := citations[id]
			if !ok || citation.Source == "" || len(links[id]) == 0 {
				continue
			}
			supported = true
			text := strings.ToLower(citation.Text)
			for _, link := range links[id] {
				text += " " + strings.ToLower(link.Text)
			}
			for _, keyword := range payload.ExpectedKeywords {
				if strings.Contains(text, strings.ToLower(keyword)) {
					domain.RelevantEvidence++
				}
			}
		}
		if supported {
			domain.SupportedClaims++
		} else {
			domain.UnsupportedClaims = append(domain.UnsupportedClaims, claim.Text)
		}
	}
	matched := len(domain.UnsupportedClaims) == 0 && domain.ValidCitations == domain.TraceableCitations
	status := StatusSucceeded
	if !matched {
		status = StatusFailed
	}
	ids := make([]string, 0, len(citations))
	for id := range citations {
		ids = append(ids, id)
	}
	return CaseResult{CaseID: evalCase.ID, Status: status, Matched: matched, Latency: time.Since(start), TraceComplete: domain.ValidCitations == domain.TraceableCitations, EvidenceCount: len(payload.Evidence), EvidenceIDs: ids, FailurePhase: "evidence", Domain: MarshalDomain(domain)}
}
func (a *EvidenceAdapter) Aggregate(results []CaseResult) (string, json.RawMessage, []GateResult, error) {
	claims, supported, citations, traceable, relevant := 0, 0, 0, 0, 0
	for _, result := range results {
		var domain evidenceCaseDomain
		if len(result.Domain) == 0 {
			continue
		}
		if err := json.Unmarshal(result.Domain, &domain); err != nil {
			return "", nil, nil, err
		}
		claims += domain.Claims
		supported += domain.SupportedClaims
		citations += domain.ValidCitations
		traceable += domain.TraceableCitations
		relevant += domain.RelevantEvidence
	}
	metrics := map[string]float64{"claim_support_rate": ratioInt(supported, claims), "citation_traceability": ratioInt(traceable, citations), "relevant_evidence_count": float64(relevant)}
	claimSupport := metrics["claim_support_rate"]
	citationTraceability := metrics["citation_traceability"]
	gates := []GateResult{
		{Name: "claim_support", Layer: "domain", Suite: SuiteEvidence, Metric: "claim_support_rate", Operator: "==", Threshold: 1, Actual: &claimSupport, Severity: GateBlocking, Passed: claimSupport == 1},
		{Name: "citation_traceability", Layer: "domain", Suite: SuiteEvidence, Metric: "citation_traceability", Operator: "==", Threshold: 1, Actual: &citationTraceability, Severity: GateBlocking, Passed: citationTraceability == 1},
	}
	return "evidence-metrics/v1", MarshalDomain(metrics), gates, nil
}

func ratioInt(numerator, denominator int) float64 {
	if denominator == 0 {
		return 1
	}
	return float64(numerator) / float64(denominator)
}
