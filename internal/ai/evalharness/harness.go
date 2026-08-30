package evalharness

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Harness struct {
	registry *Registry
	now      func() time.Time
}

func NewHarness(registry *Registry) *Harness {
	return &Harness{registry: registry, now: time.Now}
}

func (h *Harness) Run(ctx context.Context, manifest *Manifest) (*Report, error) {
	if h == nil || h.registry == nil {
		return nil, fmt.Errorf("registry is required")
	}
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	if err := ValidateRegressionCorpus(manifest); err != nil {
		return nil, err
	}
	started := h.now()
	if manifest.Budget.TotalTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, manifest.Budget.TotalTimeout)
		defer cancel()
	}
	report := &Report{
		SchemaVersion: ReportSchemaVersion, RunID: uuid.NewString(), RunName: manifest.RunName,
		StartedAt: started, DatasetRole: manifest.DatasetRole, LabelSource: manifest.LabelSource, Profile: manifest.Profile,
		TruthBoundary: TruthBoundary(manifest.Profile), Dependencies: manifest.Dependencies, Budget: manifest.Budget,
	}
	if manifest.ExternalCorpusManifest != "" {
		corpus, err := LoadExternalCorpusManifest(resolveManifestPath(manifest.SourcePath, manifest.ExternalCorpusManifest))
		if err != nil {
			return nil, fmt.Errorf("load external corpus provenance: %w", err)
		}
		report.ExternalCorpus = &corpus.Provenance
		for _, item := range corpus.Coverage {
			if item.Gap > 0 {
				report.CoverageGaps = append(report.CoverageGaps, item)
			}
		}
	}
	runBudget := NewBudget(manifest.Budget)
	for _, suiteConfig := range manifest.Suites {
		if !suiteConfig.Enabled {
			report.Suites = append(report.Suites, SuiteReport{Name: suiteConfig.Name, Status: StatusSkipped})
			continue
		}
		suiteReport := h.runSuite(ctx, manifest, suiteConfig, runBudget)
		report.Suites = append(report.Suites, suiteReport)
		report.Failures = append(report.Failures, suiteReport.Failures...)
		if suiteReport.Status == StatusBudgetExceeded {
			break
		}
		if suiteReport.Status == StatusFailed && !manifest.ContinueOnError {
			break
		}
	}
	report.Usage = runBudget.Usage()
	fingerprints, fingerprintErr := ManifestFingerprints(manifest, report.Suites)
	if fingerprintErr != nil {
		return nil, fingerprintErr
	}
	report.Fingerprints = fingerprints
	report.PlanGoSComparison = collectPlanGoSComparison(report.Suites)
	report.CrossSuiteGates = EvaluateCrossSuiteGates(report.Suites)
	report.Status = reportStatus(report.Suites, report.CrossSuiteGates)
	report.FinishedAt = h.now()
	return report, nil
}

func (h *Harness) runSuite(ctx context.Context, manifest *Manifest, cfg SuiteConfig, runBudget *Budget) SuiteReport {
	report := SuiteReport{Name: cfg.Name, Status: StatusFailed, DomainSchema: cfg.PayloadSchema}
	adapter, ok := h.registry.Get(cfg.Name)
	if !ok {
		report.Failures = []Failure{{Suite: cfg.Name, Phase: "plan", Reason: "adapter not registered"}}
		return report
	}
	if adapter.PayloadSchema() != cfg.PayloadSchema && !supportsPayloadSchema(adapter, cfg.PayloadSchema) {
		report.Failures = []Failure{{Suite: cfg.Name, Phase: "plan", Reason: "adapter payload schema mismatch"}}
		return report
	}
	if err := adapter.Validate(cfg, manifest.DatasetRole, manifest.Profile); err != nil {
		report.Failures = []Failure{{Suite: cfg.Name, Phase: "plan", Reason: err.Error()}}
		return report
	}
	cases, datasetHash, err := LoadCases(manifest.SourcePath, cfg)
	if err != nil {
		report.Failures = []Failure{{Suite: cfg.Name, Phase: "plan", Reason: err.Error()}}
		return report
	}
	report.Fingerprints.Dataset = datasetHash
	if cfg.Budget.MaxCases > 0 && len(cases) > cfg.Budget.MaxCases {
		cases = cases[:cfg.Budget.MaxCases]
	}
	suiteBudget := NewBudget(cfg.Budget)
	type indexedResult struct {
		index     int
		result    CaseResult
		budgetErr error
	}
	concurrency := cfg.Budget.Concurrency
	if concurrency <= 0 {
		concurrency = manifest.Budget.Concurrency
	}
	if concurrency <= 0 {
		concurrency = 1
	}
	caseResults := make([]CaseResult, len(cases))
	completed := make([]bool, len(cases))
	resultCh := make(chan indexedResult, len(cases))
	sem := make(chan struct{}, concurrency)
	suiteCtx, cancelSuite := context.WithCancel(ctx)
	defer cancelSuite()
	var wg sync.WaitGroup
	for index, evalCase := range cases {
		if err := ctx.Err(); err != nil {
			report.Status = StatusBudgetExceeded
			report.Failures = append(report.Failures, Failure{Suite: cfg.Name, CaseID: evalCase.ID, Phase: "report", Reason: err.Error()})
			break
		}
		if err := runBudget.ReserveCase(); err != nil {
			report.Status = StatusBudgetExceeded
			report.Failures = append(report.Failures, Failure{Suite: cfg.Name, CaseID: evalCase.ID, Phase: "report", Reason: err.Error()})
			break
		}
		if err := suiteBudget.ReserveCase(); err != nil {
			report.Status = StatusBudgetExceeded
			report.Failures = append(report.Failures, Failure{Suite: cfg.Name, CaseID: evalCase.ID, Phase: "report", Reason: err.Error()})
			break
		}
		wg.Add(1)
		go func(index int, evalCase CaseEnvelope) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-suiteCtx.Done():
				resultCh <- indexedResult{index: index, result: CaseResult{CaseID: evalCase.ID, Status: StatusBudgetExceeded, Reason: suiteCtx.Err().Error()}}
				return
			}
			defer func() { <-sem }()
			if err := suiteCtx.Err(); err != nil {
				resultCh <- indexedResult{index: index, result: CaseResult{CaseID: evalCase.ID, Status: StatusBudgetExceeded, Reason: err.Error()}}
				return
			}
			caseCtx := suiteCtx
			cancel := func() {}
			if cfg.Budget.CaseTimeout > 0 {
				caseCtx, cancel = context.WithTimeout(suiteCtx, cfg.Budget.CaseTimeout)
			}
			result := adapter.RunCase(caseCtx, evalCase)
			cancel()
			if result.FailurePhase != "" || result.Status != StatusSucceeded {
				result.FailurePhase = NormalizeFailurePhase(result.FailurePhase, result.Reason)
			}
			suiteBudgetErr := suiteBudget.Add(result.Usage)
			runBudgetErr := runBudget.Add(result.Usage)
			budgetErr := errors.Join(suiteBudgetErr, runBudgetErr)
			if budgetErr != nil {
				cancelSuite()
			}
			resultCh <- indexedResult{index: index, result: result, budgetErr: budgetErr}
		}(index, evalCase)
	}
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	for item := range resultCh {
		if item.budgetErr != nil {
			item.result.Status = StatusBudgetExceeded
			item.result.FailurePhase = "report"
			item.result.Reason = item.budgetErr.Error()
		}
		caseResults[item.index] = item.result
		completed[item.index] = true
		if item.budgetErr != nil {
			report.Status = StatusBudgetExceeded
		}
	}
	for index, result := range caseResults {
		if !completed[index] {
			continue
		}
		report.Cases = append(report.Cases, result)
		if result.Status == StatusFailed || result.Status == StatusBudgetExceeded {
			report.Failures = append(report.Failures, resultFailure(cfg.Name, result, result.Reason))
		}
	}
	report.CommonMetrics = AggregateCommon(report.Cases)
	domainSchema, domainMetrics, domainGates, aggregateErr := adapter.Aggregate(report.Cases)
	if aggregateErr != nil {
		report.Status = StatusFailed
		report.Failures = append(report.Failures, Failure{Suite: cfg.Name, Phase: "report", Reason: aggregateErr.Error()})
		return report
	}
	report.DomainSchema = domainSchema
	report.DomainMetrics = domainMetrics
	report.Gates = append(report.Gates, domainGates...)
	commonMetrics := CommonMetricMap(report.CommonMetrics)
	domainMetricMap := DomainMetricMap(report.DomainMetrics)
	for _, gate := range append(append([]GateSpec(nil), manifest.Gates...), cfg.Gates...) {
		metrics := commonMetrics
		layer := "common"
		if _, ok := metrics[gate.Metric]; !ok {
			metrics = domainMetricMap
			layer = "domain"
		}
		evaluated := EvaluateGate(gate, metrics, layer)
		evaluated.Suite = cfg.Name
		if !evaluated.Passed {
			for _, result := range report.Cases {
				if !result.Matched || result.Status != StatusSucceeded {
					evaluated.CaseRefs = append(evaluated.CaseRefs, result.CaseID)
				}
			}
		}
		report.Gates = append(report.Gates, evaluated)
	}
	if report.Status != StatusBudgetExceeded {
		report.Status = suiteStatus(report.Cases, report.Gates)
	}
	return report
}

func supportsPayloadSchema(adapter Adapter, schema string) bool {
	aware, ok := adapter.(SchemaAwareAdapter)
	if !ok {
		return false
	}
	for _, supported := range aware.SupportedPayloadSchemas() {
		if supported == schema {
			return true
		}
	}
	return false
}

func collectPlanGoSComparison(suites []SuiteReport) []PlanGoSComparison {
	var plan, gos []CaseResult
	for _, suite := range suites {
		switch suite.Name {
		case SuitePlan:
			plan = suite.Cases
		case SuiteGoS:
			gos = suite.Cases
		}
	}
	if len(plan) == 0 || len(gos) == 0 {
		return nil
	}
	return ComparePlanGoSCases(plan, gos)
}

func suiteStatus(results []CaseResult, gates []GateResult) SuiteStatus {
	status := StatusSucceeded
	for _, result := range results {
		switch result.Status {
		case StatusFailed:
			return StatusFailed
		case StatusBudgetExceeded:
			status = StatusBudgetExceeded
		case StatusDegraded:
			if status == StatusSucceeded {
				status = StatusDegraded
			}
		}
	}
	for _, gate := range gates {
		if gate.Severity == GateBlocking && !gate.Passed {
			return StatusFailed
		}
	}
	return status
}

func resultFailure(suite SuiteName, result CaseResult, reason string) Failure {
	return Failure{Suite: suite, CaseID: result.CaseID, Phase: NormalizeFailurePhase(result.FailurePhase, reason), Reason: reason, TraceID: result.TraceID, EvidenceIDs: result.EvidenceIDs}
}

func TruthBoundary(profile Profile) string {
	switch profile {
	case ProfileDeterministic:
		return "deterministic fixtures only; not live or production validation"
	case ProfileRecorded:
		return "recorded evidence replay; not live or production validation"
	case ProfileLive:
		return "controlled live dependencies; production effect remains unverified unless separately evidenced"
	default:
		return "unknown validation profile"
	}
}

func QueryHash(query string) string {
	digest := sha256.Sum256([]byte(query))
	return hex.EncodeToString(digest[:])
}

func MarshalDomain(value any) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}
