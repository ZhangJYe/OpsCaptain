package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalCorpusContract(t *testing.T) {
	development, err := LoadDataset("testdata/dev.json")
	require.NoError(t, err)
	holdout, err := LoadDataset("testdata/holdout.json")
	require.NoError(t, err)
	regression, err := LoadDataset("testdata/smoke.json")
	require.NoError(t, err)

	require.NoError(t, ValidateCorpus(development, holdout))
	assert.Len(t, development.Cases, 24)
	assert.Len(t, holdout.Cases, 16)
	assert.Len(t, regression.Cases, 5)
	assert.GreaterOrEqual(t, len(development.Cases)+len(holdout.Cases), 30)
}

func TestValidateModeDatasetIsolation(t *testing.T) {
	holdout, err := LoadDataset("testdata/holdout.json")
	require.NoError(t, err)
	development, err := LoadDataset("testdata/dev.json")
	require.NoError(t, err)
	regression, err := LoadDataset("testdata/smoke.json")
	require.NoError(t, err)

	assert.NoError(t, ValidateModeDataset("compare", "real", holdout))
	assert.NoError(t, ValidateModeDataset("compare", "recorded", holdout))
	assert.NoError(t, ValidateModeDataset("baseline", "recorded", holdout))
	assert.NoError(t, ValidateModeDataset("baseline", "recorded", development))
	assert.NoError(t, ValidateModeDataset("compare", "recorded", development))
	assert.NoError(t, ValidateModeDataset("gos", "recorded", holdout))
	assert.NoError(t, ValidateModeDataset("gos-batch", "recorded", holdout))
	assert.NoError(t, ValidateModeDataset("gate", "eval", regression))
	assert.Error(t, ValidateModeDataset("gate", "eval", holdout))
	assert.Error(t, ValidateModeDataset("gos", "eval", holdout))
	assert.Error(t, ValidateModeDataset("compare", "eval", holdout))
	assert.Error(t, ValidateModeDataset("compare", "real", development))
}

func TestValidateModeDatasetRejectsWeakRecordedMatcher(t *testing.T) {
	dataset := &EvalDataset{
		SchemaVersion: DatasetSchemaVersion,
		Role:          DatasetRoleHoldout,
		Cases: []EvalCase{{
			ID:               "recorded-case",
			Domain:           "service",
			Scenario:         "recorded_blind_root_cause",
			Symptom:          "analyze this incident window",
			GroundTruth:      "dns error; affected entity checkoutservice",
			ExpectedKeywords: []string{"dns error", "checkoutservice"},
		}},
	}

	assert.ErrorContains(t, ValidateModeDataset("gos-batch", "recorded", dataset), "structured cause and entity")
	dataset.Cases[0].ExpectedCauseKeywords = []string{"dns error"}
	dataset.Cases[0].ExpectedEntityKeywords = []string{"checkoutservice"}
	assert.NoError(t, ValidateModeDataset("gos-batch", "recorded", dataset))
}

func TestValidateCorpusRejectsOverlap(t *testing.T) {
	development, err := LoadDataset("testdata/dev.json")
	require.NoError(t, err)
	holdout, err := LoadDataset("testdata/holdout.json")
	require.NoError(t, err)

	holdout.Cases[0].ID = development.Cases[0].ID
	assert.ErrorContains(t, ValidateCorpus(development, holdout), "not disjoint")
}

func TestValidateDatasetRequiresFailurePhaseContract(t *testing.T) {
	dataset := &EvalDataset{
		SchemaVersion: DatasetSchemaVersion,
		Role:          DatasetRoleDevelopment,
		Cases: []EvalCase{{
			ID:             "failure-case",
			Domain:         "cpu",
			Scenario:       "tool_timeout",
			Symptom:        "tool timed out",
			GroundTruth:    "tool unavailable",
			ExpectedStatus: "degraded",
		}},
	}

	assert.ErrorContains(t, ValidateDataset(dataset), "expected_failure_phase")
	dataset.Cases[0].ExpectedFailurePhase = "act"
	assert.NoError(t, ValidateDataset(dataset))
}

func TestHoldoutDoesNotLeakIntoRuntimeSources(t *testing.T) {
	holdout, err := LoadDataset("testdata/holdout.json")
	require.NoError(t, err)
	repoRoot := findRepoRoot(t)

	paths := []string{
		filepath.Join(repoRoot, "cmd", "gos_eval"),
		filepath.Join(repoRoot, "internal", "ai", "agent", "experts"),
		filepath.Join(repoRoot, "internal", "ai", "agent", "gos_engine"),
		filepath.Join(repoRoot, "internal", "ai", "prompts"),
		filepath.Join(repoRoot, "manifest", "config"),
	}
	for _, root := range paths {
		info, statErr := os.Stat(root)
		if os.IsNotExist(statErr) {
			continue
		}
		require.NoError(t, statErr)
		require.True(t, info.IsDir())
		err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if filepath.Clean(path) == filepath.Join(repoRoot, "internal", "ai", "agent", "gos_engine", "eval", "testdata") {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".go" && filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" && filepath.Ext(path) != ".md" {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			content := string(data)
			for _, evalCase := range holdout.Cases {
				if strings.Contains(content, evalCase.ID) || strings.Contains(content, evalCase.Symptom) || strings.Contains(content, evalCase.GroundTruth) {
					t.Errorf("holdout case %s leaks into runtime source %s", evalCase.ID, path)
				}
			}
			return nil
		})
		require.NoError(t, err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatal("go.mod not found")
		}
		current = parent
	}
}
