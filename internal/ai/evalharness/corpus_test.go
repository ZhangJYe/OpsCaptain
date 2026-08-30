package evalharness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareAndValidateAIOps2025Corpus(t *testing.T) {
	source := t.TempDir()
	inputs := []map[string]string{
		{"uuid": "case-a", "Anomaly Description": "A"},
		{"uuid": "case-b", "Anomaly Description": "B"},
	}
	input, _ := json.Marshal(inputs)
	if err := os.WriteFile(filepath.Join(source, "input.json"), input, 0o600); err != nil {
		t.Fatal(err)
	}
	truth := "{\"uuid\":\"case-a\",\"fault_type\":\"cpu_stress\",\"instance_type\":\"service\",\"service\":\"email\",\"instance\":\"email\",\"start_time\":\"2025-06-05T00:00:00Z\",\"key_observations\":[{\"type\":\"metric\",\"keyword\":[\"cpu\"]}],\"key_metrics\":[\"cpu\"],\"fault_description\":[\"cpu stress\"]}\n" +
		"{\"uuid\":\"case-b\",\"fault_type\":\"network_loss\",\"instance_type\":\"service\",\"service\":\"checkout\",\"instance\":[\"checkout\",\"shipping\"],\"start_time\":\"2025-06-06T00:00:00Z\",\"key_observations\":[{\"type\":\"trace\",\"keyword\":[\"timeout\"]}],\"key_metrics\":[\"timeout\"],\"fault_description\":[\"network loss\"]}\n"
	if err := os.WriteFile(filepath.Join(source, "groundtruth.jsonl"), []byte(truth), 0o600); err != nil {
		t.Fatal(err)
	}

	output := t.TempDir()
	manifest, err := PrepareAIOps2025Corpus(CorpusPrepareOptions{SourceDir: source, OutputDir: output, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Provenance.Version != "test" || len(manifest.Files) != 4 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	validated, err := ValidateExternalCorpus(filepath.Join(output, "corpus-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if validated.Provenance.SplitFingerprint != manifest.Provenance.SplitFingerprint {
		t.Fatal("split fingerprint changed")
	}
}

func TestValidateSplitEnvelopeFamiliesRejectsLeakage(t *testing.T) {
	development := []CaseEnvelope{{ID: "a", Tags: []string{"family-same"}}}
	holdout := []CaseEnvelope{{ID: "b", Tags: []string{"family-same"}}}
	if err := validateSplitEnvelopeFamilies(development, holdout); err == nil {
		t.Fatal("expected family leakage rejection")
	}
}

func TestManifestRejectsExternalCorpusWrongSplitDataset(t *testing.T) {
	source := t.TempDir()
	inputs := []map[string]string{
		{"uuid": "case-a", "Anomaly Description": "A"},
		{"uuid": "case-b", "Anomaly Description": "B"},
	}
	input, _ := json.Marshal(inputs)
	if err := os.WriteFile(filepath.Join(source, "input.json"), input, 0o600); err != nil {
		t.Fatal(err)
	}
	truth := "{\"uuid\":\"case-a\",\"fault_type\":\"cpu_stress\",\"instance_type\":\"service\",\"service\":\"email\",\"start_time\":\"2025-06-05T00:00:00Z\",\"key_observations\":[{\"type\":\"metric\",\"keyword\":[\"cpu\"]}]}\n" +
		"{\"uuid\":\"case-b\",\"fault_type\":\"network_loss\",\"instance_type\":\"service\",\"service\":\"checkout\",\"start_time\":\"2025-06-06T00:00:00Z\",\"key_observations\":[{\"type\":\"trace\",\"keyword\":[\"timeout\"]}]}\n"
	if err := os.WriteFile(filepath.Join(source, "groundtruth.jsonl"), []byte(truth), 0o600); err != nil {
		t.Fatal(err)
	}
	output := t.TempDir()
	if _, err := PrepareAIOps2025Corpus(CorpusPrepareOptions{SourceDir: source, OutputDir: output}); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(output, "manifests", "wrong-split.yaml")
	content := "schema_version: evaluation-harness/v1\nrun_name: wrong-split\ndataset_role: development\nlabel_source: fixture\nprofile: recorded\nexternal_corpus_manifest: ../corpus-manifest.json\ndependencies: {model: fixture}\ncode_scope: fixture\ncode_paths: [" + filepath.Join(output, "corpus-manifest.json") + "]\nmodel_fingerprint: fixture\nprompt_fingerprint: fixture\nevaluator_fingerprint: fixture\nevidence_corpus_sha256: fixture\nsuites:\n  - {name: gos, enabled: true, dataset: ../holdout/gos.jsonl, payload_schema: gos-eval/v1}\n"
	if err := os.WriteFile(manifestPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(manifestPath); err == nil {
		t.Fatal("expected wrong split dataset rejection")
	}
}
