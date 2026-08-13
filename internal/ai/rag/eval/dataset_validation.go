package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"unicode"
)

const (
	CategoryExactEntity        = "exact_entity"
	CategorySemanticParaphrase = "semantic_paraphrase"
	CategoryMultiDocument      = "multi_document"
	CategoryCrossLanguage      = "cross_language"
	CategoryHardNegative       = "hard_negative"
)

var validDifficulties = map[string]struct{}{
	"easy": {}, "medium": {}, "hard": {},
}

var validLanguages = map[string]struct{}{
	"zh": {}, "en": {}, "mixed": {},
}

type CorpusDocument struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Title string `json:"title"`
}

type ExcludedCorpusDocument struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type DatasetDeclaration struct {
	Version     string `json:"version"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

type CorpusManifest struct {
	SchemaVersion int                      `json:"schema_version"`
	CorpusVersion string                   `json:"corpus_version"`
	Documents     []CorpusDocument         `json:"documents"`
	Excluded      []ExcludedCorpusDocument `json:"excluded,omitempty"`
	Development   DatasetDeclaration       `json:"development"`
	Holdout       DatasetDeclaration       `json:"holdout"`
}

type DatasetValidationConfig struct {
	DevelopmentMinCases    int                `json:"development_min_cases"`
	HoldoutMinCases        int                `json:"holdout_min_cases"`
	NearDuplicateThreshold float64            `json:"near_duplicate_threshold"`
	CategoryTolerance      float64            `json:"category_tolerance"`
	CategoryTargets        map[string]float64 `json:"category_targets"`
}

type DatasetStats struct {
	Role               string         `json:"role"`
	Cases              int            `json:"cases"`
	Adequate           bool           `json:"adequate"`
	Fingerprint        string         `json:"fingerprint"`
	CategoryCounts     map[string]int `json:"category_counts"`
	DifficultyCounts   map[string]int `json:"difficulty_counts"`
	LanguageCounts     map[string]int `json:"language_counts"`
	CoveredDocumentIDs []string       `json:"covered_document_ids"`
	MissingDocumentIDs []string       `json:"missing_document_ids"`
	CoverageRate       float64        `json:"coverage_rate"`
}

type NearDuplicate struct {
	DevelopmentID string  `json:"development_id"`
	HoldoutID     string  `json:"holdout_id"`
	Similarity    float64 `json:"similarity"`
}

type DatasetValidationReport struct {
	Valid          bool                    `json:"valid"`
	CorpusVersion  string                  `json:"corpus_version"`
	Config         DatasetValidationConfig `json:"config"`
	Development    DatasetStats            `json:"development"`
	Holdout        DatasetStats            `json:"holdout"`
	NearDuplicates []NearDuplicate         `json:"near_duplicates,omitempty"`
	Issues         []string                `json:"issues,omitempty"`
}

func DefaultDatasetValidationConfig() DatasetValidationConfig {
	return DatasetValidationConfig{
		DevelopmentMinCases:    60,
		HoldoutMinCases:        100,
		NearDuplicateThreshold: 0.8,
		CategoryTolerance:      0.05,
		CategoryTargets: map[string]float64{
			CategoryExactEntity:        0.20,
			CategorySemanticParaphrase: 0.25,
			CategoryMultiDocument:      0.20,
			CategoryCrossLanguage:      0.10,
			CategoryHardNegative:       0.25,
		},
	}
}

func LoadCorpusManifest(path string) (CorpusManifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return CorpusManifest{}, fmt.Errorf("read corpus manifest %s: %w", path, err)
	}
	var manifest CorpusManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return CorpusManifest{}, fmt.Errorf("decode corpus manifest %s: %w", path, err)
	}
	return manifest, nil
}

func ValidateDatasetPair(development, holdout []EvalCase, manifest CorpusManifest, cfg DatasetValidationConfig) DatasetValidationReport {
	report := DatasetValidationReport{
		Valid:         true,
		CorpusVersion: manifest.CorpusVersion,
		Config:        cfg,
	}
	eligible, manifestIssues := validateManifest(manifest)
	report.Issues = append(report.Issues, manifestIssues...)
	if cfg.NearDuplicateThreshold <= 0 || cfg.NearDuplicateThreshold > 1 {
		report.Issues = append(report.Issues, "near_duplicate_threshold must be within (0,1]")
	}
	if cfg.CategoryTolerance < 0 || cfg.CategoryTolerance > 1 {
		report.Issues = append(report.Issues, "category_tolerance must be within [0,1]")
	}

	report.Development = validateDataset("development", development, cfg.DevelopmentMinCases, eligible, cfg, &report.Issues)
	report.Holdout = validateDataset("holdout", holdout, cfg.HoldoutMinCases, eligible, cfg, &report.Issues)
	validateDatasetIDsAcrossSplits(development, holdout, &report.Issues)
	report.NearDuplicates = findNearDuplicates(development, holdout, cfg.NearDuplicateThreshold)
	for _, pair := range report.NearDuplicates {
		report.Issues = append(report.Issues, fmt.Sprintf(
			"cross-split near duplicate: development=%s holdout=%s similarity=%.3f",
			pair.DevelopmentID, pair.HoldoutID, pair.Similarity,
		))
	}
	validateDeclaredFingerprint("development", manifest.Development.Fingerprint, report.Development.Fingerprint, &report.Issues)
	validateDeclaredFingerprint("holdout", manifest.Holdout.Fingerprint, report.Holdout.Fingerprint, &report.Issues)

	report.Development.Adequate = datasetAdequate(report.Development, cfg.DevelopmentMinCases, cfg)
	report.Holdout.Adequate = datasetAdequate(report.Holdout, cfg.HoldoutMinCases, cfg)
	report.Valid = len(report.Issues) == 0 && report.Development.Adequate && report.Holdout.Adequate
	return report
}

func DatasetFingerprint(cases []EvalCase) string {
	raw, _ := json.Marshal(cases)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validateManifest(manifest CorpusManifest) (map[string]struct{}, []string) {
	eligible := make(map[string]struct{}, len(manifest.Documents))
	issues := make([]string, 0)
	if manifest.SchemaVersion <= 0 {
		issues = append(issues, "manifest schema_version must be positive")
	}
	if strings.TrimSpace(manifest.CorpusVersion) == "" {
		issues = append(issues, "manifest corpus_version is required")
	}
	for index, doc := range manifest.Documents {
		id := strings.TrimSpace(doc.ID)
		if id == "" || strings.TrimSpace(doc.Path) == "" {
			issues = append(issues, fmt.Sprintf("manifest document %d requires id and path", index))
			continue
		}
		if _, exists := eligible[id]; exists {
			issues = append(issues, fmt.Sprintf("manifest contains duplicate document id %q", id))
			continue
		}
		eligible[id] = struct{}{}
	}
	if len(eligible) == 0 {
		issues = append(issues, "manifest requires at least one eligible document")
	}
	return eligible, issues
}

func validateDataset(role string, cases []EvalCase, minCases int, eligible map[string]struct{}, cfg DatasetValidationConfig, issues *[]string) DatasetStats {
	stats := DatasetStats{
		Role:             role,
		Cases:            len(cases),
		Fingerprint:      DatasetFingerprint(cases),
		CategoryCounts:   make(map[string]int),
		DifficultyCounts: make(map[string]int),
		LanguageCounts:   make(map[string]int),
	}
	if len(cases) < minCases {
		*issues = append(*issues, fmt.Sprintf("%s requires at least %d cases, got %d", role, minCases, len(cases)))
	}

	ids := make(map[string]struct{}, len(cases))
	queries := make(map[string]string, len(cases))
	covered := make(map[string]struct{}, len(eligible))
	for index, item := range cases {
		id := strings.TrimSpace(item.ID)
		query := strings.TrimSpace(item.Query)
		if id == "" || query == "" {
			*issues = append(*issues, fmt.Sprintf("%s case %d requires id and query", role, index))
		}
		if _, exists := ids[id]; exists {
			*issues = append(*issues, fmt.Sprintf("%s contains duplicate case id %q", role, id))
		}
		ids[id] = struct{}{}
		normalized := normalizeQuery(query)
		if prior, exists := queries[normalized]; exists && normalized != "" {
			*issues = append(*issues, fmt.Sprintf("%s contains duplicate query in %s and %s", role, prior, id))
		}
		queries[normalized] = id

		if len(item.RelevantIDs) == 0 {
			*issues = append(*issues, fmt.Sprintf("%s case %q requires relevant_ids", role, id))
		}
		validateDocumentIDs(role, id, "relevant_ids", item.RelevantIDs, eligible, covered, issues)
		validateDocumentIDs(role, id, "distractor_ids", item.DistractorIDs, eligible, nil, issues)
		if intersects(item.RelevantIDs, item.DistractorIDs) {
			*issues = append(*issues, fmt.Sprintf("%s case %q has overlapping relevant_ids and distractor_ids", role, id))
		}

		if _, ok := cfg.CategoryTargets[item.Category]; !ok {
			*issues = append(*issues, fmt.Sprintf("%s case %q has invalid category %q", role, id, item.Category))
		}
		if item.Category == CategoryHardNegative && len(item.DistractorIDs) == 0 {
			*issues = append(*issues, fmt.Sprintf("%s hard-negative case %q requires distractor_ids", role, id))
		}
		if _, ok := validDifficulties[item.Difficulty]; !ok {
			*issues = append(*issues, fmt.Sprintf("%s case %q has invalid difficulty %q", role, id, item.Difficulty))
		}
		if _, ok := validLanguages[item.Language]; !ok {
			*issues = append(*issues, fmt.Sprintf("%s case %q has invalid language %q", role, id, item.Language))
		}
		stats.CategoryCounts[item.Category]++
		stats.DifficultyCounts[item.Difficulty]++
		stats.LanguageCounts[item.Language]++
	}

	for id := range eligible {
		if _, ok := covered[id]; ok {
			stats.CoveredDocumentIDs = append(stats.CoveredDocumentIDs, id)
		} else {
			stats.MissingDocumentIDs = append(stats.MissingDocumentIDs, id)
		}
	}
	sort.Strings(stats.CoveredDocumentIDs)
	sort.Strings(stats.MissingDocumentIDs)
	if len(eligible) > 0 {
		stats.CoverageRate = float64(len(stats.CoveredDocumentIDs)) / float64(len(eligible))
	}
	if len(stats.MissingDocumentIDs) > 0 {
		*issues = append(*issues, fmt.Sprintf("%s does not cover %d eligible documents", role, len(stats.MissingDocumentIDs)))
	}
	validateCategoryDistribution(role, stats, cfg, issues)
	return stats
}

func validateDocumentIDs(role, caseID, field string, ids []string, eligible map[string]struct{}, covered map[string]struct{}, issues *[]string) {
	seen := make(map[string]struct{}, len(ids))
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" {
			*issues = append(*issues, fmt.Sprintf("%s case %q contains empty %s", role, caseID, field))
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			*issues = append(*issues, fmt.Sprintf("%s case %q contains duplicate %s %q", role, caseID, field, id))
			continue
		}
		seen[id] = struct{}{}
		if _, exists := eligible[id]; !exists {
			*issues = append(*issues, fmt.Sprintf("%s case %q references unknown %s %q", role, caseID, field, id))
			continue
		}
		if covered != nil {
			covered[id] = struct{}{}
		}
	}
}

func validateCategoryDistribution(role string, stats DatasetStats, cfg DatasetValidationConfig, issues *[]string) {
	if stats.Cases == 0 {
		return
	}
	for category, target := range cfg.CategoryTargets {
		actual := float64(stats.CategoryCounts[category]) / float64(stats.Cases)
		if math.Abs(actual-target) > cfg.CategoryTolerance+1e-9 {
			*issues = append(*issues, fmt.Sprintf(
				"%s category %s ratio %.3f is outside target %.3f±%.3f",
				role, category, actual, target, cfg.CategoryTolerance,
			))
		}
	}
}

func datasetAdequate(stats DatasetStats, minCases int, cfg DatasetValidationConfig) bool {
	if stats.Cases < minCases || len(stats.MissingDocumentIDs) > 0 {
		return false
	}
	for category, target := range cfg.CategoryTargets {
		actual := float64(stats.CategoryCounts[category]) / float64(stats.Cases)
		if math.Abs(actual-target) > cfg.CategoryTolerance+1e-9 {
			return false
		}
	}
	return true
}

func validateDatasetIDsAcrossSplits(development, holdout []EvalCase, issues *[]string) {
	ids := make(map[string]struct{}, len(development))
	for _, item := range development {
		ids[strings.TrimSpace(item.ID)] = struct{}{}
	}
	for _, item := range holdout {
		id := strings.TrimSpace(item.ID)
		if _, exists := ids[id]; exists && id != "" {
			*issues = append(*issues, fmt.Sprintf("duplicate case id across splits %q", id))
		}
	}
}

func findNearDuplicates(development, holdout []EvalCase, threshold float64) []NearDuplicate {
	if threshold <= 0 || threshold > 1 {
		return nil
	}
	duplicates := make([]NearDuplicate, 0)
	for _, dev := range development {
		for _, test := range holdout {
			similarity := querySimilarity(dev.Query, test.Query)
			if similarity >= threshold {
				duplicates = append(duplicates, NearDuplicate{
					DevelopmentID: dev.ID,
					HoldoutID:     test.ID,
					Similarity:    similarity,
				})
			}
		}
	}
	sort.Slice(duplicates, func(i, j int) bool {
		if duplicates[i].Similarity == duplicates[j].Similarity {
			if duplicates[i].DevelopmentID == duplicates[j].DevelopmentID {
				return duplicates[i].HoldoutID < duplicates[j].HoldoutID
			}
			return duplicates[i].DevelopmentID < duplicates[j].DevelopmentID
		}
		return duplicates[i].Similarity > duplicates[j].Similarity
	})
	return duplicates
}

func querySimilarity(a, b string) float64 {
	aNormalized := normalizeQuery(a)
	bNormalized := normalizeQuery(b)
	if aNormalized == "" || bNormalized == "" {
		return 0
	}
	if aNormalized == bNormalized {
		return 1
	}
	aTokens := queryBigrams(aNormalized)
	bTokens := queryBigrams(bNormalized)
	intersection := 0
	for token := range aTokens {
		if _, ok := bTokens[token]; ok {
			intersection++
		}
	}
	union := len(aTokens) + len(bTokens) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func normalizeQuery(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(raw)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func queryBigrams(normalized string) map[string]struct{} {
	runes := []rune(normalized)
	tokens := make(map[string]struct{})
	if len(runes) < 2 {
		if len(runes) == 1 {
			tokens[string(runes)] = struct{}{}
		}
		return tokens
	}
	for i := 0; i < len(runes)-1; i++ {
		tokens[string(runes[i:i+2])] = struct{}{}
	}
	return tokens
}

func intersects(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, id := range left {
		seen[strings.TrimSpace(id)] = struct{}{}
	}
	for _, id := range right {
		if _, ok := seen[strings.TrimSpace(id)]; ok {
			return true
		}
	}
	return false
}

func validateDeclaredFingerprint(role, declared, actual string, issues *[]string) {
	if strings.TrimSpace(declared) == "" {
		*issues = append(*issues, fmt.Sprintf("%s fingerprint is not sealed in manifest", role))
		return
	}
	if declared != actual {
		*issues = append(*issues, fmt.Sprintf("%s fingerprint differs from manifest", role))
	}
}
