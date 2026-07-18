package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	einoschema "github.com/cloudwego/eino/schema"
)

const (
	recordedTelemetryToolName      = "query_recorded_telemetry"
	recordedEvidenceMaxContentSize = 128 * 1024
)

var recordedCaseIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type recordedEvidenceSource struct {
	root          string
	caseID        string
	timeout       time.Duration
	toolMu        sync.Mutex
	toolDelivered bool
}

type recordedEvidenceInput struct {
	Query string `json:"query" jsonschema:"description=Question about the metrics logs and traces recorded for the current incident"`
}

type recordedEvidenceOutput struct {
	Success           bool   `json:"success"`
	Degraded          bool   `json:"degraded,omitempty"`
	Message           string `json:"message,omitempty"`
	Error             string `json:"error,omitempty"`
	CaseID            string `json:"case_id"`
	ProvenanceProfile string `json:"provenance_profile,omitempty"`
	ArtifactRef       string `json:"artifact_ref,omitempty"`
	Data              string `json:"data,omitempty"`
}

type recordedEvidenceMetadata struct {
	CaseID                string `json:"case_id"`
	ProvenanceProfile     string `json:"provenance_profile"`
	EvaluationEligibility string `json:"evaluation_eligibility"`
	TargetSelection       string `json:"target_selection"`
	Service               string `json:"service"`
	Instance              []any  `json:"instance"`
}

func newRecordedEvidenceSource(root, caseID string, timeout time.Duration) (*recordedEvidenceSource, error) {
	root = strings.TrimSpace(root)
	caseID = strings.TrimSpace(caseID)
	if root == "" {
		return nil, fmt.Errorf("recorded evidence root is required")
	}
	if !recordedCaseIDPattern.MatchString(caseID) || caseID == "." || caseID == ".." {
		return nil, fmt.Errorf("invalid recorded evidence case id %q", caseID)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve recorded evidence root: %w", err)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("recorded evidence timeout must be positive")
	}
	return &recordedEvidenceSource{root: absRoot, caseID: caseID, timeout: timeout}, nil
}

func (s *recordedEvidenceSource) Tool() (tool.InvokableTool, error) {
	return utils.InferOptionableTool(
		recordedTelemetryToolName,
		"Read the metrics, logs, and traces recorded for only the current evaluation incident. This read-only tool never queries live infrastructure or another evaluation case.",
		func(ctx context.Context, input *recordedEvidenceInput, opts ...tool.Option) (string, error) {
			content, err := s.Load(ctx)
			if err != nil {
				data, marshalErr := json.Marshal(recordedEvidenceOutput{
					Success:  false,
					Degraded: true,
					Message:  "当前 case 的只读录制遥测不可用；禁止回退到其他 case 或生产遥测。",
					Error:    err.Error(),
					CaseID:   s.caseID,
				})
				if marshalErr != nil {
					return "", marshalErr
				}
				return string(data), nil
			}
			fullSnapshot := s.claimToolSnapshot()
			message := ""
			if !fullSnapshot {
				content = "同一 case 的不可变遥测快照已在本轮先前的工具结果中完整返回；请复用已有证据，禁止重复拉取或回退到其他数据源。"
				message = "recorded snapshot already delivered for this case"
			}
			data, err := json.Marshal(recordedEvidenceOutput{
				Success:           true,
				Message:           message,
				CaseID:            s.caseID,
				ProvenanceProfile: "recorded_blind",
				ArtifactRef:       "recorded://" + s.caseID,
				Data:              content,
			})
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	)
}

func (s *recordedEvidenceSource) claimToolSnapshot() bool {
	s.toolMu.Lock()
	defer s.toolMu.Unlock()
	if s.toolDelivered {
		return false
	}
	s.toolDelivered = true
	return true
}

func (s *recordedEvidenceSource) RAGQuery(ctx context.Context, _ string) ([]*einoschema.Document, error) {
	content, err := s.Load(ctx)
	if err != nil {
		return nil, err
	}
	return []*einoschema.Document{{
		ID:      s.caseID,
		Content: content,
		MetaData: map[string]any{
			"case_id":                s.caseID,
			"provenance_profile":     "recorded_blind",
			"evaluation_eligibility": "development_only",
			"artifact_ref":           "recorded://" + s.caseID,
		},
	}}, nil
}

func (s *recordedEvidenceSource) Load(ctx context.Context) (string, error) {
	loadCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	docsDir := filepath.Join(s.root, "docs_evidence_telemetry")
	metadataPath := filepath.Join(docsDir, s.caseID+".metadata.json")
	documentPath := filepath.Join(docsDir, s.caseID+".md")
	if err := rejectSymlink(docsDir); err != nil {
		return "", err
	}
	if err := rejectSymlink(metadataPath); err != nil {
		return "", err
	}
	if err := rejectSymlink(documentPath); err != nil {
		return "", err
	}

	metadataData, err := readBoundedFile(loadCtx, metadataPath, recordedEvidenceMaxContentSize)
	if err != nil {
		return "", fmt.Errorf("read recorded evidence metadata: %w", err)
	}
	if err := validateRecordedMetadata(metadataData, s.caseID); err != nil {
		return "", err
	}
	documentData, err := readBoundedFile(loadCtx, documentPath, recordedEvidenceMaxContentSize)
	if err != nil {
		return "", fmt.Errorf("read recorded evidence document: %w", err)
	}
	document := string(documentData)
	if err := validateRecordedDocument(document, s.caseID); err != nil {
		return "", err
	}
	return document, nil
}

func validateRecordedMetadata(data []byte, caseID string) error {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("decode recorded evidence metadata: %w", err)
	}
	if field := findForbiddenRecordedField(raw); field != "" {
		return fmt.Errorf("recorded evidence metadata contains forbidden label field %q", field)
	}
	var metadata recordedEvidenceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return fmt.Errorf("decode recorded evidence metadata contract: %w", err)
	}
	if metadata.CaseID != caseID {
		return fmt.Errorf("recorded evidence case mismatch: expected %q, got %q", caseID, metadata.CaseID)
	}
	if metadata.ProvenanceProfile != "recorded_blind" || metadata.EvaluationEligibility != "development_only" || metadata.TargetSelection != "input_time_window_only" {
		return fmt.Errorf("recorded evidence provenance contract is invalid")
	}
	if metadata.Service != "unknown" || len(metadata.Instance) != 0 {
		return fmt.Errorf("recorded evidence metadata contains label-derived target scope")
	}
	return nil
}

func findForbiddenRecordedField(value any) string {
	forbidden := map[string]bool{
		"fault_type": true, "fault_category": true, "fault_description": true,
		"groundtruth": true, "ground_truth": true, "key_observations": true,
		"key_metrics": true, "target": true, "targets": true,
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			if forbidden[normalized] {
				return normalized
			}
			if found := findForbiddenRecordedField(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findForbiddenRecordedField(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func validateRecordedDocument(document, caseID string) error {
	required := []string{
		"- case_id: " + caseID,
		"- provenance_profile: recorded_blind",
		"- evaluation_eligibility: development_only",
		"- target_selection: input_time_window_only",
	}
	for _, marker := range required {
		if !strings.Contains(document, marker) {
			return fmt.Errorf("recorded evidence document is missing contract marker %q", marker)
		}
	}
	lower := strings.ToLower(document)
	for _, marker := range []string{"ground_truth:", "groundtruth:", "fault_type:", "fault_category:", "fault_description:", "key_observations:", "key_metrics:"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("recorded evidence document contains forbidden label marker %q", marker)
		}
	}
	return nil
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", filepath.Base(path), err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("recorded evidence symlink is not allowed: %s", filepath.Base(path))
	}
	return nil
}

func readBoundedFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d-byte budget", maxBytes)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}
