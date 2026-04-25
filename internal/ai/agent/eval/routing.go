package eval

import (
	"context"
	"fmt"
)

func EvaluateRouting(ctx context.Context, cases []DiagCase, runner Runner) (RoutingReport, error) {
	if runner == nil {
		return RoutingReport{}, fmt.Errorf("runner is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	report := RoutingReport{
		Summary: RoutingSummary{Cases: len(cases)},
		Results: make([]RoutingCaseResult, 0, len(cases)),
	}

	var precisionTotal, recallTotal, f1Total float64
	for _, tc := range cases {
		result, err := runner.Run(ctx, tc.Query)
		item := RoutingCaseResult{
			CaseID:          tc.ID,
			Query:           tc.Query,
			ExpectedIntent:  tc.ExpectedIntent,
			ExpectedDomains: append([]string(nil), tc.ExpectedDomains...),
		}
		if err != nil {
			item.Failure = err.Error()
			report.Summary.Failed++
			report.Results = append(report.Results, item)
			continue
		}
		if result == nil {
			item.Failure = "runner returned nil result"
			report.Summary.Failed++
			report.Results = append(report.Results, item)
			continue
		}

		item.ActualIntent = result.Intent
		item.ActualDomains = append([]string(nil), result.Domains...)
		item.IntentMatched = tc.ExpectedIntent == "" || result.Intent == tc.ExpectedIntent
		item.Fallback = metadataBool(result.Metadata, "triage_fallback")
		item.DomainPrecision, item.DomainRecall, item.DomainF1 = domainScore(tc.ExpectedDomains, result.Domains)

		if item.IntentMatched {
			report.Summary.IntentCorrect++
		}
		if item.Fallback {
			report.Summary.FallbackCount++
		}
		precisionTotal += item.DomainPrecision
		recallTotal += item.DomainRecall
		f1Total += item.DomainF1
		report.Results = append(report.Results, item)
	}

	evaluated := len(cases)
	if evaluated > 0 {
		report.Summary.IntentAccuracy = float64(report.Summary.IntentCorrect) / float64(evaluated)
		report.Summary.DomainPrecision = precisionTotal / float64(evaluated)
		report.Summary.DomainRecall = recallTotal / float64(evaluated)
		report.Summary.DomainF1 = f1Total / float64(evaluated)
		report.Summary.FallbackRate = float64(report.Summary.FallbackCount) / float64(evaluated)
	}
	return report, nil
}

func domainScore(expected, actual []string) (float64, float64, float64) {
	if len(expected) == 0 && len(actual) == 0 {
		return 1, 1, 1
	}
	if len(expected) == 0 || len(actual) == 0 {
		return 0, 0, 0
	}
	hits := 0
	for _, domain := range actual {
		if containsString(expected, domain) {
			hits++
		}
	}
	precision := float64(hits) / float64(len(actual))
	recall := float64(hits) / float64(len(expected))
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return precision, recall, f1
}

func metadataBool(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	switch value := metadata[key].(type) {
	case bool:
		return value
	case string:
		return value == "true"
	default:
		return false
	}
}
