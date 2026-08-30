package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"SuperBizAgent/internal/ai/evalharness"
	"SuperBizAgent/internal/app/evaladapter"
)

const (
	exitSuccess    = 0
	exitInvalid    = 2
	exitRunFailed  = 3
	exitGateFailed = 4
)

func main() { os.Exit(runCLI(os.Args[1:])) }

func runCLI(args []string) int {
	if len(args) == 0 {
		printUsage()
		return exitInvalid
	}
	switch args[0] {
	case "corpus":
		return corpusCommand(args[1:])
	case "validate":
		return validateCommand(args[1:])
	case "run":
		return runCommand(args[1:], false)
	case "gate":
		return runCommand(args[1:], true)
	case "compare":
		return compareCommand(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", args[0])
		printUsage()
		return exitInvalid
	}
}

func corpusCommand(args []string) int {
	if len(args) == 0 {
		printUsage()
		return exitInvalid
	}
	switch args[0] {
	case "prepare":
		flags := flag.NewFlagSet("corpus prepare", flag.ContinueOnError)
		source := flags.String("source", "", "AIOps2025 source directory")
		output := flags.String("output", "", "external corpus output directory")
		version := flags.String("version", "", "corpus version")
		if flags.Parse(args[1:]) != nil || *source == "" || *output == "" {
			return exitInvalid
		}
		projectRoot, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitInvalid
		}
		manifest, err := evalharness.PrepareAIOps2025Corpus(evalharness.CorpusPrepareOptions{SourceDir: *source, OutputDir: *output, Version: *version, ProjectRoot: projectRoot})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitInvalid
		}
		encoded, _ := json.MarshalIndent(manifest, "", "  ")
		fmt.Println(string(encoded))
		return exitSuccess
	case "validate":
		flags := flag.NewFlagSet("corpus validate", flag.ContinueOnError)
		manifestPath := flags.String("manifest", "", "external corpus manifest path")
		if flags.Parse(args[1:]) != nil || *manifestPath == "" {
			return exitInvalid
		}
		manifest, err := evalharness.ValidateExternalCorpus(*manifestPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitInvalid
		}
		encoded, _ := json.MarshalIndent(manifest, "", "  ")
		fmt.Println(string(encoded))
		return exitSuccess
	default:
		fmt.Fprintf(os.Stderr, "unknown corpus command %q\n", args[0])
		return exitInvalid
	}
}

func validateCommand(args []string) int {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "", "manifest path")
	if flags.Parse(args) != nil || *manifestPath == "" {
		return exitInvalid
	}
	manifest, err := evalharness.LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalid
	}
	if err := evalharness.ValidateRegressionCorpus(manifest); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalid
	}
	for _, suite := range manifest.Suites {
		if !suite.Enabled {
			continue
		}
		if _, _, err := evalharness.LoadCases(manifest.SourcePath, suite); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return exitInvalid
		}
	}
	fmt.Printf("manifest valid: %s\n", manifest.RunName)
	return exitSuccess
}

func runCommand(args []string, gate bool) int {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "", "manifest path")
	outputDir := flags.String("output-dir", "", "report output directory")
	if flags.Parse(args) != nil || *manifestPath == "" {
		return exitInvalid
	}
	manifest, err := evalharness.LoadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalid
	}
	registry, err := evaladapter.NewDefaultRegistry(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitRunFailed
	}
	report, err := evalharness.NewHarness(registry).Run(context.Background(), manifest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitRunFailed
	}
	dir := *outputDir
	if dir == "" {
		dir = manifest.ReportDir
	}
	if dir == "" {
		dir = filepath.Join(filepath.Dir(*manifestPath), "..", "reports")
	}
	jsonPath, markdownPath, err := evalharness.WriteReport(report, dir, manifest.Redaction)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitRunFailed
	}
	fmt.Printf("json_report=%s\nmarkdown_report=%s\nstatus=%s\n", jsonPath, markdownPath, report.Status)
	if report.Status == evalharness.StatusFailed || report.Status == evalharness.StatusBudgetExceeded || gate && (report.Status == evalharness.StatusSkipped || report.Status == evalharness.StatusDegraded) {
		if gate {
			return exitGateFailed
		}
		return exitRunFailed
	}
	return exitSuccess
}

func compareCommand(args []string) int {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	baselinePath := flags.String("baseline", "", "baseline report")
	candidatePath := flags.String("candidate", "", "candidate report")
	if flags.Parse(args) != nil || *baselinePath == "" || *candidatePath == "" {
		return exitInvalid
	}
	baseline, err := loadReport(*baselinePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalid
	}
	candidate, err := loadReport(*candidatePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalid
	}
	results, err := evalharness.CompareReports(baseline, candidate)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitInvalid
	}
	encoded, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(encoded))
	for _, result := range results {
		if result.Severity == evalharness.GateBlocking && !result.Passed {
			return exitGateFailed
		}
	}
	return exitSuccess
}

func loadReport(path string) (*evalharness.Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var report evalharness.Report
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	if report.SchemaVersion != evalharness.ReportSchemaVersion {
		return nil, errors.New("unsupported report schema")
	}
	return &report, nil
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: eval_harness <corpus|validate|run|gate|compare> [flags]")
}
