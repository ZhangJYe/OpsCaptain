package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	agenteval "SuperBizAgent/internal/ai/agent/eval"
)

func main() {
	mode := flag.String("mode", "routing", "routing or golden")
	runnerName := flag.String("runner", "triage", "triage or multi_agent")
	casesPath := flag.String("cases", "internal/ai/agent/eval/testdata/diag_golden.jsonl", "diagnostic cases jsonl path")
	format := flag.String("format", "markdown", "markdown or json")
	outputPath := flag.String("out", "", "optional output file")
	name := flag.String("name", "current-baseline", "report name")
	flag.Parse()

	ctx := context.Background()
	cases, err := agenteval.LoadDiagCasesJSONL(*casesPath)
	exitOnErr(err)

	runner, err := buildRunner(*runnerName)
	exitOnErr(err)

	var output string
	switch strings.ToLower(strings.TrimSpace(*mode)) {
	case "routing":
		report, err := agenteval.EvaluateRouting(ctx, cases, runner)
		exitOnErr(err)
		output, err = encodeRoutingReport(*name, report, *format)
		exitOnErr(err)
	case "golden":
		summary, results, err := agenteval.RunGoldenCases(ctx, cases, runner)
		exitOnErr(err)
		output, err = encodeGoldenReport(*name, summary, results, *format)
		exitOnErr(err)
	default:
		exitOnErr(fmt.Errorf("unsupported mode %q", *mode))
	}

	if strings.TrimSpace(*outputPath) != "" {
		exitOnErr(os.WriteFile(*outputPath, []byte(output), 0o644))
		return
	}
	fmt.Print(output)
}

func buildRunner(name string) (agenteval.Runner, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "triage":
		return agenteval.NewTriageRunner(), nil
	case "multi_agent", "multi-agent", "supervisor":
		return agenteval.NewMultiAgentRunner()
	default:
		return nil, fmt.Errorf("unsupported runner %q", name)
	}
}

func encodeRoutingReport(name string, report agenteval.RoutingReport, format string) (string, error) {
	if strings.EqualFold(format, "json") {
		return marshalJSON(map[string]any{
			"name":       name,
			"created_at": time.Now().Format(time.RFC3339),
			"report":     report,
		})
	}

	var b strings.Builder
	b.WriteString("# 多智能体路由基线报告\n\n")
	b.WriteString(fmt.Sprintf("- 名称：%s\n", name))
	b.WriteString(fmt.Sprintf("- 时间：%s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- Case 数：%d\n", report.Summary.Cases))
	b.WriteString(fmt.Sprintf("- Intent Accuracy：%.2f\n", report.Summary.IntentAccuracy))
	b.WriteString(fmt.Sprintf("- Domain Precision：%.2f\n", report.Summary.DomainPrecision))
	b.WriteString(fmt.Sprintf("- Domain Recall：%.2f\n", report.Summary.DomainRecall))
	b.WriteString(fmt.Sprintf("- Domain F1：%.2f\n", report.Summary.DomainF1))
	b.WriteString(fmt.Sprintf("- Fallback Rate：%.2f（%d/%d）\n\n", report.Summary.FallbackRate, report.Summary.FallbackCount, report.Summary.Cases))
	b.WriteString("| Case | Intent | Domains | Fallback | Query |\n")
	b.WriteString("|---|---|---|---:|---|\n")
	for _, item := range report.Results {
		intent := passMark(item.IntentMatched)
		domains := fmt.Sprintf("P=%.2f R=%.2f F1=%.2f", item.DomainPrecision, item.DomainRecall, item.DomainF1)
		b.WriteString(fmt.Sprintf("| %s | %s %s -> %s | %s | %t | %s |\n",
			item.CaseID,
			intent,
			item.ExpectedIntent,
			item.ActualIntent,
			domains,
			item.Fallback,
			escapeMarkdownCell(item.Query),
		))
	}
	return b.String(), nil
}

func encodeGoldenReport(name string, summary agenteval.GoldenSummary, results []agenteval.GoldenCaseResult, format string) (string, error) {
	if strings.EqualFold(format, "json") {
		return marshalJSON(map[string]any{
			"name":       name,
			"created_at": time.Now().Format(time.RFC3339),
			"summary":    summary,
			"results":    results,
		})
	}

	var b strings.Builder
	b.WriteString("# 多智能体 Golden Case 基线报告\n\n")
	b.WriteString(fmt.Sprintf("- 名称：%s\n", name))
	b.WriteString(fmt.Sprintf("- 时间：%s\n", time.Now().Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("- Case 数：%d\n", summary.Cases))
	b.WriteString(fmt.Sprintf("- 通过率：%.2f（%d/%d）\n\n", summary.PassRate, summary.Passed, summary.Cases))
	b.WriteString("| Case | Result | Failures |\n")
	b.WriteString("|---|---:|---|\n")
	for _, item := range results {
		b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", item.CaseID, passMark(item.Passed), escapeMarkdownCell(strings.Join(item.Failures, "; "))))
	}
	return b.String(), nil
}

func marshalJSON(value any) (string, error) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(body) + "\n", nil
}

func passMark(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func exitOnErr(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
