package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeRecordedEvidenceCase(t *testing.T, root, caseID, marker, provenance string) {
	t.Helper()
	docsDir := filepath.Join(root, "docs_evidence_telemetry")
	require.NoError(t, os.MkdirAll(docsDir, 0o700))
	metadata := map[string]any{
		"case_id": caseID, "provenance_profile": provenance,
		"evaluation_eligibility": "development_only", "target_selection": "input_time_window_only",
		"service": "unknown", "instance": []any{},
	}
	metadataData, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(docsDir, caseID+".metadata.json"), metadataData, 0o600))
	document := "# Telemetry Evidence Case\n\n" +
		"- case_id: " + caseID + "\n" +
		"- provenance_profile: " + provenance + "\n" +
		"- evaluation_eligibility: development_only\n" +
		"- target_selection: input_time_window_only\n\n" +
		"## Metric Signals\n\n- " + marker + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(docsDir, caseID+".md"), []byte(document), 0o600))
}

func TestRecordedEvidenceSourceIsolatesCases(t *testing.T) {
	root := t.TempDir()
	writeRecordedEvidenceCase(t, root, "case-a", "only-a", "recorded_blind")
	writeRecordedEvidenceCase(t, root, "case-b", "only-b", "recorded_blind")
	source, err := newRecordedEvidenceSource(root, "case-a", time.Second)
	require.NoError(t, err)

	content, err := source.Load(context.Background())

	require.NoError(t, err)
	require.Contains(t, content, "only-a")
	require.NotContains(t, content, "only-b")
	docs, err := source.RAGQuery(context.Background(), "show telemetry")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, "case-a", docs[0].MetaData["case_id"])
}

func TestRecordedEvidenceSourceRejectsTraversalAndLabelAssistedArtifacts(t *testing.T) {
	_, err := newRecordedEvidenceSource(t.TempDir(), "../case-b", time.Second)
	require.ErrorContains(t, err, "invalid recorded evidence case id")

	root := t.TempDir()
	writeRecordedEvidenceCase(t, root, "case-a", "data", "recorded_label_assisted")
	source, err := newRecordedEvidenceSource(root, "case-a", time.Second)
	require.NoError(t, err)
	_, err = source.Load(context.Background())
	require.ErrorContains(t, err, "provenance contract")
}

func TestRecordedEvidenceToolDegradesWithoutFallingBack(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "docs_evidence_telemetry"), 0o700))
	source, err := newRecordedEvidenceSource(root, "missing-case", time.Second)
	require.NoError(t, err)
	toolInstance, err := source.Tool()
	require.NoError(t, err)

	info, err := toolInstance.Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, recordedTelemetryToolName, info.Name)
	require.NotNil(t, info.ParamsOneOf)
	output, err := toolInstance.InvokableRun(context.Background(), `{"query":"show evidence"}`)
	require.NoError(t, err)
	var decoded recordedEvidenceOutput
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	require.False(t, decoded.Success)
	require.True(t, decoded.Degraded)
	require.Equal(t, "missing-case", decoded.CaseID)
	require.NotContains(t, output, "case-a")
	_, err = source.RAGQuery(context.Background(), "show evidence")
	require.Error(t, err)
}

func TestRecordedEvidenceSourceRejectsLabelFields(t *testing.T) {
	root := t.TempDir()
	writeRecordedEvidenceCase(t, root, "case-a", "data", "recorded_blind")
	metadataPath := filepath.Join(root, "docs_evidence_telemetry", "case-a.metadata.json")
	metadata := map[string]any{
		"case_id": "case-a", "provenance_profile": "recorded_blind",
		"evaluation_eligibility": "development_only", "target_selection": "input_time_window_only",
		"service": "unknown", "instance": []any{}, "fault_type": "code error",
	}
	data, err := json.Marshal(metadata)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(metadataPath, data, 0o600))
	source, err := newRecordedEvidenceSource(root, "case-a", time.Second)
	require.NoError(t, err)

	_, err = source.Load(context.Background())

	require.ErrorContains(t, err, "forbidden label field")
}

func TestRecordedEvidenceToolReturnsTheFullSnapshotOnlyOnce(t *testing.T) {
	root := t.TempDir()
	writeRecordedEvidenceCase(t, root, "case-a", "unique-full-snapshot", "recorded_blind")
	source, err := newRecordedEvidenceSource(root, "case-a", time.Second)
	require.NoError(t, err)
	toolInstance, err := source.Tool()
	require.NoError(t, err)

	first, err := toolInstance.InvokableRun(context.Background(), `{"query":"metrics"}`)
	require.NoError(t, err)
	second, err := toolInstance.InvokableRun(context.Background(), `{"query":"logs"}`)
	require.NoError(t, err)

	require.Contains(t, first, "unique-full-snapshot")
	require.NotContains(t, second, "unique-full-snapshot")
	require.Contains(t, second, "snapshot already delivered")
}
