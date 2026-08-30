package evalharness

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("decode manifest yaml: %w", err)
		}
	default:
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("decode manifest json: %w", err)
		}
	}
	manifest.SourcePath = path
	for i := range manifest.Suites {
		if len(manifest.Suites[i].ConfigMap) > 0 {
			encoded, marshalErr := json.Marshal(manifest.Suites[i].ConfigMap)
			if marshalErr != nil {
				return nil, fmt.Errorf("encode suite %s config: %w", manifest.Suites[i].Name, marshalErr)
			}
			manifest.Suites[i].Config = encoded
		}
	}
	manifest.Budget.Normalize()
	for i := range manifest.Suites {
		manifest.Suites[i].Budget.Normalize()
	}
	if err := ValidateManifest(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func ValidateManifest(manifest *Manifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is required")
	}
	if manifest.SchemaVersion != ManifestSchemaVersion {
		return fmt.Errorf("unsupported manifest schema_version %q", manifest.SchemaVersion)
	}
	if strings.TrimSpace(manifest.RunName) == "" {
		return fmt.Errorf("run_name is required")
	}
	if !validRole(manifest.DatasetRole) {
		return fmt.Errorf("unsupported dataset_role %q", manifest.DatasetRole)
	}
	if strings.TrimSpace(manifest.LabelSource) == "" {
		return fmt.Errorf("label_source is required")
	}
	for name, value := range map[string]string{
		"code_scope": manifest.CodeScope, "model_fingerprint": manifest.ModelFingerprint,
		"prompt_fingerprint": manifest.PromptFingerprint, "evaluator_fingerprint": manifest.EvaluatorFingerprint,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if len(manifest.CodePaths) == 0 {
		return fmt.Errorf("code_paths is required")
	}
	if len(manifest.Dependencies) == 0 {
		return fmt.Errorf("dependencies are required")
	}
	if manifest.Profile == ProfileRecorded && len(manifest.EvidenceCorpusPaths) == 0 && strings.TrimSpace(manifest.EvidenceCorpusSHA256) == "" {
		return fmt.Errorf("recorded profile requires an evidence corpus fingerprint")
	}
	if !validProfile(manifest.Profile) {
		return fmt.Errorf("unsupported profile %q", manifest.Profile)
	}
	if !validRoleProfile(manifest.DatasetRole, manifest.Profile) {
		return fmt.Errorf("dataset_role %q is incompatible with profile %q", manifest.DatasetRole, manifest.Profile)
	}
	if strings.TrimSpace(manifest.ExternalCorpusManifest) != "" {
		if manifest.Profile != ProfileRecorded {
			return fmt.Errorf("external corpus manifest requires recorded profile")
		}
		if err := validateExternalCorpusReference(manifest); err != nil {
			return err
		}
	}
	if len(manifest.Suites) == 0 {
		return fmt.Errorf("at least one suite is required")
	}
	seen := make(map[SuiteName]struct{}, len(manifest.Suites))
	for index, suite := range manifest.Suites {
		if !validSuite(suite.Name) {
			return fmt.Errorf("suite[%d] has unsupported name %q", index, suite.Name)
		}
		if _, ok := seen[suite.Name]; ok {
			return fmt.Errorf("duplicate suite %q", suite.Name)
		}
		seen[suite.Name] = struct{}{}
		if !suite.Enabled {
			continue
		}
		if strings.TrimSpace(suite.Dataset) == "" {
			return fmt.Errorf("suite %q dataset is required", suite.Name)
		}
		if strings.TrimSpace(suite.PayloadSchema) == "" {
			return fmt.Errorf("suite %q payload_schema is required", suite.Name)
		}
		if err := validateGateSpecs(suite.Gates); err != nil {
			return fmt.Errorf("suite %q gates: %w", suite.Name, err)
		}
	}
	if err := validateGateSpecs(manifest.Gates); err != nil {
		return fmt.Errorf("manifest gates: %w", err)
	}
	return nil
}

func validateExternalCorpusReference(manifest *Manifest) error {
	path := resolveManifestPath(manifest.SourcePath, manifest.ExternalCorpusManifest)
	corpus, err := ValidateExternalCorpus(path)
	if err != nil {
		return fmt.Errorf("validate external corpus: %w", err)
	}
	found := make(map[SuiteName]bool)
	for _, file := range corpus.Files {
		if file.Role == manifest.DatasetRole {
			found[file.Suite] = true
		}
	}
	for _, suite := range manifest.Suites {
		if !suite.Enabled {
			continue
		}
		if !found[suite.Name] {
			return fmt.Errorf("external corpus has no %s dataset for %s", suite.Name, manifest.DatasetRole)
		}
		if !externalCorpusDatasetMatches(manifest.SourcePath, path, corpus, manifest.DatasetRole, suite) {
			return fmt.Errorf("suite %s dataset does not match declared external corpus split", suite.Name)
		}
	}
	return nil
}

func externalCorpusDatasetMatches(harnessManifestPath, corpusManifestPath string, corpus *ExternalCorpusManifest, role DatasetRole, suite SuiteConfig) bool {
	actual := resolveManifestPath(harnessManifestPath, suite.Dataset)
	for _, file := range corpus.Files {
		if file.Role != role || file.Suite != suite.Name {
			continue
		}
		expected := resolveManifestPath(corpusManifestPath, file.Path)
		if filepath.Clean(actual) == filepath.Clean(expected) {
			return true
		}
	}
	return false
}

func LoadCases(manifestPath string, suite SuiteConfig) ([]CaseEnvelope, string, error) {
	path := resolveManifestPath(manifestPath, suite.Dataset)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read suite %s dataset: %w", suite.Name, err)
	}
	digest := sha256.Sum256(data)
	actualHash := hex.EncodeToString(digest[:])
	if suite.DatasetSHA256 != "" && !strings.EqualFold(suite.DatasetSHA256, actualHash) {
		return nil, "", fmt.Errorf("suite %s dataset fingerprint mismatch", suite.Name)
	}

	cases, err := decodeCases(data, filepath.Ext(path))
	if err != nil {
		return nil, "", fmt.Errorf("decode suite %s dataset: %w", suite.Name, err)
	}
	if len(cases) == 0 {
		return nil, "", fmt.Errorf("suite %s dataset is empty", suite.Name)
	}
	ids := make(map[string]struct{}, len(cases))
	for index := range cases {
		if err := ValidateCase(cases[index], suite); err != nil {
			return nil, "", fmt.Errorf("suite %s case[%d]: %w", suite.Name, index, err)
		}
		if _, ok := ids[cases[index].ID]; ok {
			return nil, "", fmt.Errorf("suite %s contains duplicate case id %q", suite.Name, cases[index].ID)
		}
		ids[cases[index].ID] = struct{}{}
	}
	return cases, actualHash, nil
}

func ValidateCase(evalCase CaseEnvelope, suite SuiteConfig) error {
	if evalCase.SchemaVersion != CaseSchemaVersion {
		return fmt.Errorf("unsupported schema_version %q", evalCase.SchemaVersion)
	}
	if strings.TrimSpace(evalCase.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if evalCase.Suite != suite.Name {
		return fmt.Errorf("case suite %q does not match manifest suite %q", evalCase.Suite, suite.Name)
	}
	if strings.TrimSpace(evalCase.Input.Query) == "" && evalCase.Suite != SuiteTool && evalCase.Suite != SuiteEvidence {
		return fmt.Errorf("input.query is required")
	}
	if evalCase.PayloadSchemaVersion != suite.PayloadSchema {
		return fmt.Errorf("payload schema %q does not match %q", evalCase.PayloadSchemaVersion, suite.PayloadSchema)
	}
	if len(bytes.TrimSpace(evalCase.Payload)) == 0 || !json.Valid(evalCase.Payload) {
		return fmt.Errorf("payload must be valid json")
	}
	if len(bytes.TrimSpace(evalCase.Expectation)) == 0 || !json.Valid(evalCase.Expectation) {
		return fmt.Errorf("expectation must be valid json")
	}
	return nil
}

func decodeCases(data []byte, ext string) ([]CaseEnvelope, error) {
	if strings.EqualFold(ext, ".jsonl") {
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		var cases []CaseEnvelope
		for scanner.Scan() {
			line := bytes.TrimSpace(scanner.Bytes())
			if len(line) == 0 {
				continue
			}
			var evalCase CaseEnvelope
			if err := json.Unmarshal(line, &evalCase); err != nil {
				return nil, err
			}
			cases = append(cases, evalCase)
		}
		return cases, scanner.Err()
	}
	var cases []CaseEnvelope
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func resolveManifestPath(manifestPath, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(filepath.Dir(manifestPath), target))
}

func validRole(role DatasetRole) bool {
	return role == DatasetDevelopment || role == DatasetRegression || role == DatasetHoldout
}

func validProfile(profile Profile) bool {
	return profile == ProfileDeterministic || profile == ProfileRecorded || profile == ProfileLive
}

func validRoleProfile(role DatasetRole, profile Profile) bool {
	switch role {
	case DatasetDevelopment:
		return true
	case DatasetRegression:
		return profile == ProfileDeterministic
	case DatasetHoldout:
		return profile == ProfileRecorded || profile == ProfileLive
	default:
		return false
	}
}

func validSuite(suite SuiteName) bool {
	switch suite {
	case SuiteRoute, SuiteRAG, SuitePlan, SuiteGoS, SuiteTool, SuiteEvidence:
		return true
	default:
		return false
	}
}

func validateGateSpecs(gates []GateSpec) error {
	for _, gate := range gates {
		if strings.TrimSpace(gate.Name) == "" || strings.TrimSpace(gate.Metric) == "" {
			return fmt.Errorf("gate name and metric are required")
		}
		if gate.Severity != GateBlocking && gate.Severity != GateWarning {
			return fmt.Errorf("gate %q has invalid severity %q", gate.Name, gate.Severity)
		}
		switch gate.Operator {
		case ">=", ">", "<=", "<", "==":
		default:
			return fmt.Errorf("gate %q has invalid operator %q", gate.Name, gate.Operator)
		}
	}
	return nil
}
