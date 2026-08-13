package eval

import "testing"

func TestValidateDatasetPairAcceptsValidDatasets(t *testing.T) {
	manifest := testCorpusManifest()
	cfg := DatasetValidationConfig{
		DevelopmentMinCases:    2,
		HoldoutMinCases:        2,
		NearDuplicateThreshold: 0.9,
		CategoryTolerance:      0,
		CategoryTargets:        map[string]float64{CategoryExactEntity: 0.5, CategoryHardNegative: 0.5},
	}
	development := []EvalCase{
		{ID: "D-1", Query: "Prometheus alert rule fields", RelevantIDs: []string{"doc-a"}, Category: CategoryExactEntity, Difficulty: "easy", Language: "en"},
		{ID: "D-2", Query: "服务抖动为何持续通知", RelevantIDs: []string{"doc-b"}, Category: CategoryHardNegative, Difficulty: "hard", Language: "zh", DistractorIDs: []string{"doc-a"}},
	}
	holdout := []EvalCase{
		{ID: "H-1", Query: "告警表达式配置项说明", RelevantIDs: []string{"doc-a"}, Category: CategoryExactEntity, Difficulty: "medium", Language: "zh"},
		{ID: "H-2", Query: "发布历史和回退边界", RelevantIDs: []string{"doc-b"}, Category: CategoryHardNegative, Difficulty: "hard", Language: "zh", DistractorIDs: []string{"doc-a"}},
	}
	manifest.Development.Fingerprint = DatasetFingerprint(development)
	manifest.Holdout.Fingerprint = DatasetFingerprint(holdout)

	report := ValidateDatasetPair(development, holdout, manifest, cfg)
	if !report.Valid {
		t.Fatalf("expected valid report, issues=%v", report.Issues)
	}
	if report.Development.CoverageRate != 1 || report.Holdout.CoverageRate != 1 {
		t.Fatalf("expected full coverage, development=%v holdout=%v", report.Development.CoverageRate, report.Holdout.CoverageRate)
	}
}

func TestValidateDatasetPairRejectsInvalidLabelsAndLeakage(t *testing.T) {
	manifest := testCorpusManifest()
	cfg := DatasetValidationConfig{
		DevelopmentMinCases:    1,
		HoldoutMinCases:        1,
		NearDuplicateThreshold: 0.8,
		CategoryTolerance:      1,
		CategoryTargets:        map[string]float64{CategoryHardNegative: 1},
	}
	development := []EvalCase{{
		ID: "same", Query: "Pod CrashLoopBackOff 怎么排查", RelevantIDs: []string{"missing"},
		Category: CategoryHardNegative, Difficulty: "hard", Language: "zh", DistractorIDs: []string{"missing"},
	}}
	holdout := []EvalCase{{
		ID: "same", Query: "Pod CrashLoopBackOff该怎么排查", RelevantIDs: []string{"doc-a"},
		Category: CategoryHardNegative, Difficulty: "hard", Language: "zh", DistractorIDs: []string{"doc-b"},
	}}
	manifest.Development.Fingerprint = DatasetFingerprint(development)
	manifest.Holdout.Fingerprint = DatasetFingerprint(holdout)

	report := ValidateDatasetPair(development, holdout, manifest, cfg)
	if report.Valid {
		t.Fatal("expected invalid report")
	}
	if len(report.NearDuplicates) == 0 {
		t.Fatal("expected cross-split near duplicate")
	}
	if len(report.Issues) < 3 {
		t.Fatalf("expected multiple validation issues, got %v", report.Issues)
	}
}

func TestValidateDatasetPairMarksUndersizedDatasetsInadequate(t *testing.T) {
	manifest := testCorpusManifest()
	cfg := DatasetValidationConfig{
		DevelopmentMinCases:    2,
		HoldoutMinCases:        2,
		NearDuplicateThreshold: 1,
		CategoryTolerance:      1,
		CategoryTargets:        map[string]float64{CategoryExactEntity: 1},
	}
	development := []EvalCase{{ID: "D", Query: "query a", RelevantIDs: []string{"doc-a", "doc-b"}, Category: CategoryExactEntity, Difficulty: "easy", Language: "en"}}
	holdout := []EvalCase{{ID: "H", Query: "different query", RelevantIDs: []string{"doc-a", "doc-b"}, Category: CategoryExactEntity, Difficulty: "easy", Language: "en"}}
	manifest.Development.Fingerprint = DatasetFingerprint(development)
	manifest.Holdout.Fingerprint = DatasetFingerprint(holdout)

	report := ValidateDatasetPair(development, holdout, manifest, cfg)
	if report.Valid || report.Development.Adequate || report.Holdout.Adequate {
		t.Fatalf("expected undersized datasets to be inadequate: %+v", report)
	}
}

func testCorpusManifest() CorpusManifest {
	return CorpusManifest{
		SchemaVersion: 1,
		CorpusVersion: "test-v1",
		Documents: []CorpusDocument{
			{ID: "doc-a", Path: "a.md", Title: "A"},
			{ID: "doc-b", Path: "b.md", Title: "B"},
		},
		Development: DatasetDeclaration{Version: "dev-v1", Path: "dev.jsonl"},
		Holdout:     DatasetDeclaration{Version: "holdout-v1", Path: "holdout.jsonl"},
	}
}
