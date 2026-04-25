package eval

import (
	"context"
	"fmt"
	"strings"
)

func RunGoldenCases(ctx context.Context, cases []DiagCase, runner Runner) (GoldenSummary, []GoldenCaseResult, error) {
	if runner == nil {
		return GoldenSummary{}, nil, fmt.Errorf("runner is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	results := make([]GoldenCaseResult, 0, len(cases))
	summary := GoldenSummary{Cases: len(cases)}
	for _, tc := range cases {
		runResult, err := runner.Run(ctx, tc.Query)
		caseResult := GoldenCaseResult{
			CaseID: tc.ID,
			Query:  tc.Query,
			Result: runResult,
		}
		if err != nil {
			caseResult.Failures = append(caseResult.Failures, fmt.Sprintf("runner error: %v", err))
		} else {
			caseResult.Failures = append(caseResult.Failures, validateDiagCase(tc, runResult)...)
		}
		caseResult.Passed = len(caseResult.Failures) == 0
		if caseResult.Passed {
			summary.Passed++
		} else {
			summary.Failed++
		}
		results = append(results, caseResult)
	}
	if summary.Cases > 0 {
		summary.PassRate = float64(summary.Passed) / float64(summary.Cases)
	}
	return summary, results, nil
}

func validateDiagCase(tc DiagCase, result *RunResult) []string {
	if result == nil {
		return []string{"result is nil"}
	}
	var failures []string
	if strings.TrimSpace(tc.ExpectedIntent) != "" && result.Intent != tc.ExpectedIntent {
		failures = append(failures, fmt.Sprintf("intent expected %q got %q", tc.ExpectedIntent, result.Intent))
	}
	if tc.ExpectedDomains != nil {
		failures = append(failures, validateDomains(tc.ExpectedDomains, result.Domains)...)
	}
	for _, expected := range tc.MustMention {
		if !containsText(result.Summary, expected) {
			failures = append(failures, fmt.Sprintf("summary must mention %q", expected))
		}
	}
	for _, forbidden := range tc.MustNotMention {
		if containsText(result.Summary, forbidden) {
			failures = append(failures, fmt.Sprintf("summary must not mention %q", forbidden))
		}
	}
	if strings.TrimSpace(tc.ExpectedAction) != "" && !containsText(result.Summary, tc.ExpectedAction) {
		failures = append(failures, fmt.Sprintf("summary must include expected action %q", tc.ExpectedAction))
	}
	return failures
}

func validateDomains(expected, actual []string) []string {
	var failures []string
	if len(expected) == 0 {
		if len(actual) > 0 {
			failures = append(failures, fmt.Sprintf("domains expected empty got %v", actual))
		}
		return failures
	}
	for _, domain := range expected {
		if !containsString(actual, domain) {
			failures = append(failures, fmt.Sprintf("domains expected to include %q got %v", domain, actual))
		}
	}
	return failures
}

func containsText(text, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(needle))
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
