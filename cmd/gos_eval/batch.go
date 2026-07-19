package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	goseval "SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/utility/common"
)

const batchSchemaVersion = "gos-batch-v1"

type batchOptions struct {
	DatasetPath        string
	OutputDir          string
	Profile            string
	RecordedRoot       string
	RecordedTimeout    time.Duration
	CaseTimeout        time.Duration
	Resume             bool
	MaxNewCases        int
	MaxLLMCalls        int
	MinFreeDiskBytes   uint64
	CleanupCheckpoints bool
}

type batchIdentity struct {
	SchemaVersion        string              `json:"schema_version"`
	Profile              string              `json:"profile"`
	DatasetPath          string              `json:"dataset_path"`
	DatasetRole          goseval.DatasetRole `json:"dataset_role"`
	DatasetSHA256        string              `json:"dataset_sha256"`
	ConfigPath           string              `json:"config_path"`
	ConfigSHA256         string              `json:"config_sha256"`
	CodeSHA256           string              `json:"code_sha256"`
	CodeFingerprintScope string              `json:"code_fingerprint_scope"`
	EvidenceCorpusSHA256 string              `json:"evidence_corpus_sha256,omitempty"`
	EvidenceProvenance   string              `json:"evidence_provenance,omitempty"`
	ModelProvider        string              `json:"model_provider"`
	ModelName            string              `json:"model_name"`
	ModelEndpointSHA256  string              `json:"model_endpoint_sha256"`
	CaseTimeoutMS        int64               `json:"case_timeout_ms"`
	RecordedTimeoutMS    int64               `json:"recorded_timeout_ms,omitempty"`
}

type batchArtifact struct {
	Identity                batchIdentity        `json:"identity"`
	Status                  string               `json:"status"`
	Commit                  string               `json:"commit"`
	StartedAt               string               `json:"started_at"`
	UpdatedAt               string               `json:"updated_at"`
	TotalCases              int                  `json:"total_cases"`
	CompletedCases          int                  `json:"completed_cases"`
	Metrics                 *goseval.EvalMetrics `json:"metrics"`
	Results                 []goseval.EvalResult `json:"results"`
	EvaluationDatasetSHA256 string               `json:"evaluation_dataset_sha256,omitempty"`
	EvaluationCodeSHA256    string               `json:"evaluation_code_sha256,omitempty"`
	RescoredAt              string               `json:"rescored_at,omitempty"`
	RescoreSource           string               `json:"rescore_source,omitempty"`
}

func buildBatchIdentity(
	ctx context.Context,
	datasetPath, profile, recordedRoot string,
	recordedTimeout, caseTimeout time.Duration,
	dataset *goseval.EvalDataset,
) (batchIdentity, error) {
	datasetHash, err := fileSHA256(datasetPath)
	if err != nil {
		return batchIdentity{}, fmt.Errorf("hash batch dataset: %w", err)
	}
	configHash, err := fileSHA256(evalConfigPath)
	if err != nil {
		return batchIdentity{}, fmt.Errorf("hash batch config: %w", err)
	}
	codeHash, err := codeContentSHA256()
	if err != nil {
		return batchIdentity{}, fmt.Errorf("hash batch runtime code: %w", err)
	}
	modelProvider := "deterministic"
	modelName := "gos-deterministic-eval-v1"
	modelEndpointHash := "deterministic"
	if profile != "eval" {
		modelConfig, loadErr := common.LoadChatModelConfig(ctx, common.ChatModelFast)
		if loadErr != nil {
			return batchIdentity{}, fmt.Errorf("load batch model metadata: %w", loadErr)
		}
		modelProvider = modelConfig.Provider
		modelName = modelConfig.Model
		endpointDigest := sha256.Sum256([]byte(modelConfig.Provider + "\x00" + modelConfig.Model + "\x00" + modelConfig.BaseURL))
		modelEndpointHash = fmt.Sprintf("%x", endpointDigest)
	}

	identity := batchIdentity{
		SchemaVersion:        batchSchemaVersion,
		Profile:              profile,
		DatasetPath:          datasetPath,
		DatasetRole:          dataset.Role,
		DatasetSHA256:        datasetHash,
		ConfigPath:           evalConfigPath,
		ConfigSHA256:         configHash,
		CodeSHA256:           codeHash,
		CodeFingerprintScope: runtimeCodeFingerprintScope,
		ModelProvider:        modelProvider,
		ModelName:            modelName,
		ModelEndpointSHA256:  modelEndpointHash,
		CaseTimeoutMS:        caseTimeout.Milliseconds(),
		RecordedTimeoutMS:    recordedTimeout.Milliseconds(),
	}
	if profile == "recorded" {
		evidenceHash, hashErr := recordedCorpusSHA256(ctx, datasetPath, recordedRoot, recordedTimeout)
		if hashErr != nil {
			return batchIdentity{}, fmt.Errorf("hash recorded batch corpus: %w", hashErr)
		}
		identity.EvidenceCorpusSHA256 = evidenceHash
		identity.EvidenceProvenance = "recorded_blind"
	}
	return identity, nil
}

func runGoSBatch(ctx context.Context, options batchOptions) error {
	dataset, err := loadDatasetForMode(options.DatasetPath, "gos-batch", options.Profile)
	if err != nil {
		return fmt.Errorf("load batch dataset: %w", err)
	}
	runner, _, err := buildGoSRunner(options.Profile, options.RecordedRoot, options.RecordedTimeout)
	if err != nil {
		return fmt.Errorf("build batch runner: %w", err)
	}
	identity, err := buildBatchIdentity(
		ctx,
		options.DatasetPath,
		options.Profile,
		options.RecordedRoot,
		options.RecordedTimeout,
		options.CaseTimeout,
		dataset,
	)
	if err != nil {
		return err
	}
	artifact, err := executeGoSBatch(ctx, dataset, runner, identity, options)
	if artifact != nil {
		printMetrics("GoS Batch", artifact.Metrics)
		fmt.Printf(
			"GoS batch status=%s completed=%d/%d output=%s\n",
			artifact.Status,
			artifact.CompletedCases,
			artifact.TotalCases,
			filepath.Join(options.OutputDir, "run.json"),
		)
	}
	return err
}

func runPlanBatch(ctx context.Context, options batchOptions) error {
	dataset, err := loadDatasetForMode(options.DatasetPath, "baseline", options.Profile)
	if err != nil {
		return fmt.Errorf("load plan batch dataset: %w", err)
	}
	identity, err := buildBatchIdentity(
		ctx,
		options.DatasetPath,
		options.Profile,
		options.RecordedRoot,
		options.RecordedTimeout,
		options.CaseTimeout,
		dataset,
	)
	if err != nil {
		return err
	}
	modelConfig, err := common.LoadChatModelConfig(ctx, common.ChatModelPrimary)
	if err != nil {
		return fmt.Errorf("load plan batch model metadata: %w", err)
	}
	identity.ModelProvider = modelConfig.Provider
	identity.ModelName = modelConfig.Model
	endpointDigest := sha256.Sum256([]byte(modelConfig.Provider + "\x00" + modelConfig.Model + "\x00" + modelConfig.BaseURL))
	identity.ModelEndpointSHA256 = fmt.Sprintf("%x", endpointDigest)
	identity.CodeSHA256, err = baselineCodeContentSHA256()
	if err != nil {
		return fmt.Errorf("hash plan batch runtime code: %w", err)
	}
	identity.CodeFingerprintScope = baselineCodeFingerprintScope

	runCase := func(caseCtx context.Context, evalCase goseval.EvalCase) (goseval.EvalResult, error) {
		result, caseErr := runPlanBaselineCase(
			caseCtx,
			evalCase,
			options.Profile,
			options.RecordedRoot,
			options.RecordedTimeout,
		)
		if caseErr != nil {
			return goseval.EvalResult{}, caseErr
		}
		return *result, nil
	}
	artifact, err := executeEvalBatch(ctx, dataset, runCase, identity, options)
	if artifact != nil {
		printMetrics("Plan Batch", artifact.Metrics)
		fmt.Printf(
			"Plan batch status=%s completed=%d/%d output=%s\n",
			artifact.Status,
			artifact.CompletedCases,
			artifact.TotalCases,
			filepath.Join(options.OutputDir, "run.json"),
		)
	}
	return err
}

func executeGoSBatch(
	ctx context.Context,
	dataset *goseval.EvalDataset,
	runner *goseval.Runner,
	identity batchIdentity,
	options batchOptions,
) (*batchArtifact, error) {
	if runner == nil {
		return nil, errors.New("batch runner is required")
	}
	return executeEvalBatch(ctx, dataset, runner.RunCase, identity, options)
}

func executeEvalBatch(
	ctx context.Context,
	dataset *goseval.EvalDataset,
	runCase func(context.Context, goseval.EvalCase) (goseval.EvalResult, error),
	identity batchIdentity,
	options batchOptions,
) (*batchArtifact, error) {
	if dataset == nil || runCase == nil {
		return nil, errors.New("batch dataset and case runner are required")
	}
	if options.CaseTimeout <= 0 {
		return nil, errors.New("batch case timeout must be positive")
	}
	if options.MaxNewCases < 0 || options.MaxLLMCalls < 0 {
		return nil, errors.New("batch limits cannot be negative")
	}
	if err := os.MkdirAll(options.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("create batch output directory: %w", err)
	}

	startedAt := time.Now().Format(time.RFC3339)
	manifestPath := filepath.Join(options.OutputDir, "run.json")
	if existing, err := loadBatchArtifact(manifestPath); err == nil {
		if !options.Resume {
			return nil, fmt.Errorf("batch output already exists at %s; enable resume or choose another directory", options.OutputDir)
		}
		if err := validateBatchIdentity(existing.Identity, identity); err != nil {
			return nil, err
		}
		if existing.Status == "completed" {
			return existing, nil
		}
		startedAt = existing.StartedAt
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(options.OutputDir, "cases"), 0o755); err != nil {
		return nil, fmt.Errorf("create batch checkpoint directory: %w", err)
	}

	resultsByIndex, err := loadBatchCheckpoints(filepath.Join(options.OutputDir, "cases"), dataset.Cases)
	if err != nil {
		return nil, err
	}
	newCases := 0
	for index, evalCase := range dataset.Cases {
		if _, completed := resultsByIndex[index]; completed {
			continue
		}
		if options.MaxNewCases > 0 && newCases >= options.MaxNewCases {
			break
		}
		current := buildBatchArtifact(identity, startedAt, dataset.Cases, resultsByIndex)
		if options.MaxLLMCalls > 0 && batchLLMCalls(current.Results) >= options.MaxLLMCalls {
			break
		}
		if err := ensureBatchFreeDisk(options.OutputDir, options.MinFreeDiskBytes); err != nil {
			return current, err
		}

		caseCtx, cancel := context.WithTimeout(ctx, options.CaseTimeout)
		result, runErr := runCase(caseCtx, evalCase)
		cancel()
		if runErr != nil {
			return buildBatchArtifact(identity, startedAt, dataset.Cases, resultsByIndex), runErr
		}
		if err := writeJSONAtomic(batchCheckpointPath(options.OutputDir, index), result); err != nil {
			return nil, fmt.Errorf("write checkpoint for case %q: %w", evalCase.ID, err)
		}
		resultsByIndex[index] = result
		newCases++

		progress := buildBatchArtifact(identity, startedAt, dataset.Cases, resultsByIndex)
		if err := writeJSONAtomic(manifestPath, progress); err != nil {
			return nil, fmt.Errorf("write batch progress: %w", err)
		}
		fmt.Printf(
			"Batch progress: %d/%d case=%s status=%s matched=%t llm_calls=%d\n",
			progress.CompletedCases,
			progress.TotalCases,
			result.CaseID,
			result.Status,
			result.Matched,
			result.LLMCalls,
		)
	}

	artifact := buildBatchArtifact(identity, startedAt, dataset.Cases, resultsByIndex)
	if err := writeJSONAtomic(manifestPath, artifact); err != nil {
		return nil, fmt.Errorf("write batch artifact: %w", err)
	}
	if artifact.Status == "completed" && options.CleanupCheckpoints {
		if err := os.RemoveAll(filepath.Join(options.OutputDir, "cases")); err != nil {
			return nil, fmt.Errorf("cleanup batch checkpoints: %w", err)
		}
	}
	return artifact, nil
}

func buildBatchArtifact(
	identity batchIdentity,
	startedAt string,
	cases []goseval.EvalCase,
	resultsByIndex map[int]goseval.EvalResult,
) *batchArtifact {
	indexes := make([]int, 0, len(resultsByIndex))
	for index := range resultsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	results := make([]goseval.EvalResult, 0, len(indexes))
	metrics := goseval.NewEvalMetrics()
	for _, index := range indexes {
		result := resultsByIndex[index]
		results = append(results, result)
		metrics.AddResult(&result)
	}
	metrics.Finalize()
	status := "partial"
	if len(results) == len(cases) {
		status = "completed"
	}
	return &batchArtifact{
		Identity:       identity,
		Status:         status,
		Commit:         gitCommit(),
		StartedAt:      startedAt,
		UpdatedAt:      time.Now().Format(time.RFC3339),
		TotalCases:     len(cases),
		CompletedCases: len(results),
		Metrics:        metrics,
		Results:        results,
	}
}

func loadBatchArtifact(path string) (*batchArtifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var artifact batchArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		return nil, fmt.Errorf("decode batch artifact: %w", err)
	}
	return &artifact, nil
}

func rescoreGoSBatch(datasetPath, inputPath, outputPath string) (*batchArtifact, error) {
	dataset, err := goseval.LoadDataset(datasetPath)
	if err != nil {
		return nil, fmt.Errorf("load rescore dataset: %w", err)
	}
	artifact, err := loadBatchArtifact(inputPath)
	if err != nil {
		return nil, fmt.Errorf("load batch artifact for rescore: %w", err)
	}
	if len(artifact.Results) > len(dataset.Cases) {
		return nil, errors.New("batch artifact has more results than the rescore dataset")
	}

	metrics := goseval.NewEvalMetrics()
	for index := range artifact.Results {
		result := &artifact.Results[index]
		evalCase := dataset.Cases[index]
		if result.CaseID != evalCase.ID || result.GroundTruth != evalCase.GroundTruth {
			return nil, fmt.Errorf("batch result %d does not match rescore case %q", index+1, evalCase.ID)
		}
		result.Matched = goseval.MatchCasePrediction(result.Prediction, evalCase)
		result.PrematureStop = goseval.IsPrematureStop(
			result.Status,
			result.StatusMatched,
			evalCase.RequireRefine,
			result.Refined,
			evalCase.RequireBacktrack,
			result.Backtracked,
		)
		if result.Status == "succeeded" && result.StatusMatched &&
			(!evalCase.RequireRefine || result.Refined) &&
			(!evalCase.RequireBacktrack || result.Backtracked) {
			if result.Matched {
				result.FailurePhase = ""
			} else {
				result.FailurePhase = "report"
			}
		}
		result.FailurePhaseMatched = evalCase.ExpectedFailurePhase == "" || evalCase.ExpectedFailurePhase == result.FailurePhase
		result.ContractMatched = result.StatusMatched && result.FailurePhaseMatched &&
			(!evalCase.RequireRefine || result.Refined) &&
			(!evalCase.RequireBacktrack || result.Backtracked)
		metrics.AddResult(result)
	}
	metrics.Finalize()
	datasetHash, err := fileSHA256(datasetPath)
	if err != nil {
		return nil, fmt.Errorf("hash rescore dataset: %w", err)
	}
	codeHash, err := codeContentSHA256()
	if err != nil {
		return nil, fmt.Errorf("hash rescore evaluator code: %w", err)
	}
	artifact.Metrics = metrics
	artifact.UpdatedAt = time.Now().Format(time.RFC3339)
	artifact.EvaluationDatasetSHA256 = datasetHash
	artifact.EvaluationCodeSHA256 = codeHash
	artifact.RescoredAt = artifact.UpdatedAt
	artifact.RescoreSource = inputPath
	if err := writeJSONAtomic(outputPath, artifact); err != nil {
		return nil, fmt.Errorf("write rescored batch artifact: %w", err)
	}
	return artifact, nil
}

func loadBatchCheckpoints(dir string, cases []goseval.EvalCase) (map[int]goseval.EvalResult, error) {
	results := make(map[int]goseval.EvalResult)
	for index, evalCase := range cases {
		data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%04d.json", index+1)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var result goseval.EvalResult
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("decode checkpoint %d: %w", index+1, err)
		}
		if result.CaseID != evalCase.ID || result.GroundTruth != evalCase.GroundTruth {
			return nil, fmt.Errorf("checkpoint %d does not match dataset case %q", index+1, evalCase.ID)
		}
		results[index] = result
	}
	return results, nil
}

func validateBatchIdentity(existing, current batchIdentity) error {
	if existing != current {
		return errors.New("batch checkpoint identity does not match current dataset, code, config, model, evidence, or timeout")
	}
	return nil
}

func batchCheckpointPath(outputDir string, index int) string {
	return filepath.Join(outputDir, "cases", fmt.Sprintf("%04d.json", index+1))
}

func batchLLMCalls(results []goseval.EvalResult) int {
	total := 0
	for _, result := range results {
		total += result.LLMCalls
	}
	return total
}

func ensureBatchFreeDisk(path string, minimum uint64) error {
	if minimum == 0 {
		return nil
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return fmt.Errorf("inspect batch disk space: %w", err)
	}
	available := uint64(stat.Bavail) * uint64(stat.Bsize)
	if available < minimum {
		return fmt.Errorf("batch stopped before disk exhaustion: available=%d minimum=%d", available, minimum)
	}
	return nil
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".gos-batch-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := file.Name()
	defer os.Remove(tmpPath)
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
