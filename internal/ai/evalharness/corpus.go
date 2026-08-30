package evalharness

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	goseval "SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/internal/ai/protocol"
)

const ExternalCorpusSchemaVersion = "recorded-evaluation-corpus/v1"

type CorpusPrepareOptions struct {
	SourceDir   string
	OutputDir   string
	Version     string
	ProjectRoot string
}

type CorpusProvenance struct {
	SchemaVersion     string    `json:"schema_version" yaml:"schema_version"`
	CorpusName        string    `json:"corpus_name" yaml:"corpus_name"`
	Version           string    `json:"version" yaml:"version"`
	Source            string    `json:"source" yaml:"source"`
	License           string    `json:"license" yaml:"license"`
	InputSHA256       string    `json:"input_sha256" yaml:"input_sha256"`
	GroundTruthSHA256 string    `json:"groundtruth_sha256" yaml:"groundtruth_sha256"`
	SplitStrategy     string    `json:"split_strategy" yaml:"split_strategy"`
	SplitFingerprint  string    `json:"split_fingerprint" yaml:"split_fingerprint"`
	CreatedAt         time.Time `json:"created_at" yaml:"created_at"`
}

type CorpusFile struct {
	Role   DatasetRole `json:"role"`
	Suite  SuiteName   `json:"suite"`
	Path   string      `json:"path"`
	SHA256 string      `json:"sha256"`
	Cases  int         `json:"cases"`
}

type CorpusCoverage struct {
	Role       DatasetRole    `json:"role"`
	Cases      int            `json:"cases"`
	Target     int            `json:"target"`
	Gap        int            `json:"gap"`
	FaultTypes map[string]int `json:"fault_types"`
	Instances  map[string]int `json:"instance_types"`
	Services   map[string]int `json:"services"`
	Modalities map[string]int `json:"modalities"`
}

type ExternalCorpusManifest struct {
	Provenance CorpusProvenance `json:"provenance"`
	Files      []CorpusFile     `json:"files"`
	Coverage   []CorpusCoverage `json:"coverage"`
}

type corpusInput struct {
	UUID        string `json:"uuid"`
	Description string `json:"Anomaly Description"`
}

type corpusGroundTruth struct {
	UUID             string              `json:"uuid"`
	FaultType        string              `json:"fault_type"`
	InstanceType     string              `json:"instance_type"`
	Service          string              `json:"service"`
	Instance         json.RawMessage     `json:"instance"`
	Source           string              `json:"source"`
	Destination      string              `json:"destination"`
	StartTime        string              `json:"start_time"`
	KeyObservations  []corpusObservation `json:"key_observations"`
	KeyMetrics       []string            `json:"key_metrics"`
	FaultDescription []string            `json:"fault_description"`
}

type corpusObservation struct {
	Type    string   `json:"type"`
	Keyword []string `json:"keyword"`
}

type corpusCase struct {
	Input  corpusInput
	Truth  corpusGroundTruth
	Family string
}

func PrepareAIOps2025Corpus(options CorpusPrepareOptions) (*ExternalCorpusManifest, error) {
	if strings.TrimSpace(options.SourceDir) == "" || strings.TrimSpace(options.OutputDir) == "" {
		return nil, fmt.Errorf("source and output directories are required")
	}
	if strings.TrimSpace(options.Version) == "" {
		options.Version = "aiops2025-v1"
	}
	inputs, inputPath, err := loadCorpusInputs(options.SourceDir)
	if err != nil {
		return nil, err
	}
	truths, truthPath, err := loadCorpusGroundTruth(options.SourceDir)
	if err != nil {
		return nil, err
	}
	cases, err := joinCorpusCases(inputs, truths)
	if err != nil {
		return nil, err
	}
	inputSHA, err := FileSHA256(inputPath)
	if err != nil {
		return nil, fmt.Errorf("fingerprint input: %w", err)
	}
	truthSHA, err := FileSHA256(truthPath)
	if err != nil {
		return nil, fmt.Errorf("fingerprint ground truth: %w", err)
	}
	development, holdout := groupSplit(cases)
	if err := validateSplit(development, holdout); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(options.OutputDir, 0o750); err != nil {
		return nil, fmt.Errorf("create corpus output: %w", err)
	}
	files, err := writeCorpusCases(options.OutputDir, development, holdout)
	if err != nil {
		return nil, err
	}
	manifest := &ExternalCorpusManifest{
		Provenance: CorpusProvenance{
			SchemaVersion: ExternalCorpusSchemaVersion, CorpusName: "aiops2025", Version: options.Version,
			Source: filepath.Clean(options.SourceDir), License: "CC BY-NC 4.0", InputSHA256: inputSHA,
			GroundTruthSHA256: truthSHA, SplitStrategy: "sha256(fault_type|instance_type|target|observation_date), development when first byte < 192",
			SplitFingerprint: splitFingerprint(development, holdout), CreatedAt: time.Now().UTC(),
		},
		Files:    files,
		Coverage: []CorpusCoverage{coverage(DatasetDevelopment, development, 3000), coverage(DatasetHoldout, holdout, 700)},
	}
	if err := WriteExternalCorpusManifest(filepath.Join(options.OutputDir, "corpus-manifest.json"), manifest); err != nil {
		return nil, err
	}
	if err := writeGeneratedManifests(options.OutputDir, options.ProjectRoot, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func LoadExternalCorpusManifest(path string) (*ExternalCorpusManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus manifest: %w", err)
	}
	var manifest ExternalCorpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode corpus manifest: %w", err)
	}
	if err := ValidateExternalCorpusManifest(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func WriteExternalCorpusManifest(path string, manifest *ExternalCorpusManifest) error {
	if err := ValidateExternalCorpusManifest(manifest); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("encode corpus manifest: %w", err)
	}
	return atomicWrite(path, append(data, '\n'))
}

func ValidateExternalCorpusManifest(manifest *ExternalCorpusManifest) error {
	if manifest == nil || manifest.Provenance.SchemaVersion != ExternalCorpusSchemaVersion {
		return fmt.Errorf("unsupported external corpus schema")
	}
	for name, value := range map[string]string{"version": manifest.Provenance.Version, "source": manifest.Provenance.Source, "input_sha256": manifest.Provenance.InputSHA256, "groundtruth_sha256": manifest.Provenance.GroundTruthSHA256, "split_fingerprint": manifest.Provenance.SplitFingerprint} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("corpus provenance %s is required", name)
		}
	}
	roles := map[DatasetRole]bool{}
	for _, file := range manifest.Files {
		if file.Role != DatasetDevelopment && file.Role != DatasetHoldout || file.Suite != SuiteGoS && file.Suite != SuiteEvidence || file.Cases <= 0 || strings.TrimSpace(file.Path) == "" || strings.TrimSpace(file.SHA256) == "" {
			return fmt.Errorf("invalid corpus file declaration")
		}
		roles[file.Role] = true
	}
	if !roles[DatasetDevelopment] || !roles[DatasetHoldout] {
		return fmt.Errorf("corpus must declare development and holdout files")
	}
	return nil
}

func ValidateExternalCorpus(path string) (*ExternalCorpusManifest, error) {
	manifest, err := LoadExternalCorpusManifest(path)
	if err != nil {
		return nil, err
	}
	base := filepath.Dir(path)
	development, holdout := make([]CaseEnvelope, 0), make([]CaseEnvelope, 0)
	for _, file := range manifest.Files {
		resolved := file.Path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(base, resolved)
		}
		actual, err := FileSHA256(resolved)
		if err != nil || !strings.EqualFold(actual, file.SHA256) {
			return nil, fmt.Errorf("corpus file fingerprint mismatch: %s", file.Path)
		}
		cases, _, err := LoadCases(path, SuiteConfig{Name: file.Suite, Dataset: resolved, PayloadSchema: payloadSchema(file.Suite)})
		if err != nil {
			return nil, err
		}
		if len(cases) != file.Cases {
			return nil, fmt.Errorf("corpus file case count mismatch: %s", file.Path)
		}
		if file.Role == DatasetDevelopment {
			development = append(development, cases...)
		} else {
			holdout = append(holdout, cases...)
		}
	}
	if err := validateSplitEnvelopeFamilies(development, holdout); err != nil {
		return nil, err
	}
	return manifest, nil
}

func loadCorpusInputs(source string) (map[string]corpusInput, string, error) {
	path := filepath.Join(source, "input.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, path, fmt.Errorf("read corpus input: %w", err)
	}
	var raw []corpusInput
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, path, fmt.Errorf("decode corpus input: %w", err)
	}
	items := make(map[string]corpusInput, len(raw))
	for _, item := range raw {
		if strings.TrimSpace(item.UUID) == "" || strings.TrimSpace(item.Description) == "" {
			return nil, path, fmt.Errorf("invalid corpus input")
		}
		if _, exists := items[item.UUID]; exists {
			return nil, path, fmt.Errorf("duplicate input uuid %q", item.UUID)
		}
		items[item.UUID] = item
	}
	return items, path, nil
}

func loadCorpusGroundTruth(source string) (map[string]corpusGroundTruth, string, error) {
	path := filepath.Join(source, "groundtruth.jsonl")
	file, err := os.Open(path)
	if err != nil {
		return nil, path, fmt.Errorf("open corpus ground truth: %w", err)
	}
	defer file.Close()
	items := make(map[string]corpusGroundTruth)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var item corpusGroundTruth
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return nil, path, fmt.Errorf("decode ground truth: %w", err)
		}
		if strings.TrimSpace(item.UUID) == "" || strings.TrimSpace(item.FaultType) == "" || strings.TrimSpace(item.StartTime) == "" {
			return nil, path, fmt.Errorf("invalid ground truth")
		}
		if _, exists := items[item.UUID]; exists {
			return nil, path, fmt.Errorf("duplicate ground truth uuid %q", item.UUID)
		}
		items[item.UUID] = item
	}
	if err := scanner.Err(); err != nil {
		return nil, path, err
	}
	return items, path, nil
}

func joinCorpusCases(inputs map[string]corpusInput, truths map[string]corpusGroundTruth) ([]corpusCase, error) {
	if len(inputs) == 0 || len(inputs) != len(truths) {
		return nil, fmt.Errorf("input and ground truth counts differ: %d vs %d", len(inputs), len(truths))
	}
	cases := make([]corpusCase, 0, len(inputs))
	for id, input := range inputs {
		truth, ok := truths[id]
		if !ok {
			return nil, fmt.Errorf("missing ground truth for %q", id)
		}
		cases = append(cases, corpusCase{Input: input, Truth: truth, Family: familyKey(truth)})
	}
	sort.Slice(cases, func(i, j int) bool { return cases[i].Input.UUID < cases[j].Input.UUID })
	return cases, nil
}

func familyKey(truth corpusGroundTruth) string {
	target := canonicalTarget(truth)
	day := strings.Split(strings.TrimSpace(truth.StartTime), "T")[0]
	return strings.Join([]string{strings.ToLower(strings.TrimSpace(truth.FaultType)), strings.ToLower(strings.TrimSpace(truth.InstanceType)), target, day}, "|")
}

func canonicalTarget(truth corpusGroundTruth) string {
	parts := []string{truth.Service, truth.Source, truth.Destination}
	var instance string
	if json.Unmarshal(truth.Instance, &instance) == nil {
		parts = append(parts, instance)
	} else {
		var instances []string
		if json.Unmarshal(truth.Instance, &instances) == nil {
			parts = append(parts, instances...)
		}
	}
	filtered := make([]string, 0, len(parts))
	for _, value := range parts {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			filtered = append(filtered, value)
		}
	}
	sort.Strings(filtered)
	return strings.Join(filtered, ",")
}

func groupSplit(cases []corpusCase) ([]corpusCase, []corpusCase) {
	development, holdout := make([]corpusCase, 0, len(cases)), make([]corpusCase, 0, len(cases)/4)
	for _, item := range cases {
		digest := sha256.Sum256([]byte(item.Family))
		if digest[0] < 192 {
			development = append(development, item)
		} else {
			holdout = append(holdout, item)
		}
	}
	if len(cases) > 1 && (len(development) == 0 || len(holdout) == 0) {
		// Small corpora can hash into one bucket. Move one whole, deterministically
		// selected family so the split remains usable without splitting a family.
		from, to := &development, &holdout
		if len(development) == 0 {
			from, to = &holdout, &development
		}
		families := make([]string, 0)
		seen := make(map[string]bool)
		for _, item := range *from {
			if !seen[item.Family] {
				seen[item.Family] = true
				families = append(families, item.Family)
			}
		}
		sort.Strings(families)
		selected := families[len(families)-1]
		remaining := (*from)[:0]
		for _, item := range *from {
			if item.Family == selected {
				*to = append(*to, item)
			} else {
				remaining = append(remaining, item)
			}
		}
		*from = remaining
	}
	return development, holdout
}

func validateSplit(development, holdout []corpusCase) error {
	families := map[string]string{}
	for _, pair := range []struct {
		role  string
		cases []corpusCase
	}{{"development", development}, {"holdout", holdout}} {
		for _, item := range pair.cases {
			if previous, exists := families[item.Family]; exists && previous != pair.role {
				return fmt.Errorf("fault family leaks across splits: %s", item.Family)
			}
			families[item.Family] = pair.role
		}
	}
	if len(development) == 0 || len(holdout) == 0 {
		return fmt.Errorf("group split produced an empty role")
	}
	return nil
}

func writeCorpusCases(output string, development, holdout []corpusCase) ([]CorpusFile, error) {
	files := make([]CorpusFile, 0, 4)
	for _, split := range []struct {
		role  DatasetRole
		cases []corpusCase
	}{{DatasetDevelopment, development}, {DatasetHoldout, holdout}} {
		for _, suite := range []SuiteName{SuiteGoS, SuiteEvidence} {
			path := filepath.Join(output, string(split.role), string(suite)+".jsonl")
			if err := writeCaseFile(path, split.cases, suite); err != nil {
				return nil, err
			}
			hash, err := FileSHA256(path)
			if err != nil {
				return nil, err
			}
			files = append(files, CorpusFile{Role: split.role, Suite: suite, Path: filepath.Join(string(split.role), string(suite)+".jsonl"), SHA256: hash, Cases: len(split.cases)})
		}
	}
	return files, nil
}

func writeCaseFile(path string, cases []corpusCase, suite SuiteName) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	for _, item := range cases {
		encoded, err := json.Marshal(buildEnvelope(item, suite))
		if err != nil {
			file.Close()
			return err
		}
		if _, err := writer.Write(append(encoded, '\n')); err != nil {
			file.Close()
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func buildEnvelope(item corpusCase, suite SuiteName) CaseEnvelope {
	if suite == SuiteEvidence {
		return evidenceEnvelope(item)
	}
	return gosEnvelope(item)
}

func gosEnvelope(item corpusCase) CaseEnvelope {
	truth := item.Truth
	evidence := evidenceItems(truth)
	groundTruth := strings.Join(append([]string{truth.FaultType}, truth.FaultDescription...), " ")
	payload := map[string]any{
		"case":        goseval.EvalCase{ID: item.Input.UUID, Domain: truth.InstanceType, Scenario: "aiops2025_recorded", Symptom: item.Input.Description, GroundTruth: groundTruth, ExpectedKeywords: faultKeywords(truth), ExpectedEvidenceKeywords: observationKeywords(truth), ExpectedStatus: string(protocol.ResultStatusSucceeded)},
		"task_result": protocol.TaskResult{TaskID: item.Input.UUID, Agent: "recorded-aiops2025", Status: protocol.ResultStatusSucceeded, Summary: groundTruth, Confidence: 1, Evidence: evidence, Metadata: map[string]any{"belief_graph": map[string]any{}, "fsm_history": []any{}, "graph_valid": true, "recorded_fixture": true}},
	}
	return envelope(item, SuiteGoS, GoSPayloadSchema, payload)
}

func evidenceEnvelope(item corpusCase) CaseEnvelope {
	claims := make([]EvidenceClaim, 0, len(item.Truth.KeyObservations))
	citations := make([]EvidenceCitation, 0, len(item.Truth.KeyObservations))
	links := make([]EvidenceLink, 0, len(item.Truth.KeyObservations))
	for index, observation := range item.Truth.KeyObservations {
		id := fmt.Sprintf("%s-%s-%d", item.Input.UUID, observation.Type, index)
		text := strings.Join(observation.Keyword, " ")
		claims = append(claims, EvidenceClaim{Text: item.Truth.FaultType + " " + text, CitationIDs: []string{id}, RequiresEvidence: true})
		citations = append(citations, EvidenceCitation{ID: id, Source: "aiops2025/" + observation.Type, TraceID: item.Input.UUID, Text: text})
		links = append(links, EvidenceLink{CitationID: id, Text: text})
	}
	payload := EvidencePayload{Claims: claims, Citations: citations, Evidence: links, ExpectedKeywords: observationKeywords(item.Truth)}
	return envelope(item, SuiteEvidence, EvidencePayloadSchema, payload)
}

func envelope(item corpusCase, suite SuiteName, schema string, payload any) CaseEnvelope {
	encodedPayload, _ := json.Marshal(payload)
	expectation, _ := json.Marshal(map[string]any{"recorded": true, "fault_type": item.Truth.FaultType, "family": item.Family})
	return CaseEnvelope{SchemaVersion: CaseSchemaVersion, ID: item.Input.UUID, Suite: suite, Input: CaseInput{Query: item.Input.Description}, Expectation: expectation, Tags: []string{"aiops2025", strings.ToLower(item.Truth.FaultType), "recorded", suiteFamilyTag(item.Family)}, PayloadSchemaVersion: schema, Payload: encodedPayload}
}

func evidenceItems(truth corpusGroundTruth) []protocol.EvidenceItem {
	items := make([]protocol.EvidenceItem, 0, len(truth.KeyObservations))
	for index, observation := range truth.KeyObservations {
		items = append(items, protocol.EvidenceItem{SourceType: observation.Type, SourceID: fmt.Sprintf("%s-%d", truth.FaultType, index), Title: observation.Type + " observation", Snippet: strings.Join(observation.Keyword, " ")})
	}
	if len(items) == 0 {
		items = append(items, protocol.EvidenceItem{SourceType: "recorded_label", SourceID: truth.FaultType + "-no-observation", Title: "recorded label", Snippet: "no key observation label"})
	}
	return items
}

func observationKeywords(truth corpusGroundTruth) []string {
	seen, values := map[string]bool{}, make([]string, 0)
	for _, observation := range truth.KeyObservations {
		for _, value := range observation.Keyword {
			value = strings.TrimSpace(value)
			if value != "" && !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	}
	return values
}

func faultKeywords(truth corpusGroundTruth) []string {
	return []string{truth.FaultType}
}

func suiteFamilyTag(family string) string {
	digest := sha256.Sum256([]byte(family))
	return "family-" + hex.EncodeToString(digest[:8])
}

func splitFingerprint(development, holdout []corpusCase) string {
	values := make([]string, 0, len(development)+len(holdout))
	for _, item := range development {
		values = append(values, "development:"+item.Input.UUID+":"+item.Family)
	}
	for _, item := range holdout {
		values = append(values, "holdout:"+item.Input.UUID+":"+item.Family)
	}
	sort.Strings(values)
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:])
}

func coverage(role DatasetRole, cases []corpusCase, target int) CorpusCoverage {
	result := CorpusCoverage{Role: role, Cases: len(cases), Target: target, FaultTypes: map[string]int{}, Instances: map[string]int{}, Services: map[string]int{}, Modalities: map[string]int{}}
	if target > len(cases) {
		result.Gap = target - len(cases)
	}
	for _, item := range cases {
		result.FaultTypes[item.Truth.FaultType]++
		result.Instances[item.Truth.InstanceType]++
		result.Services[item.Truth.Service]++
		for _, observation := range item.Truth.KeyObservations {
			result.Modalities[observation.Type]++
		}
	}
	return result
}

func payloadSchema(suite SuiteName) string {
	if suite == SuiteGoS {
		return GoSPayloadSchema
	}
	return EvidencePayloadSchema
}

func validateSplitEnvelopeFamilies(development, holdout []CaseEnvelope) error {
	seen := map[string]string{}
	for _, pair := range []struct {
		role  string
		cases []CaseEnvelope
	}{{"development", development}, {"holdout", holdout}} {
		for _, item := range pair.cases {
			family := ""
			for _, tag := range item.Tags {
				if strings.HasPrefix(tag, "family-") {
					family = tag
					break
				}
			}
			if family == "" {
				return fmt.Errorf("case %q is missing family tag", item.ID)
			}
			if previous, exists := seen[family]; exists && previous != pair.role {
				return fmt.Errorf("fault family leaks across splits: %s", family)
			}
			seen[family] = pair.role
		}
	}
	return nil
}

func writeGeneratedManifests(output, projectRoot string, corpus *ExternalCorpusManifest) error {
	if strings.TrimSpace(projectRoot) == "" {
		var err error
		projectRoot, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve project root: %w", err)
		}
	}
	codePath := filepath.Join(projectRoot, "internal", "ai", "evalharness", "harness.go")
	for _, role := range []DatasetRole{DatasetDevelopment, DatasetHoldout} {
		path := filepath.Join(output, "manifests", "recorded-"+string(role)+".yaml")
		content := fmt.Sprintf("schema_version: evaluation-harness/v1\nrun_name: aiops2025-recorded-%s\ndataset_role: %s\nlabel_source: AIOps2025 groundtruth; offline recorded fixture\nprofile: recorded\nexternal_corpus_manifest: %s\ndependencies: {model: not_used, evidence: recorded_aiops2025, live_services: not_used}\ncontinue_on_error: true\ncode_scope: evaluation-harness-v1\ncode_paths: [%s]\nmodel_fingerprint: not-used-recorded-fixture\nprompt_fingerprint: not-used-recorded-fixture\nevaluator_fingerprint: evaluation-harness-v1\nevidence_corpus_sha256: %s\nbudget: {max_cases: 800, concurrency: 1, case_timeout_ms: 5000, total_timeout_ms: 600000}\nredaction: {max_text_chars: 512, sensitive_keys: [authorization, token, api_key, password, secret, dsn]}\nsuites:\n  - {name: gos, enabled: true, dataset: ../%s/gos.jsonl, payload_schema: gos-eval/v1, dataset_sha256: %s}\n  - {name: evidence, enabled: true, dataset: ../%s/evidence.jsonl, payload_schema: evidence-eval/v1, dataset_sha256: %s}\n", role, role, filepath.Join("..", "corpus-manifest.json"), codePath, corpus.Provenance.SplitFingerprint, role, corpusFileSHA(corpus, role, SuiteGoS), role, corpusFileSHA(corpus, role, SuiteEvidence))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func corpusFileSHA(corpus *ExternalCorpusManifest, role DatasetRole, suite SuiteName) string {
	for _, file := range corpus.Files {
		if file.Role == role && file.Suite == suite {
			return file.SHA256
		}
	}
	return ""
}
