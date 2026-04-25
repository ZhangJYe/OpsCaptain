package contextengine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type FaithfulnessResult struct {
	Checked        bool
	Faithful       bool
	Violations     []string
	CheckLatencyMs int64
	Skipped        bool
	SkipReason     string
}

func CheckFaithfulness(ctx context.Context, answer string, documents []ContextItem) FaithfulnessResult {
	if !faithfulnessEnabled() {
		return FaithfulnessResult{Skipped: true, SkipReason: "disabled"}
	}
	if strings.TrimSpace(answer) == "" {
		return FaithfulnessResult{Skipped: true, SkipReason: "empty_answer"}
	}
	if len(documents) == 0 {
		return FaithfulnessResult{Skipped: true, SkipReason: "no_documents"}
	}

	start := time.Now()
	violations := detectViolations(answer, documents)
	return FaithfulnessResult{
		Checked:        true,
		Faithful:       len(violations) == 0,
		Violations:     violations,
		CheckLatencyMs: time.Since(start).Milliseconds(),
	}
}

func detectViolations(answer string, documents []ContextItem) []string {
	var violations []string

	docContent := collectDocContent(documents)

	fabricationPatterns := []struct {
		pattern string
		check   func(answer, docs string) bool
	}{
		{
			"ungrounded_numbers",
			func(answer, docs string) bool {
				return containsSpecificNumbers(answer) && !numbersFoundInDocs(answer, docs)
			},
		},
	}

	for _, fp := range fabricationPatterns {
		if fp.check(answer, docContent) {
			violations = append(violations, fp.pattern)
		}
	}

	if hasCitationMarkers(answer) {
		maxCited := countCitedDocuments(answer)
		if maxCited > len(documents) {
			violations = append(violations, fmt.Sprintf("citation_out_of_range: cited [%d] but only %d documents available", maxCited, len(documents)))
		}
	}

	return violations
}

func collectDocContent(documents []ContextItem) string {
	parts := make([]string, 0, len(documents))
	for _, doc := range documents {
		parts = append(parts, doc.Content)
	}
	return strings.Join(parts, "\n")
}

func containsSpecificNumbers(text string) bool {
	digitRuns := 0
	inDigit := false
	for _, r := range text {
		if r >= '0' && r <= '9' {
			if !inDigit {
				inDigit = true
				digitRuns++
			}
		} else {
			inDigit = false
		}
	}
	return digitRuns >= 3
}

func numbersFoundInDocs(answer, docs string) bool {
	inDigit := false
	start := 0
	matched := 0
	total := 0
	for i, r := range answer {
		if r >= '0' && r <= '9' {
			if !inDigit {
				start = i
				inDigit = true
			}
		} else {
			if inDigit {
				num := answer[start:i]
				if len(num) >= 2 {
					total++
					if strings.Contains(docs, num) {
						matched++
					}
				}
				inDigit = false
			}
		}
	}
	if inDigit {
		num := answer[start:]
		if len(num) >= 2 {
			total++
			if strings.Contains(docs, num) {
				matched++
			}
		}
	}
	if total == 0 {
		return true
	}
	return float64(matched)/float64(total) >= 0.5
}

func hasCitationMarkers(text string) bool {
	return strings.Contains(text, "[1]") || strings.Contains(text, "[2]")
}

func countCitedDocuments(text string) int {
	maxCited := 0
	for i := 1; i <= 20; i++ {
		marker := fmt.Sprintf("[%d]", i)
		if strings.Contains(text, marker) {
			maxCited = i
		}
	}
	return maxCited
}

func faithfulnessEnabled() bool {
	v, err := g.Cfg().Get(context.Background(), "rag.faithfulness_check")
	if err != nil {
		return false
	}
	return v.Bool()
}
