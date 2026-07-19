package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const DatasetSchemaVersion = "gos-eval-v2"

type DatasetRole string

const (
	DatasetRoleDevelopment DatasetRole = "development"
	DatasetRoleHoldout     DatasetRole = "holdout"
	DatasetRoleRegression  DatasetRole = "regression"
)

type EvalDataset struct {
	SchemaVersion string      `json:"schema_version"`
	Role          DatasetRole `json:"role"`
	Description   string      `json:"description,omitempty"`
	Cases         []EvalCase  `json:"cases"`
}

var requiredDomains = []string{
	"cpu", "memory", "network", "database", "cache", "message_queue", "config_change", "dependency",
}

var requiredScenarios = []string{
	"support_evidence", "refute_evidence", "evidence_conflict", "wrong_initial_hypothesis",
	"drilldown_required", "backtracking_required", "tool_timeout", "rag_empty", "invalid_llm_json",
}

func LoadDataset(filePath string) (*EvalDataset, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var dataset EvalDataset
	if err := json.Unmarshal(data, &dataset); err != nil {
		return nil, err
	}
	if err := ValidateDataset(&dataset); err != nil {
		return nil, err
	}
	return &dataset, nil
}

func ValidateDataset(dataset *EvalDataset) error {
	if dataset == nil {
		return fmt.Errorf("dataset is required")
	}
	if dataset.SchemaVersion != DatasetSchemaVersion {
		return fmt.Errorf("dataset schema_version must be %q", DatasetSchemaVersion)
	}
	switch dataset.Role {
	case DatasetRoleDevelopment, DatasetRoleHoldout, DatasetRoleRegression:
	default:
		return fmt.Errorf("unsupported dataset role %q", dataset.Role)
	}
	if len(dataset.Cases) == 0 {
		return fmt.Errorf("dataset cases are empty")
	}

	ids := make(map[string]bool)
	symptoms := make(map[string]bool)
	for index, evalCase := range dataset.Cases {
		if strings.TrimSpace(evalCase.ID) == "" || strings.TrimSpace(evalCase.Symptom) == "" || strings.TrimSpace(evalCase.GroundTruth) == "" {
			return fmt.Errorf("case %d requires id, symptom and ground_truth", index)
		}
		if strings.TrimSpace(evalCase.Domain) == "" || strings.TrimSpace(evalCase.Scenario) == "" {
			return fmt.Errorf("case %q requires domain and scenario", evalCase.ID)
		}
		if (len(evalCase.ExpectedCauseKeywords) == 0) != (len(evalCase.ExpectedEntityKeywords) == 0) {
			return fmt.Errorf("case %q requires both expected_cause_keywords and expected_entity_keywords", evalCase.ID)
		}
		if ids[evalCase.ID] {
			return fmt.Errorf("duplicate case id %q", evalCase.ID)
		}
		if symptoms[evalCase.Symptom] {
			return fmt.Errorf("duplicate symptom in case %q", evalCase.ID)
		}
		ids[evalCase.ID] = true
		symptoms[evalCase.Symptom] = true
		if evalCase.ExpectedStatus != "" && evalCase.ExpectedStatus != "succeeded" && evalCase.ExpectedStatus != "degraded" && evalCase.ExpectedStatus != "failed" {
			return fmt.Errorf("case %q has invalid expected_status %q", evalCase.ID, evalCase.ExpectedStatus)
		}
		if evalCase.ExpectedFailurePhase != "" && !validFailurePhase(evalCase.ExpectedFailurePhase) {
			return fmt.Errorf("case %q has invalid expected_failure_phase %q", evalCase.ID, evalCase.ExpectedFailurePhase)
		}
		if (evalCase.ExpectedStatus == "degraded" || evalCase.ExpectedStatus == "failed") && evalCase.ExpectedFailurePhase == "" {
			return fmt.Errorf("case %q requires expected_failure_phase for expected_status %q", evalCase.ID, evalCase.ExpectedStatus)
		}
		if evalCase.ExpectedStatus == "succeeded" && evalCase.ExpectedFailurePhase != "" {
			return fmt.Errorf("case %q cannot expect a failure phase when expected_status is succeeded", evalCase.ID)
		}
	}
	return nil
}

func ValidateCorpus(development, holdout *EvalDataset) error {
	if err := ValidateDataset(development); err != nil {
		return fmt.Errorf("development dataset: %w", err)
	}
	if err := ValidateDataset(holdout); err != nil {
		return fmt.Errorf("holdout dataset: %w", err)
	}
	if development.Role != DatasetRoleDevelopment || holdout.Role != DatasetRoleHoldout {
		return fmt.Errorf("corpus requires development and holdout roles")
	}
	if len(development.Cases)+len(holdout.Cases) < 30 {
		return fmt.Errorf("development and holdout corpus must contain at least 30 cases")
	}

	ids := make(map[string]bool)
	symptoms := make(map[string]bool)
	domains := make(map[string]bool)
	scenarios := make(map[string]bool)
	for _, dataset := range []*EvalDataset{development, holdout} {
		for _, evalCase := range dataset.Cases {
			if ids[evalCase.ID] || symptoms[evalCase.Symptom] {
				return fmt.Errorf("development and holdout are not disjoint at case %q", evalCase.ID)
			}
			ids[evalCase.ID] = true
			symptoms[evalCase.Symptom] = true
			domains[evalCase.Domain] = true
			scenarios[evalCase.Scenario] = true
		}
	}
	for _, domain := range requiredDomains {
		if !domains[domain] {
			return fmt.Errorf("corpus is missing domain %q", domain)
		}
	}
	for _, scenario := range requiredScenarios {
		if !scenarios[scenario] {
			return fmt.Errorf("corpus is missing scenario %q", scenario)
		}
	}
	return nil
}

func ValidateModeDataset(mode, profile string, dataset *EvalDataset) error {
	if err := ValidateDataset(dataset); err != nil {
		return err
	}
	if profile == "recorded" {
		for _, evalCase := range dataset.Cases {
			if evalCase.Scenario == "recorded_blind_root_cause" &&
				(len(evalCase.ExpectedCauseKeywords) == 0 || len(evalCase.ExpectedEntityKeywords) == 0) {
				return fmt.Errorf("recorded case %q requires structured cause and entity labels", evalCase.ID)
			}
		}
	}
	switch mode {
	case "gate", "smoke", "regression-baseline":
		if dataset.Role != DatasetRoleRegression {
			return fmt.Errorf("%s mode requires regression dataset, got %s", mode, dataset.Role)
		}
	case "baseline", "compare":
		if profile == "real" && dataset.Role != DatasetRoleHoldout {
			return fmt.Errorf("%s real mode requires holdout dataset, got %s", mode, dataset.Role)
		}
		if profile == "recorded" && dataset.Role != DatasetRoleDevelopment && dataset.Role != DatasetRoleHoldout {
			return fmt.Errorf("%s recorded mode requires development or holdout dataset, got %s", mode, dataset.Role)
		}
		if profile != "real" && profile != "recorded" {
			return fmt.Errorf("%s mode requires real or recorded profile", mode)
		}
	case "gos", "gos-batch", "export-runs":
		if profile == "eval" && dataset.Role == DatasetRoleHoldout {
			return fmt.Errorf("eval profile cannot run holdout dataset")
		}
	default:
		return fmt.Errorf("unsupported eval mode %q", mode)
	}
	return nil
}

func validFailurePhase(phase string) bool {
	switch phase {
	case "ingest", "plan", "act", "update", "state", "report":
		return true
	default:
		return false
	}
}
