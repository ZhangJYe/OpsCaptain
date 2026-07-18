package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	einoretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	goseval "SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/utility/common"
)

type probeRetriever struct {
	docs []*schema.Document
	err  error
}

func (r probeRetriever) Retrieve(_ context.Context, _ string, _ ...einoretriever.Option) ([]*schema.Document, error) {
	return r.docs, r.err
}

func writeTestDataset(t *testing.T, path, symptom string) {
	t.Helper()
	data, err := json.Marshal(goseval.EvalDataset{
		SchemaVersion: goseval.DatasetSchemaVersion,
		Role:          goseval.DatasetRoleHoldout,
		Cases: []goseval.EvalCase{
			{ID: "holdout-test", Domain: "cpu", Scenario: "support_evidence", Symptom: symptom, GroundTruth: "root cause"},
		},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o600))
}

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "holdout.json")
	require.NoError(t, os.WriteFile(path, []byte("opscaptain"), 0o600))

	hash, err := fileSHA256(path)

	require.NoError(t, err)
	require.Equal(t, "00c5dccefa2c18d61da7bb6d4203ef3fdf12bff8cfa1ad03d903ac07de33aab4", hash)
}

func TestFileSHA256MissingFile(t *testing.T) {
	_, err := fileSHA256(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
}

func TestContentFilesSHA256IsOrderIndependentAndContentSensitive(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.go"), []byte("package b\n"), 0o600))

	first, err := contentFilesSHA256(root, []string{"b.go", "a.go"})
	require.NoError(t, err)
	second, err := contentFilesSHA256(root, []string{"a.go", "b.go"})
	require.NoError(t, err)
	require.Equal(t, first, second)

	require.NoError(t, os.WriteFile(filepath.Join(root, "b.go"), []byte("package changed\n"), 0o600))
	changed, err := contentFilesSHA256(root, []string{"a.go", "b.go"})
	require.NoError(t, err)
	require.NotEqual(t, first, changed)
	_, err = contentFilesSHA256(root, []string{"../outside.go"})
	require.Error(t, err)
}

func TestValidateArtifactFingerprints(t *testing.T) {
	dir := t.TempDir()
	holdoutPath := filepath.Join(dir, "holdout.json")
	configPath := filepath.Join(dir, "config.yaml")
	writeTestDataset(t, holdoutPath, "holdout-v1")
	require.NoError(t, os.WriteFile(configPath, []byte("config-v1"), 0o600))
	holdoutHash, err := fileSHA256(holdoutPath)
	require.NoError(t, err)
	configHash, err := fileSHA256(configPath)
	require.NoError(t, err)
	codeHash, err := baselineCodeContentSHA256()
	require.NoError(t, err)

	artifact := BaselineArtifact{
		Profile:                "recorded",
		DatasetSchemaVersion:   goseval.DatasetSchemaVersion,
		DatasetRole:            goseval.DatasetRoleHoldout,
		DatasetSHA256:          holdoutHash,
		ConfigSHA256:           configHash,
		CodeSHA256:             codeHash,
		CodeFingerprintScope:   baselineCodeFingerprintScope,
		EvidenceMetricContract: evidenceMetricContract,
		PromptVersion:          "test-v1",
		DependencyState:        map[string]string{"llm": "test"},
	}
	require.NoError(t, validateArtifactFingerprints(artifact, holdoutPath, configPath))

	writeTestDataset(t, holdoutPath, "holdout-v2")
	require.Error(t, validateArtifactFingerprints(artifact, holdoutPath, configPath))
}

func TestValidateArtifactFingerprintsRejectsLegacyArtifact(t *testing.T) {
	dir := t.TempDir()
	holdoutPath := filepath.Join(dir, "holdout.json")
	configPath := filepath.Join(dir, "config.yaml")
	writeTestDataset(t, holdoutPath, "holdout")
	require.NoError(t, os.WriteFile(configPath, []byte("config"), 0o600))

	require.Error(t, validateArtifactFingerprints(BaselineArtifact{}, holdoutPath, configPath))
}

func TestValidateArtifactUse(t *testing.T) {
	qualityFields := func(profile string) BaselineArtifact {
		llmCalls := 0
		if profile == "real" || profile == "recorded" {
			llmCalls = 1
		}
		return BaselineArtifact{
			BaselineQualityContract: baselineQualityContract,
			Profile:                 profile,
			Metrics:                 &goseval.EvalMetrics{TotalCases: 1, Traceability: 1},
			Results:                 []goseval.EvalResult{{CaseID: "case-1", TraceComplete: true, LLMCalls: llmCalls}},
		}
	}
	evalArtifact := qualityFields("eval")
	evalArtifact.DatasetRole = goseval.DatasetRoleRegression
	require.NoError(t, validateArtifactUse(evalArtifact, "gate", "eval"))
	realArtifact := qualityFields("real")
	realArtifact.DatasetRole = goseval.DatasetRoleHoldout
	realArtifact.EvidenceProvenance = "controlled_fault_injection_v1"
	realArtifact.DependencyState = map[string]string{
		"telemetry": "real", "telemetry_provenance": realArtifact.EvidenceProvenance,
	}
	require.NoError(t, validateArtifactUse(realArtifact, "compare", "real"))
	recordedArtifact := qualityFields("recorded")
	recordedArtifact.DatasetRole = goseval.DatasetRoleHoldout
	recordedArtifact.DependencyState = map[string]string{
		"telemetry": "recorded_blind", "evaluation_eligibility": "development_only",
	}
	require.NoError(t, validateArtifactUse(recordedArtifact, "compare", "recorded"))
	recordedArtifact.DatasetRole = goseval.DatasetRoleDevelopment
	require.NoError(t, validateArtifactUse(recordedArtifact, "compare", "recorded"))
	require.Error(t, validateArtifactUse(BaselineArtifact{Profile: "eval", DatasetRole: goseval.DatasetRoleHoldout}, "gate", "eval"))
	require.Error(t, validateArtifactUse(BaselineArtifact{Profile: "eval", DatasetRole: goseval.DatasetRoleRegression}, "compare", "real"))
	require.Error(t, validateArtifactUse(BaselineArtifact{
		Profile:         "real",
		DatasetRole:     goseval.DatasetRoleHoldout,
		DependencyState: map[string]string{"telemetry": "synthetic"},
	}, "compare", "real"))
}

func TestValidateBaselineRunQuality(t *testing.T) {
	artifact := BaselineArtifact{
		BaselineQualityContract: baselineQualityContract,
		Profile:                 "recorded",
		Metrics:                 &goseval.EvalMetrics{TotalCases: 2, Traceability: 1},
		Results: []goseval.EvalResult{
			{CaseID: "case-1", TraceComplete: true, LLMCalls: 1},
			{CaseID: "case-2", TraceComplete: true, LLMCalls: 2},
		},
	}
	require.NoError(t, validateBaselineRunQuality(artifact))

	artifact.Results[1].TraceComplete = false
	require.ErrorContains(t, validateBaselineRunQuality(artifact), "缺少完整 trace")
	artifact.Results[1].TraceComplete = true
	artifact.Results[1].LLMCalls = 0
	require.ErrorContains(t, validateBaselineRunQuality(artifact), "未发生真实模型调用")
	artifact.Profile = "eval"
	require.NoError(t, validateBaselineRunQuality(artifact))
}

func TestValidateBaselineRunQualityRejectsContaminatedMetrics(t *testing.T) {
	artifact := BaselineArtifact{
		BaselineQualityContract: baselineQualityContract,
		Profile:                 "recorded",
		Metrics:                 &goseval.EvalMetrics{TotalCases: 2, Traceability: 0.5},
		Results: []goseval.EvalResult{
			{CaseID: "case-1", TraceComplete: true, LLMCalls: 1},
			{CaseID: "case-2", TraceComplete: false},
		},
	}
	require.ErrorContains(t, validateBaselineRunQuality(artifact), "traceability 必须为 100%")

	artifact.BaselineQualityContract = ""
	require.ErrorContains(t, validateBaselineRunQuality(artifact), "quality contract 不匹配")
}

func TestValidateArtifactCases(t *testing.T) {
	dataset := &goseval.EvalDataset{Cases: []goseval.EvalCase{{ID: "case-1", GroundTruth: "root cause"}}}
	artifact := BaselineArtifact{
		Metrics: &goseval.EvalMetrics{TotalCases: 1},
		Results: []goseval.EvalResult{{CaseID: "case-1", GroundTruth: "root cause"}},
	}
	require.NoError(t, validateArtifactCases(artifact, dataset))

	artifact.Results[0].GroundTruth = "changed"
	require.Error(t, validateArtifactCases(artifact, dataset))
	artifact.Results = nil
	require.Error(t, validateArtifactCases(artifact, dataset))
}

func TestResolveDatasetPath(t *testing.T) {
	require.Equal(t, defaultRegressionDataset, resolveDatasetPath("gate", ""))
	require.Equal(t, defaultRegressionDataset, resolveDatasetPath("smoke", ""))
	require.Equal(t, defaultRegressionDataset, resolveDatasetPath("regression-baseline", ""))
	require.Equal(t, defaultHoldoutDataset, resolveDatasetPath("baseline", ""))
	require.Equal(t, "custom.json", resolveDatasetPath("gate", "custom.json"))
}

func TestLoadDatasetForModeRejectsHoldoutEval(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	holdoutPath := filepath.Join(repoRoot, defaultHoldoutDataset)
	regressionPath := filepath.Join(repoRoot, defaultRegressionDataset)

	_, err := loadDatasetForMode(holdoutPath, "gos", "eval")
	require.Error(t, err)
	_, err = loadDatasetForMode(regressionPath, "gate", "eval")
	require.NoError(t, err)
}

func TestFakeRAGQueryUsesDeterministicPriority(t *testing.T) {
	documents, err := fakeRAGQuery(context.Background(), "Kafka 消费者积压但 CPU 正常")
	require.NoError(t, err)
	require.Len(t, documents, 1)
	require.Contains(t, documents[0].Content, "Consumer lag")
}

func TestEvalGenerateContentReturnsStructuredEvidenceAssessments(t *testing.T) {
	content, err := evalGenerateContent(context.Background(), &belief.Frontier{
		NodeID: "hypothesis", Label: "CPU 饱和", Why: "CPU 持续高负载",
	}, belief.NewBeliefGraph(), []experts.RetrievalRecord{
		{Tool: "query_logs", Output: `{"success":true,"data":"CPU overload"}`},
		{Tool: "rag", Output: "CPU runbook"},
	}, map[string]string{"action": "analyze"})

	require.NoError(t, err)
	var proposal evalAnalysisProposal
	require.NoError(t, json.Unmarshal([]byte(content), &proposal))
	require.NotEmpty(t, proposal.Analysis)
	require.Len(t, proposal.Evidence, 2)
	for index, assessment := range proposal.Evidence {
		require.Equal(t, index, assessment.Index)
		require.Equal(t, string(experts.EvidenceRelationSupport), assessment.Relation)
		require.Greater(t, assessment.Strength, 0.0)
	}
}

func TestValidateRealDependencyConfigReportsAllMissingInputs(t *testing.T) {
	err := validateRealDependencyConfig(realDependencyConfig{
		MilvusAddress:    "localhost:19530",
		MilvusCollection: "opscaption_knowledge_v2",
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "chat_model.api_key")
	require.ErrorContains(t, err, "embedding_model.api_key")
	require.ErrorContains(t, err, "MCP_LOG_URL or MCP_LOG_HTTP_URL")
}

func TestValidateRealDependencyConfigAcceptsResolvedInputs(t *testing.T) {
	err := validateRealDependencyConfig(realDependencyConfig{
		ChatModel:        common.AIModelConfig{Provider: "deepseek", APIKey: "test-deepseek-key"},
		EmbeddingModel:   common.AIModelConfig{Provider: "doubao", APIKey: "test-ark-key"},
		MCPLogHTTPURL:    "http://log-service/tools/query_logs",
		MilvusAddress:    "localhost:19530",
		MilvusCollection: "opscaption_knowledge_v2",
	})

	require.NoError(t, err)
}

func TestValidateRealDependencyConfigRejectsPlaceholders(t *testing.T) {
	err := validateRealDependencyConfig(realDependencyConfig{
		ChatModel:        common.AIModelConfig{Provider: "deepseek", APIKey: "your-deepseek-api-key"},
		EmbeddingModel:   common.AIModelConfig{Provider: "doubao", APIKey: "replace-with-your-ark-key"},
		MCPLogURL:        "${MCP_LOG_URL}",
		MilvusAddress:    "localhost:19530",
		MilvusCollection: "opscaption_knowledge_v2",
	})

	require.Error(t, err)
	require.ErrorContains(t, err, "chat_model.api_key")
	require.ErrorContains(t, err, "embedding_model.api_key")
	require.ErrorContains(t, err, "MCP_LOG_URL or MCP_LOG_HTTP_URL")
}

func TestRequiresRealDependencies(t *testing.T) {
	require.True(t, requiresRealDependencies("baseline", "eval"))
	require.False(t, requiresRealDependencies("baseline", "recorded"))
	require.True(t, requiresRealDependencies("gos", "real"))
	require.True(t, requiresRealDependencies("compare", "real"))
	require.True(t, requiresRealDependencies("export-runs", "real"))
	require.False(t, requiresRealDependencies("gos", "eval"))
	require.False(t, requiresRealDependencies("gate", "eval"))
	require.False(t, requiresRealDependencies("preflight", "real"))
}

func TestRequiresVerifiedTelemetry(t *testing.T) {
	require.False(t, requiresVerifiedTelemetry("baseline", "eval"))
	require.False(t, requiresVerifiedTelemetry("baseline", "recorded"))
	require.True(t, requiresVerifiedTelemetry("compare", "real"))
	require.False(t, requiresVerifiedTelemetry("gos", "real"))
	require.False(t, requiresVerifiedTelemetry("export-runs", "real"))
	require.False(t, requiresVerifiedTelemetry("compare", "eval"))
}

func TestRecordedProfileIsRestrictedToCaseScopedEvaluationModes(t *testing.T) {
	for _, mode := range []string{"gos", "baseline", "compare"} {
		require.NoError(t, validateProfileForMode(mode, "recorded"))
	}
	require.Error(t, validateProfileForMode("export-runs", "recorded"))
	require.Error(t, validateProfileForMode("gate", "unknown"))
}

func TestValidateRecordedDependencyConfigOnlyRequiresRealChatModel(t *testing.T) {
	require.NoError(t, validateRecordedDependencyConfig(realDependencyConfig{
		ChatModel: common.AIModelConfig{Provider: "deepseek", APIKey: "test-key"},
	}))
	require.Error(t, validateRecordedDependencyConfig(realDependencyConfig{
		ChatModel: common.AIModelConfig{Provider: "deepseek", APIKey: "your-deepseek-key"},
	}))
}

func TestValidateRecordedCorpusChecksEveryCaseBeforeModelCalls(t *testing.T) {
	root := t.TempDir()
	datasetPath := filepath.Join(root, "holdout.json")
	writeTestDataset(t, datasetPath, "time-window-only")
	writeRecordedEvidenceCase(t, root, "holdout-test", "metric-a", "recorded_blind")

	require.NoError(t, validateRecordedCorpus(context.Background(), datasetPath, root, time.Second))
	require.NoError(t, os.Remove(filepath.Join(root, "docs_evidence_telemetry", "holdout-test.md")))
	require.ErrorContains(
		t,
		validateRecordedCorpus(context.Background(), datasetPath, root, time.Second),
		"holdout-test",
	)
}

func TestValidateRecordedArtifactFingerprintDetectsReplayChanges(t *testing.T) {
	root := t.TempDir()
	datasetPath := filepath.Join(root, "holdout.json")
	writeTestDataset(t, datasetPath, "time-window-only")
	writeRecordedEvidenceCase(t, root, "holdout-test", "metric-a", "recorded_blind")
	hash, err := recordedCorpusSHA256(context.Background(), datasetPath, root, time.Second)
	require.NoError(t, err)
	artifact := BaselineArtifact{
		EvidenceCorpusSHA256:  hash,
		EvidenceProvenance:    "recorded_blind",
		EvaluationEligibility: "development_only",
	}
	require.NoError(t, validateRecordedArtifactFingerprint(
		context.Background(), artifact, datasetPath, root, time.Second,
	))

	documentPath := filepath.Join(root, "docs_evidence_telemetry", "holdout-test.md")
	file, err := os.OpenFile(documentPath, os.O_APPEND|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = file.WriteString("\n- metric-b\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	require.ErrorContains(
		t,
		validateRecordedArtifactFingerprint(context.Background(), artifact, datasetPath, root, time.Second),
		"does not match",
	)
}

func TestBaselineDegradationPreservesAUsableReportAndFailurePhase(t *testing.T) {
	err := errors.New("no analysis conclusion found in event stream")
	require.Equal(
		t,
		"[DEGRADED] ## 诊断报告\n证据不足",
		degradedBaselinePrediction("## 诊断报告\n证据不足", err),
	)
	require.Equal(t, "[ERROR] no analysis conclusion found in event stream", degradedBaselinePrediction("", err))
	require.Equal(t, "report", baselineFailurePhase(err))
	require.Equal(t, "act", baselineFailurePhase(errors.New("executor tool failed")))
}

func TestTruncateSymptomKeepsUTF8Valid(t *testing.T) {
	got := truncateSymptom("诊断报告包含完整中文字符", 6)

	require.Equal(t, "诊断报告包含...", got)
	require.NotContains(t, got, "�")
}

func TestValidateVerifiedTelemetry(t *testing.T) {
	require.NoError(t, validateVerifiedTelemetry("real"))
	for _, profile := range []string{"", "unverified", "synthetic"} {
		err := validateVerifiedTelemetry(profile)
		require.ErrorContains(t, err, "verified real telemetry", profile)
	}
}

func TestValidateTelemetryProvenance(t *testing.T) {
	require.NoError(t, validateTelemetryProvenance("controlled_fault_injection_v1"))
	require.NoError(t, validateTelemetryProvenance("production_observed_v1"))
	for _, provenance := range []string{"", "unverified", "synthetic", "recorded_blind"} {
		require.Error(t, validateTelemetryProvenance(provenance), provenance)
	}
	require.Equal(t, "staging_controlled_only", realEvaluationEligibility("controlled_fault_injection_v1"))
	require.Equal(t, "production_candidate", realEvaluationEligibility("production_observed_v1"))
}

func TestProbeRealRAGRejectsRetrievalFailure(t *testing.T) {
	err := probeRealRAG(context.Background(), probeRetriever{err: errors.New("search failed")})

	require.ErrorContains(t, err, "search failed")
}

func TestProbeRealRAGRejectsEmptyOrBlankDocuments(t *testing.T) {
	err := probeRealRAG(context.Background(), probeRetriever{docs: []*schema.Document{nil, {Content: "  "}}})

	require.ErrorContains(t, err, "no usable documents")
}

func TestProbeRealRAGAcceptsUsableDocument(t *testing.T) {
	err := probeRealRAG(context.Background(), probeRetriever{docs: []*schema.Document{{Content: "CrashLoopBackOff runbook"}}})

	require.NoError(t, err)
}

func TestBuildGoSRunnerRealLoadsConfiguredFullChain(t *testing.T) {
	oldLoader := loadRealGoSConfig
	loadRealGoSConfig = func(context.Context) *gos_engine.Config {
		cfg := gos_engine.DefaultConfig()
		cfg.StructuredCognition.Enabled = true
		cfg.StateConversion.Enabled = true
		cfg.SessionMaxSteps = 7
		cfg.FSM.MaxSteps = 6
		cfg.CallTimeoutMs = 24000
		return cfg
	}
	t.Cleanup(func() { loadRealGoSConfig = oldLoader })

	runner, cfg, err := buildGoSRunner("real", "", 0)
	if err != nil {
		t.Fatalf("buildGoSRunner(real): %v", err)
	}
	if runner == nil || cfg == nil {
		t.Fatal("real runner and config must be initialized")
	}
	if !cfg.StructuredCognition.Enabled {
		t.Fatal("real runner must load structured cognition from effective config")
	}
	if !cfg.StateConversion.Enabled {
		t.Fatal("real runner must load state conversion from effective config")
	}
	require.Equal(t, 7, cfg.SessionMaxSteps)
	require.Equal(t, 6, cfg.FSM.MaxSteps)
	require.Equal(t, 24000, cfg.CallTimeoutMs)
}

func TestEvalGenerateContentPromotesMappedRootCauseIntoGraphRefinement(t *testing.T) {
	graph := belief.NewBeliefGraph()
	graph.StartSignalID = graph.AddSignal("数据库连接超时，连接池已满")
	frontier := &belief.Frontier{NodeID: graph.AddHypothesis("资源耗尽", 0.6, 1, "需要验证"), Label: "资源耗尽"}

	raw, err := evalGenerateContent(context.Background(), frontier, graph, []experts.RetrievalRecord{{
		Tool: "query_logs", Output: `{"success":true,"data":"Connection pool exhausted, slow queries increased"}`,
	}}, map[string]string{"action": "analyze"})
	require.NoError(t, err)
	var proposal evalAnalysisProposal
	require.NoError(t, json.Unmarshal([]byte(raw), &proposal))
	require.Equal(t, "数据库连接池耗尽", proposal.Analysis)
	require.Len(t, proposal.Refinements, 1)
	require.Equal(t, "数据库连接池耗尽", proposal.Refinements[0].Label)
	require.True(t, proposal.Refinements[0].Actionable)
}

func TestBuildGoSEngineEvalProfileExercisesStructuredRefinePath(t *testing.T) {
	_, cfg, err := buildGoSEngine(true, nil)
	require.NoError(t, err)
	require.True(t, cfg.StructuredCognition.Enabled)
	require.True(t, cfg.StateConversion.Enabled)
	require.NotNil(t, cfg.StructuredGenerate)
}
