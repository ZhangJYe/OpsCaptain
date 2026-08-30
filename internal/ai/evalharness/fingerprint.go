package evalharness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
)

func FileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func FilesSHA256(paths ...string) (string, error) {
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read fingerprint file %s: %w", path, err)
		}
		hash.Write(data)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func CompareFingerprints(baseline, candidate Fingerprints) error {
	checks := []struct{ name, baseline, candidate string }{
		{"dataset", baseline.Dataset, candidate.Dataset},
		{"code_scope", baseline.CodeScope, candidate.CodeScope},
		{"evaluator", baseline.Evaluator, candidate.Evaluator},
		{"evidence_corpus", baseline.EvidenceCorpus, candidate.EvidenceCorpus},
	}
	for _, check := range checks {
		if check.baseline == "" || check.candidate == "" {
			return fmt.Errorf("%s fingerprint is required", check.name)
		}
		if check.baseline != check.candidate {
			return fmt.Errorf("%s fingerprint is incompatible", check.name)
		}
	}
	return nil
}

func ManifestFingerprints(manifest *Manifest, suites []SuiteReport) (Fingerprints, error) {
	if manifest == nil {
		return Fingerprints{}, fmt.Errorf("manifest is required")
	}
	fingerprints := Fingerprints{
		CodeScope: manifest.CodeScope, Model: manifest.ModelFingerprint, Prompt: manifest.PromptFingerprint,
		Evaluator: manifest.EvaluatorFingerprint, EvidenceCorpus: manifest.EvidenceCorpusSHA256,
	}
	if manifest.SourcePath != "" {
		hash, err := FileSHA256(manifest.SourcePath)
		if err != nil {
			return Fingerprints{}, fmt.Errorf("fingerprint manifest: %w", err)
		}
		fingerprints.Config = hash
	}
	datasetHashes := make([]string, 0, len(suites))
	for _, suite := range suites {
		if suite.Fingerprints.Dataset != "" {
			datasetHashes = append(datasetHashes, suite.Fingerprints.Dataset)
		}
	}
	if len(datasetHashes) > 0 {
		sort.Strings(datasetHashes)
		hash := sha256.New()
		for _, value := range datasetHashes {
			hash.Write([]byte(value))
			hash.Write([]byte{0})
		}
		fingerprints.Dataset = hex.EncodeToString(hash.Sum(nil))
	}
	if len(manifest.CodePaths) > 0 {
		paths := make([]string, 0, len(manifest.CodePaths))
		for _, path := range manifest.CodePaths {
			paths = append(paths, resolveManifestPath(manifest.SourcePath, path))
		}
		hash, err := FilesSHA256(paths...)
		if err != nil {
			return Fingerprints{}, err
		}
		fingerprints.Code = hash
	}
	if len(manifest.EvidenceCorpusPaths) > 0 {
		paths := make([]string, 0, len(manifest.EvidenceCorpusPaths))
		for _, path := range manifest.EvidenceCorpusPaths {
			paths = append(paths, resolveManifestPath(manifest.SourcePath, path))
		}
		hash, err := FilesSHA256(paths...)
		if err != nil {
			return Fingerprints{}, err
		}
		if fingerprints.EvidenceCorpus != "" && fingerprints.EvidenceCorpus != hash {
			return Fingerprints{}, fmt.Errorf("evidence corpus fingerprint mismatch")
		}
		fingerprints.EvidenceCorpus = hash
	}
	return fingerprints, nil
}
