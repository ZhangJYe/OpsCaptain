package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	judgeeval "SuperBizAgent/internal/ai/agent/eval"
	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	goseval "SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/models"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/rag"
	internalretriever "SuperBizAgent/internal/ai/retriever"
	aiopsservice "SuperBizAgent/internal/ai/service"
	inframv "SuperBizAgent/internal/infra/milvus"
	"SuperBizAgent/utility/common"

	einoretriever "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/components/tool"
	einoschema "github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type BaselineArtifact struct {
	Commit                  string                 `json:"commit"`
	CodeSHA256              string                 `json:"code_sha256"`
	CodeFingerprintScope    string                 `json:"code_fingerprint_scope"`
	EvidenceMetricContract  string                 `json:"evidence_metric_contract"`
	BaselineQualityContract string                 `json:"baseline_quality_contract"`
	Model                   string                 `json:"model"`
	ToolConfig              string                 `json:"tool_config"`
	Profile                 string                 `json:"profile"`
	PromptVersion           string                 `json:"prompt_version"`
	DependencyState         map[string]string      `json:"dependency_state"`
	DatasetSchemaVersion    string                 `json:"dataset_schema_version"`
	DatasetRole             goseval.DatasetRole    `json:"dataset_role"`
	DatasetPath             string                 `json:"dataset_path"`
	DatasetSHA256           string                 `json:"dataset_sha256"`
	ConfigPath              string                 `json:"config_path"`
	ConfigSHA256            string                 `json:"config_sha256"`
	ConfigFingerprintScope  string                 `json:"config_fingerprint_scope,omitempty"`
	ConfigSummary           map[string]interface{} `json:"config_summary"`
	EvidenceCorpusSHA256    string                 `json:"evidence_corpus_sha256,omitempty"`
	EvidenceProvenance      string                 `json:"evidence_provenance,omitempty"`
	EvaluationEligibility   string                 `json:"evaluation_eligibility,omitempty"`
	Timestamp               string                 `json:"timestamp"`
	Metrics                 *goseval.EvalMetrics   `json:"metrics"`
	Results                 []goseval.EvalResult   `json:"results"`
}

const evalConfigPath = "manifest/config/config.yaml"
const runtimeCodeFingerprintScope = "go-source+modules+prompts-v1"
const baselineCodeFingerprintScope = "plan-baseline+evaluator+recorded-replay-v1"
const gateCodeFingerprintScope = "gos-eval-runtime-v1"
const fullConfigFingerprintScope = "manifest-config-file-v1"
const evalConfigFingerprintScope = "gos-eval-effective-config-v1"
const evidenceMetricContract = "source-backed-signal-v2"
const baselineQualityContract = "complete-trace-profile-activity-v1"

const (
	defaultHoldoutDataset    = "internal/ai/agent/gos_engine/eval/testdata/holdout.json"
	defaultRegressionDataset = "internal/ai/agent/gos_engine/eval/testdata/smoke.json"
	defaultRecordedDataset   = "evals/aiops2025/recorded_holdout.json"
	realRAGProbeQuery        = "Kubernetes CrashLoopBackOff 排查"
)

var loadRealGoSConfig = aiopsservice.LoadAIOpsGOSConfig

type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[INFO] %s %v\n", msg, keysAndValues)
}
func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[ERROR] %s %v\n", msg, keysAndValues)
}

type realDependencyConfig struct {
	ChatModel        common.AIModelConfig
	ChatModelError   error
	EmbeddingModel   common.AIModelConfig
	EmbeddingError   error
	MCPLogURL        string
	MCPLogHTTPURL    string
	MilvusAddress    string
	MilvusCollection string
	TelemetryProfile string
	TelemetrySource  string
}

func loadRealDependencyConfig(ctx context.Context) realDependencyConfig {
	chatModel, chatModelErr := common.LoadChatModelConfig(ctx, common.ChatModelPrimary)
	embeddingModel, embeddingErr := common.LoadEmbeddingModelConfig(ctx)
	return realDependencyConfig{
		ChatModel:        chatModel,
		ChatModelError:   chatModelErr,
		EmbeddingModel:   embeddingModel,
		EmbeddingError:   embeddingErr,
		MCPLogURL:        resolveOptionalConfig(ctx, "mcp.log_url", "MCP_LOG_URL"),
		MCPLogHTTPURL:    resolveOptionalConfig(ctx, "mcp.log_http_url", "MCP_LOG_HTTP_URL"),
		MilvusAddress:    common.GetMilvusAddr(ctx),
		MilvusCollection: common.GetMilvusCollectionName(ctx),
		TelemetryProfile: loadTelemetryProfile(ctx),
		TelemetrySource:  loadTelemetryProvenance(ctx),
	}
}

func loadTelemetryProvenance(ctx context.Context) string {
	value, err := g.Cfg().Get(ctx, "aiops.gos.evaluation.telemetry_provenance")
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(value.String()))
}

func loadTelemetryProfile(ctx context.Context) string {
	value, err := g.Cfg().Get(ctx, "aiops.gos.evaluation.telemetry_profile")
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(value.String()))
}

func resolveOptionalConfig(ctx context.Context, configPath, envName string) string {
	if value, err := g.Cfg().Get(ctx, configPath); err == nil {
		if resolved, ok := common.ResolveOptionalEnv(value.String()); ok && !common.LooksLikePlaceholderSecret(resolved) {
			return resolved
		}
	}
	value := strings.TrimSpace(os.Getenv(envName))
	if common.LooksLikePlaceholderSecret(value) {
		return ""
	}
	return value
}

func validateRealDependencyConfig(cfg realDependencyConfig) error {
	var errs []error
	if cfg.ChatModelError != nil {
		errs = append(errs, fmt.Errorf("chat model config: %w", cfg.ChatModelError))
	} else if common.LooksLikePlaceholderSecret(cfg.ChatModel.APIKey) {
		errs = append(errs, errors.New("chat_model.api_key is empty or placeholder"))
	}
	if cfg.EmbeddingError != nil {
		errs = append(errs, fmt.Errorf("embedding model config: %w", cfg.EmbeddingError))
	} else if common.LooksLikePlaceholderSecret(cfg.EmbeddingModel.APIKey) {
		errs = append(errs, errors.New("embedding_model.api_key is empty or placeholder"))
	}
	if common.LooksLikePlaceholderSecret(cfg.MCPLogURL) && common.LooksLikePlaceholderSecret(cfg.MCPLogHTTPURL) {
		errs = append(errs, errors.New("MCP_LOG_URL or MCP_LOG_HTTP_URL is required for real evaluation"))
	}
	if strings.TrimSpace(cfg.MilvusAddress) == "" || common.IsEnvReference(cfg.MilvusAddress) {
		errs = append(errs, errors.New("Milvus address is empty or unresolved"))
	}
	if strings.TrimSpace(cfg.MilvusCollection) == "" || common.IsEnvReference(cfg.MilvusCollection) {
		errs = append(errs, errors.New("Milvus collection is empty or unresolved"))
	}
	return errors.Join(errs...)
}

func printRealDependencyPreflight(cfg realDependencyConfig) error {
	fmt.Println("=== GoS 真实依赖静态预检 ===")
	printDependencyStatus(modelDependencyLabel("LLM", cfg.ChatModel.Provider), cfg.ChatModelError == nil)
	printDependencyStatus(modelDependencyLabel("Embedding", cfg.EmbeddingModel.Provider), cfg.EmbeddingError == nil)
	printDependencyStatus("Logs / MCP endpoint", !common.LooksLikePlaceholderSecret(cfg.MCPLogURL) || !common.LooksLikePlaceholderSecret(cfg.MCPLogHTTPURL))
	printDependencyStatus("RAG / Milvus config", strings.TrimSpace(cfg.MilvusAddress) != "" && strings.TrimSpace(cfg.MilvusCollection) != "")
	printDependencyStatus("Verified real telemetry", cfg.TelemetryProfile == "real")
	fmt.Printf("  %-30s %s\n", "Telemetry profile", telemetryProfileLabel(cfg.TelemetryProfile))
	fmt.Printf("  %-30s %s\n", "Telemetry provenance", telemetryProfileLabel(cfg.TelemetrySource))
	fmt.Println("说明: 本模式不发起网络请求；LLM、MCP、Milvus 和真实 RAG 连通性均未验证。")
	if err := errors.Join(
		validateRealDependencyConfig(cfg),
		validateVerifiedTelemetry(cfg.TelemetryProfile),
		validateTelemetryProvenance(cfg.TelemetrySource),
	); err != nil {
		return fmt.Errorf("真实依赖静态配置未就绪: %w", err)
	}
	return nil
}

func telemetryProfileLabel(profile string) string {
	if strings.TrimSpace(profile) == "" {
		return "unconfigured"
	}
	return profile
}

func modelDependencyLabel(kind, provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		provider = "unresolved"
	}
	return fmt.Sprintf("%s / %s", kind, provider)
}

func printDependencyStatus(name string, ready bool) {
	status := "MISSING"
	if ready {
		status = "CONFIGURED"
	}
	fmt.Printf("  %-30s %s\n", name, status)
}

func requiresRealDependencies(mode, profile string) bool {
	switch mode {
	case "baseline":
		return profile != "recorded"
	case "gos", "compare", "export-runs":
		return profile == "real"
	default:
		return false
	}
}

func requiresVerifiedTelemetry(mode, profile string) bool {
	return (mode == "baseline" || mode == "compare") && profile == "real"
}

func requiresRecordedReplay(mode, profile string) bool {
	if profile != "recorded" {
		return false
	}
	switch mode {
	case "gos", "baseline", "compare":
		return true
	default:
		return false
	}
}

func validateRecordedDependencyConfig(cfg realDependencyConfig) error {
	if cfg.ChatModelError != nil {
		return fmt.Errorf("chat model config: %w", cfg.ChatModelError)
	}
	if common.LooksLikePlaceholderSecret(cfg.ChatModel.APIKey) {
		return errors.New("chat_model.api_key is empty or placeholder")
	}
	return nil
}

func validateRecordedReplayConfig(root string, timeout time.Duration) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("--recorded-evidence-root is required for recorded profile")
	}
	if timeout <= 0 {
		return errors.New("--recorded-timeout-ms must be positive")
	}
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("recorded evidence root: %w", err)
	}
	if !info.IsDir() {
		return errors.New("recorded evidence root must be a directory")
	}
	return nil
}

func validateRecordedCorpus(ctx context.Context, datasetPath, root string, timeout time.Duration) error {
	dataset, err := goseval.LoadDataset(datasetPath)
	if err != nil {
		return fmt.Errorf("load recorded dataset: %w", err)
	}
	for _, evalCase := range dataset.Cases {
		source, err := newRecordedEvidenceSource(root, evalCase.ID, timeout)
		if err != nil {
			return err
		}
		if _, err := source.Load(ctx); err != nil {
			return fmt.Errorf("validate recorded evidence for case %q: %w", evalCase.ID, err)
		}
	}
	return nil
}

func recordedCorpusSHA256(ctx context.Context, datasetPath, root string, timeout time.Duration) (string, error) {
	dataset, err := goseval.LoadDataset(datasetPath)
	if err != nil {
		return "", fmt.Errorf("load recorded dataset: %w", err)
	}
	hash := sha256.New()
	for _, evalCase := range dataset.Cases {
		source, err := newRecordedEvidenceSource(root, evalCase.ID, timeout)
		if err != nil {
			return "", err
		}
		if _, err := source.Load(ctx); err != nil {
			return "", fmt.Errorf("validate recorded evidence for case %q: %w", evalCase.ID, err)
		}
		docsDir := filepath.Join(source.root, "docs_evidence_telemetry")
		for _, suffix := range []string{".metadata.json", ".md"} {
			data, err := readBoundedFile(ctx, filepath.Join(docsDir, evalCase.ID+suffix), recordedEvidenceMaxContentSize)
			if err != nil {
				return "", err
			}
			_, _ = io.WriteString(hash, evalCase.ID+suffix+"\x00")
			_, _ = hash.Write(data)
			_, _ = hash.Write([]byte{0})
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func validateRecordedArtifactFingerprint(ctx context.Context, artifact BaselineArtifact, datasetPath, root string, timeout time.Duration) error {
	if artifact.EvidenceProvenance != "recorded_blind" || artifact.EvaluationEligibility != "development_only" {
		return errors.New("recorded baseline artifact has invalid evidence provenance")
	}
	if artifact.EvidenceCorpusSHA256 == "" {
		return errors.New("recorded baseline artifact is missing evidence_corpus_sha256")
	}
	current, err := recordedCorpusSHA256(ctx, datasetPath, root, timeout)
	if err != nil {
		return err
	}
	if current != artifact.EvidenceCorpusSHA256 {
		return errors.New("recorded baseline artifact evidence corpus does not match current replay files")
	}
	return nil
}

func validateProfileForMode(mode, profile string) error {
	switch profile {
	case "real", "eval":
		return nil
	case "recorded":
		if requiresRecordedReplay(mode, profile) {
			return nil
		}
		return fmt.Errorf("recorded profile is unsupported for mode %q", mode)
	default:
		return fmt.Errorf("unsupported GoS profile %q", profile)
	}
}

func evaluationEligibility(profile string) string {
	if profile == "recorded" || profile == "eval" {
		return "development_only"
	}
	return "requires_verified_real_dependencies"
}

func validateVerifiedTelemetry(profile string) error {
	if strings.EqualFold(strings.TrimSpace(profile), "real") {
		return nil
	}
	return fmt.Errorf("verified real telemetry is required, current profile is %q", telemetryProfileLabel(profile))
}

func validateTelemetryProvenance(provenance string) error {
	switch strings.ToLower(strings.TrimSpace(provenance)) {
	case "controlled_fault_injection_v1", "production_observed_v1":
		return nil
	default:
		return fmt.Errorf("verified telemetry provenance is required, current provenance is %q", telemetryProfileLabel(provenance))
	}
}

func realEvaluationEligibility(provenance string) string {
	if strings.EqualFold(strings.TrimSpace(provenance), "production_observed_v1") {
		return "production_candidate"
	}
	return "staging_controlled_only"
}

func bootstrapRealRAG(ctx context.Context) error {
	timeout := rag.DurationFromConfig(ctx, 30*time.Second, "aiops.gos.evaluation.rag_bootstrap_timeout_ms")
	bootstrapCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fmt.Println("真实 RAG 初始化: 检查既有 Milvus 集合（只读）")
	milvusClient, err := inframv.OpenExistingMilvusClient(bootstrapCtx)
	if err != nil {
		return fmt.Errorf("initialize Milvus for real RAG: %w", err)
	}
	fmt.Println("真实 RAG 初始化: 创建检索器")
	factory := internalretriever.NewMilvusRetrieverWithClient(milvusClient)
	rtr, err := factory(bootstrapCtx)
	if err != nil {
		_ = milvusClient.Close()
		return fmt.Errorf("initialize retriever for real RAG: %w", err)
	}
	fmt.Println("真实 RAG 初始化: 执行 Embedding + 向量检索探针")
	if err := probeRealRAG(bootstrapCtx, rtr); err != nil {
		_ = milvusClient.Close()
		return fmt.Errorf("validate real RAG retrieval: %w", err)
	}
	rag.NewRetrieverFunc = factory
	rag.ResetSharedPool()
	fmt.Println("真实 RAG 初始化: 通过")
	return nil
}

func probeRealRAG(ctx context.Context, rtr einoretriever.Retriever) error {
	timeout := rag.DurationFromConfig(ctx, 5*time.Second, "context.docs_query_timeout_ms")
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	docs, err := rtr.Retrieve(probeCtx, realRAGProbeQuery)
	if err != nil {
		return err
	}
	for _, doc := range docs {
		if doc != nil && strings.TrimSpace(doc.Content) != "" {
			return nil
		}
	}
	return errors.New("retrieval returned no usable documents")
}

type keywordResponse struct {
	keyword  string
	response string
}

type evalAnalysisProposal struct {
	Analysis    string                         `json:"analysis"`
	Confidence  float64                        `json:"confidence"`
	Evidence    []evalEvidenceAssessment       `json:"evidence"`
	Refinements []experts.HypothesisRefinement `json:"refinements,omitempty"`
}

type evalEvidenceAssessment struct {
	Index    int     `json:"index"`
	Relation string  `json:"relation"`
	Strength float64 `json:"strength"`
}

type fakeLogTool struct {
	ordered []keywordResponse
}

func newFakeLogTool() *fakeLogTool {
	return &fakeLogTool{
		ordered: []keywordResponse{
			{"Kafka", `{"success": true, "data": "Consumer lag increased, messages堆积"}`},
			{"消息堆积", `{"success": true, "data": "Consumer lag increased, messages堆积"}`},
			{"消费者", `{"success": true, "data": "Consumer lag increased, CPU low"}`},
			{"数据库", `{"success": true, "data": "Connection pool exhausted, slow queries increased"}`},
			{"连接池", `{"success": true, "data": "Connection pool exhausted, slow queries increased"}`},
			{"跨区域", `{"success": true, "data": "Cross-region latency high, packet loss 5%"}`},
			{"网络", `{"success": true, "data": "Cross-region latency high, packet loss 5%"}`},
			{"缓存", `{"success": true, "data": "Cache hit rate decreased, keys expired"}`},
			{"Redis", `{"success": true, "data": "Cache hit rate decreased, keys expired"}`},
			{"CPU", `{"success": true, "data": "CPU usage 95%, memory 80%"}`},
		},
	}
}

func (f *fakeLogTool) Info(ctx context.Context) (*einoschema.ToolInfo, error) {
	return &einoschema.ToolInfo{Name: "query_logs", Desc: "Query application logs"}, nil
}

func (f *fakeLogTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	for _, pair := range f.ordered {
		if contains(args, pair.keyword) {
			return pair.response, nil
		}
	}
	return `{"success": true, "data": "No relevant logs found"}`, nil
}

type fakeInternalDocsTool struct {
	ordered []keywordResponse
}

func newFakeInternalDocsTool() *fakeInternalDocsTool {
	return &fakeInternalDocsTool{
		ordered: []keywordResponse{
			{"Kafka", `{"success": true, "data": "Kafka consumer: check lag, partition count, consumer group"}`},
			{"消息堆积", `{"success": true, "data": "Kafka consumer: check lag, partition count, consumer group"}`},
			{"消费者", `{"success": true, "data": "Kafka consumer: check lag, partition count, consumer group"}`},
			{"数据库", `{"success": true, "data": "DB connection pool: check max_connections, wait_timeout"}`},
			{"连接池", `{"success": true, "data": "DB connection pool: check max_connections, wait_timeout"}`},
			{"跨区域", `{"success": true, "data": "Network latency: check traceroute, mtr, tcpdump"}`},
			{"网络", `{"success": true, "data": "Network latency: check traceroute, mtr, tcpdump"}`},
			{"缓存", `{"success": true, "data": "Redis cache: check hit rate, eviction policy"}`},
			{"Redis", `{"success": true, "data": "Redis cache: check hit rate, eviction policy"}`},
			{"CPU", `{"success": true, "data": "CPU overload: check top, vmstat, sar"}`},
		},
	}
}

func (f *fakeInternalDocsTool) Info(ctx context.Context) (*einoschema.ToolInfo, error) {
	return &einoschema.ToolInfo{Name: "query_internal_docs", Desc: "Query internal docs"}, nil
}

func (f *fakeInternalDocsTool) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	for _, pair := range f.ordered {
		if contains(args, pair.keyword) {
			return pair.response, nil
		}
	}
	return `{"success": true, "data": "No relevant docs found"}`, nil
}

func fakeRAGQuery(ctx context.Context, query string) ([]*einoschema.Document, error) {
	ragData := []keywordResponse{
		{"Kafka", "Runbook: Consumer lag → increase consumers, check partition count"},
		{"消息堆积", "Runbook: Consumer lag → increase consumers, check partition count"},
		{"消费者", "Runbook: Consumer lag → increase consumers, check partition count"},
		{"数据库", "Runbook: Connection pool exhaustion → increase max_connections, add read replicas"},
		{"连接池", "Runbook: Connection pool exhaustion → increase max_connections, add read replicas"},
		{"跨区域", "Runbook: Cross-region latency → check VPN/link health, switch to local endpoint"},
		{"网络", "Runbook: Cross-region latency → check VPN/link health, switch to local endpoint"},
		{"缓存", "Runbook: Cache miss spike → check TTL, warm cache, increase memory"},
		{"Redis", "Runbook: Cache miss spike → check TTL, warm cache, increase memory"},
		{"CPU", "Runbook: CPU > 90% → check runaway processes, scale replicas"},
	}
	for _, pair := range ragData {
		if contains(query, pair.keyword) {
			return []*einoschema.Document{{Content: pair.response}}, nil
		}
	}
	return []*einoschema.Document{{Content: "Generic troubleshooting: check logs, metrics, recent deployments"}}, nil
}

func evalGenerateContent(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph, history []experts.RetrievalRecord, decision map[string]string) (string, error) {
	symptom := ""
	if graph != nil && graph.StartSignalID != "" {
		if node, ok := graph.Nodes[graph.StartSignalID]; ok {
			symptom = node.Label
		}
	}

	switch decision["action"] {
	case "tool_call":
		return fmt.Sprintf("查询 %s %s 相关日志", frontier.Label, symptom), nil
	case "retrieve":
		return fmt.Sprintf("%s %s", frontier.Label, symptom), nil
	case "analyze":
		var toolData []string
		for _, h := range history {
			if h.Tool == "query_logs" || h.Tool == "query_internal_docs" {
				d := extractDataFieldEval(h.Output)
				if d != "" {
					toolData = append(toolData, d)
				}
			}
		}
		conclusion := mapToolOutputToConclusion(strings.Join(toolData, "\n"))
		mappedConclusion := conclusion
		if conclusion == "" {
			conclusion = fmt.Sprintf("针对假设「%s」的分析：%s", frontier.Label, frontier.Why)
		}
		proposal := evalAnalysisProposal{
			Analysis:   conclusion,
			Confidence: 0.8,
			Evidence:   make([]evalEvidenceAssessment, 0, len(history)),
		}
		for index := range history {
			proposal.Evidence = append(proposal.Evidence, evalEvidenceAssessment{
				Index: index, Relation: string(experts.EvidenceRelationSupport), Strength: 0.8,
			})
		}
		if mappedConclusion != "" && !strings.EqualFold(strings.TrimSpace(mappedConclusion), strings.TrimSpace(frontier.Label)) {
			proposal.Refinements = []experts.HypothesisRefinement{{
				Label:      mappedConclusion,
				Score:      0.8,
				Why:        "deterministic eval evidence supports a more actionable root cause",
				Actionable: true,
			}}
		}
		data, err := json.Marshal(proposal)
		return string(data), err
	}
	return "", fmt.Errorf("unknown action: %s", decision["action"])
}

func extractDataFieldEval(jsonStr string) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return ""
	}
	if data, ok := parsed["data"].(string); ok {
		return data
	}
	return ""
}

func mapToolOutputToConclusion(toolData string) string {
	type mapping struct {
		keywords   []string
		conclusion string
	}
	mappings := []mapping{
		{[]string{"Consumer lag", "messages堆积", "consumer group"}, "Kafka 消费者处理能力不足"},
		{[]string{"Connection pool", "slow queries", "max_connections"}, "数据库连接池耗尽"},
		{[]string{"Cross-region latency", "packet loss", "VPN"}, "网络链路问题"},
		{[]string{"Cache hit rate", "keys expired", "eviction"}, "缓存失效导致后端压力"},
		{[]string{"CPU usage", "CPU overload", "vmstat"}, "CPU 资源耗尽导致服务超时"},
	}
	lower := strings.ToLower(toolData)
	for _, m := range mappings {
		for _, kw := range m.keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return m.conclusion
			}
		}
	}
	return ""
}

type smokeBaselineRunner struct {
	logTool *fakeLogTool
	docTool *fakeInternalDocsTool
}

func newSmokeBaselineRunner() *smokeBaselineRunner {
	return &smokeBaselineRunner{
		logTool: newFakeLogTool(),
		docTool: newFakeInternalDocsTool(),
	}
}

func (b *smokeBaselineRunner) runCase(ctx context.Context, c goseval.EvalCase) *goseval.EvalResult {
	start := time.Now()
	llmCalls := 0
	evidence := make([]protocol.EvidenceItem, 0, 2)

	for _, toolName := range []string{"query_logs", "query_internal_docs"} {
		var output string
		var err error
		switch toolName {
		case "query_logs":
			output, err = b.logTool.InvokableRun(ctx, c.Symptom)
		case "query_internal_docs":
			output, err = b.docTool.InvokableRun(ctx, c.Symptom)
		}
		llmCalls++
		if err == nil {
			evidence = append(evidence, protocol.EvidenceItem{
				SourceType: toolName,
				SourceID:   c.ID + "-" + toolName,
				Title:      toolName,
				Snippet:    output,
			})
		}
	}
	llmCalls++

	prediction := b.analyzeSymptom(c.Symptom)
	matched := goseval.MatchPrediction(prediction, c.GroundTruth, c.ExpectedKeywords)
	relevantEvidence, expectedEvidence, coveredEvidence := goseval.EvaluateEvidence(evidence, c.ExpectedEvidenceKeywords)

	status := string(protocol.ResultStatusSucceeded)
	statusMatched := c.ExpectedStatus == "" || c.ExpectedStatus == status
	failurePhase := ""
	if !matched || c.RequireRefine || c.RequireBacktrack {
		failurePhase = "report"
	}
	failurePhaseMatched := c.ExpectedFailurePhase == "" || c.ExpectedFailurePhase == failurePhase
	return &goseval.EvalResult{
		CaseID:            c.ID,
		Scenario:          c.Scenario,
		Symptom:           c.Symptom,
		GroundTruth:       c.GroundTruth,
		Prediction:        prediction,
		Status:            status,
		ExpectedStatus:    c.ExpectedStatus,
		StatusMatched:     statusMatched,
		Latency:           time.Since(start),
		LLMCalls:          llmCalls,
		ToolCalls:         2,
		EvidenceCount:     len(evidence),
		RelevantEvidence:  relevantEvidence,
		ExpectedEvidence:  expectedEvidence,
		CoveredEvidence:   coveredEvidence,
		Matched:           matched,
		TraceComplete:     true,
		GraphValid:        true,
		BacktrackRequired: c.RequireBacktrack,
		PrematureStop: goseval.IsPrematureStop(
			status,
			statusMatched,
			c.RequireRefine,
			false,
			c.RequireBacktrack,
			false,
		),
		FailurePhase:         failurePhase,
		ExpectedFailurePhase: c.ExpectedFailurePhase,
		FailurePhaseMatched:  failurePhaseMatched,
		ContractMatched:      statusMatched && failurePhaseMatched && !c.RequireRefine && !c.RequireBacktrack,
	}
}

func (b *smokeBaselineRunner) analyzeSymptom(symptom string) string {
	if contains(symptom, "Kafka") || contains(symptom, "消息堆积") || contains(symptom, "消费者") {
		return "Kafka 消费者处理能力不足"
	}
	if contains(symptom, "连接池") || contains(symptom, "数据库") || contains(symptom, "慢查询") {
		return "数据库连接池耗尽"
	}
	if contains(symptom, "跨区域") || contains(symptom, "网络延迟") || contains(symptom, "packet loss") {
		return "网络链路问题"
	}
	if contains(symptom, "缓存") || contains(symptom, "Redis") || contains(symptom, "命中率") {
		return "缓存失效导致后端压力"
	}
	if contains(symptom, "CPU") || contains(symptom, "95%") {
		return "CPU 资源耗尽导致服务超时"
	}
	return "需要进一步诊断"
}

func contains(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func codeContentSHA256() (string, error) {
	return gitContentSHA256(":(glob)**/*.go", "go.mod", "go.sum", ":(glob)prompts/**")
}

func baselineCodeContentSHA256() (string, error) {
	return gitContentSHA256(
		":(glob)cmd/gos_eval/*.go",
		":(glob)internal/ai/agent/plan_execute_replan/*.go",
		":(glob)internal/ai/agent/gos_engine/eval/*.go",
		":(glob)internal/ai/models/*.go",
		":(glob)utility/common/*.go",
		"go.mod", "go.sum", "prompts/plan_engine.md",
	)
}

func gateCodeContentSHA256() (string, error) {
	return gitContentSHA256(
		":(glob)cmd/gos_eval/*.go",
		":(glob)internal/ai/agent/gos_engine/*.go",
		":(glob)internal/ai/agent/gos_engine/eval/*.go",
		":(glob)internal/ai/agent/experts/*.go",
		":(glob)internal/ai/belief/*.go",
		":(glob)internal/ai/protocol/*.go",
		"go.mod", "go.sum",
		":(exclude,glob)**/*_test.go",
	)
}

func evalConfigSummary(cfg *gos_engine.Config) map[string]interface{} {
	return map[string]interface{}{
		"enabled":              cfg.Enabled,
		"model_path":           cfg.ModelPath,
		"temperature":          cfg.Temperature,
		"max_tokens":           cfg.MaxTokens,
		"evidence_max_chars":   cfg.EvidenceMaxChars,
		"session_max_steps":    cfg.SessionMaxSteps,
		"max_retrieval_steps":  cfg.MaxRetrievalSteps,
		"call_timeout_ms":      cfg.CallTimeoutMs,
		"fsm":                  cfg.FSM,
		"confidence":           cfg.Confidence,
		"graph":                cfg.Graph,
		"state_conversion":     cfg.StateConversion,
		"structured_cognition": cfg.StructuredCognition,
		"execution":            cfg.Execution,
		"report":               cfg.Report,
		"experts":              cfg.Experts,
		"head_agent":           cfg.HeadAgent,
	}
}

func configSummarySHA256(summary map[string]interface{}) (string, error) {
	data, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash[:]), nil
}

func gitContentSHA256(pathspecs ...string) (string, error) {
	rootOutput, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("resolve git root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(rootOutput))
	if repoRoot == "" {
		return "", fmt.Errorf("git root is empty")
	}

	args := append([]string{"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"}, pathspecs...)
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("list code content: %w", err)
	}
	paths := strings.Split(string(output), "\x00")
	if len(paths) > 0 && paths[len(paths)-1] == "" {
		paths = paths[:len(paths)-1]
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("code fingerprint scope is empty")
	}
	return contentFilesSHA256(repoRoot, paths)
}

func contentFilesSHA256(root string, paths []string) (string, error) {
	sortedPaths := append([]string(nil), paths...)
	sort.Strings(sortedPaths)
	hash := sha256.New()
	for _, name := range sortedPaths {
		cleanName := filepath.Clean(name)
		if filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("code fingerprint path escapes root: %q", name)
		}
		path := filepath.Join(root, cleanName)
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("stat code fingerprint file %q: %w", cleanName, err)
		}
		var content []byte
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return "", fmt.Errorf("read code fingerprint symlink %q: %w", cleanName, err)
			}
			content = []byte("symlink:" + target)
		} else {
			if !info.Mode().IsRegular() {
				return "", fmt.Errorf("code fingerprint path is not a regular file: %q", cleanName)
			}
			content, err = os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("read code fingerprint file %q: %w", cleanName, err)
			}
		}
		normalizedName := filepath.ToSlash(cleanName)
		_, _ = fmt.Fprintf(hash, "%d:%s%d:", len(normalizedName), normalizedName, len(content))
		_, _ = hash.Write(content)
		_, _ = hash.Write([]byte{'\n'})
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func validateArtifactFingerprints(artifact BaselineArtifact, datasetPath, configPath string) error {
	dataset, err := goseval.LoadDataset(datasetPath)
	if err != nil {
		return fmt.Errorf("加载当前 dataset: %w", err)
	}
	currentDatasetHash, err := fileSHA256(datasetPath)
	if err != nil {
		return fmt.Errorf("计算当前 dataset SHA256: %w", err)
	}
	if artifact.DatasetSchemaVersion != goseval.DatasetSchemaVersion || artifact.DatasetRole != dataset.Role {
		return fmt.Errorf("baseline artifact dataset schema 或 role 不匹配")
	}
	if artifact.DatasetSHA256 == "" {
		return fmt.Errorf("baseline artifact 缺少 dataset_sha256，请重新采集 baseline")
	}
	if strings.TrimSpace(artifact.PromptVersion) == "" || len(artifact.DependencyState) == 0 {
		return fmt.Errorf("baseline artifact 缺少 prompt_version 或 dependency_state，请重新采集 baseline")
	}
	if artifact.DatasetSHA256 != currentDatasetHash {
		return fmt.Errorf("baseline artifact 与当前 dataset 内容不一致")
	}

	if artifact.ConfigSHA256 == "" {
		return fmt.Errorf("baseline artifact 缺少 config_sha256，请重新采集 baseline")
	}
	if artifact.Profile == "eval" {
		if artifact.ConfigFingerprintScope != evalConfigFingerprintScope {
			return fmt.Errorf("baseline artifact 缺少当前版本 eval 配置指纹，请重新采集 baseline")
		}
		cfg := gos_engine.DefaultConfig()
		applyDeterministicEvalConfig(cfg)
		currentConfigHash, err := configSummarySHA256(evalConfigSummary(cfg))
		if err != nil {
			return fmt.Errorf("计算当前 eval 配置指纹: %w", err)
		}
		if artifact.ConfigSHA256 != currentConfigHash {
			return fmt.Errorf("baseline artifact 与当前 eval 有效配置不一致")
		}
	} else {
		if artifact.ConfigFingerprintScope != "" && artifact.ConfigFingerprintScope != fullConfigFingerprintScope {
			return fmt.Errorf("baseline artifact 配置指纹范围不匹配")
		}
		currentConfigHash, err := fileSHA256(configPath)
		if err != nil {
			return fmt.Errorf("计算当前配置 SHA256: %w", err)
		}
		if artifact.ConfigSHA256 != currentConfigHash {
			return fmt.Errorf("baseline artifact 与当前配置内容不一致")
		}
	}
	expectedCodeScope := baselineCodeFingerprintScope
	currentCodeHash, err := baselineCodeContentSHA256()
	if artifact.Profile == "eval" {
		expectedCodeScope = gateCodeFingerprintScope
		currentCodeHash, err = gateCodeContentSHA256()
	}
	if artifact.CodeFingerprintScope != expectedCodeScope || artifact.CodeSHA256 == "" {
		return fmt.Errorf("baseline artifact 缺少当前版本代码内容指纹，请重新采集 baseline")
	}
	if err != nil {
		return fmt.Errorf("计算当前代码内容指纹: %w", err)
	}
	if artifact.CodeSHA256 != currentCodeHash {
		return fmt.Errorf("baseline artifact 与当前代码内容不一致")
	}
	if artifact.EvidenceMetricContract != evidenceMetricContract {
		return fmt.Errorf("baseline artifact evidence metric contract 不匹配，请重新采集 baseline")
	}
	return nil
}

func validateArtifactUse(artifact BaselineArtifact, mode, profile string) error {
	switch mode {
	case "gate":
		if artifact.Profile != "eval" || artifact.DatasetRole != goseval.DatasetRoleRegression {
			return fmt.Errorf("gate requires eval regression artifact")
		}
	case "compare":
		if artifact.Profile != profile {
			return fmt.Errorf("compare requires %s artifact", profile)
		}
		if profile == "real" && artifact.DatasetRole != goseval.DatasetRoleHoldout {
			return fmt.Errorf("real compare requires holdout artifact")
		}
		if profile == "recorded" && artifact.DatasetRole != goseval.DatasetRoleDevelopment && artifact.DatasetRole != goseval.DatasetRoleHoldout {
			return fmt.Errorf("recorded compare requires development or holdout artifact")
		}
		if profile == "real" && artifact.DependencyState["telemetry"] != "real" {
			return fmt.Errorf("compare requires baseline artifact with verified real telemetry")
		}
		if profile == "real" && (artifact.EvidenceProvenance == "" || artifact.DependencyState["telemetry_provenance"] != artifact.EvidenceProvenance) {
			return fmt.Errorf("compare requires baseline artifact with verified telemetry provenance")
		}
		if profile == "real" {
			if err := validateTelemetryProvenance(artifact.EvidenceProvenance); err != nil {
				return err
			}
		}
		if profile == "recorded" && (artifact.DependencyState["telemetry"] != "recorded_blind" || artifact.DependencyState["evaluation_eligibility"] != "development_only") {
			return fmt.Errorf("recorded compare requires a development_only recorded_blind baseline artifact")
		}
	default:
		return fmt.Errorf("unsupported artifact mode %q", mode)
	}
	return validateBaselineRunQuality(artifact)
}

func validateBaselineRunQuality(artifact BaselineArtifact) error {
	if artifact.BaselineQualityContract != baselineQualityContract {
		return fmt.Errorf("baseline artifact quality contract 不匹配，请重新采集 baseline")
	}
	if artifact.Metrics == nil || artifact.Metrics.TotalCases == 0 || len(artifact.Results) == 0 {
		return fmt.Errorf("baseline artifact 缺少可验证的运行结果")
	}
	if artifact.Metrics.TotalCases != len(artifact.Results) {
		return fmt.Errorf("baseline artifact metrics 与 results 数量不一致")
	}
	if artifact.Metrics.Traceability != 1 {
		return fmt.Errorf("baseline artifact traceability 必须为 100%%，实际为 %.2f%%", artifact.Metrics.Traceability*100)
	}
	for index, result := range artifact.Results {
		if !result.TraceComplete {
			return fmt.Errorf("baseline artifact case %q (index %d) 缺少完整 trace", result.CaseID, index)
		}
		if (artifact.Profile == "real" || artifact.Profile == "recorded") && result.LLMCalls <= 0 {
			return fmt.Errorf("baseline artifact case %q (index %d) 未发生真实模型调用", result.CaseID, index)
		}
	}
	return nil
}

func validateArtifactCases(artifact BaselineArtifact, dataset *goseval.EvalDataset) error {
	if dataset == nil {
		return fmt.Errorf("dataset is required")
	}
	if artifact.Metrics == nil {
		return fmt.Errorf("baseline artifact 缺少 metrics")
	}
	if artifact.Metrics.TotalCases != len(dataset.Cases) || len(artifact.Results) != len(dataset.Cases) {
		return fmt.Errorf("baseline artifact case count 与 dataset 不匹配")
	}
	for index, evalCase := range dataset.Cases {
		result := artifact.Results[index]
		if result.CaseID != evalCase.ID || result.GroundTruth != evalCase.GroundTruth {
			return fmt.Errorf("baseline artifact case 与 dataset 不匹配 (index %d)", index)
		}
	}
	return nil
}

func loadDatasetForMode(path, mode, profile string) (*goseval.EvalDataset, error) {
	dataset, err := goseval.LoadDataset(path)
	if err != nil {
		return nil, err
	}
	if err := goseval.ValidateModeDataset(mode, profile, dataset); err != nil {
		return nil, err
	}
	return dataset, nil
}

func resolveDatasetPath(mode, requested string) string {
	if strings.TrimSpace(requested) != "" {
		return requested
	}
	switch mode {
	case "gate", "smoke", "regression-baseline":
		return defaultRegressionDataset
	default:
		return defaultHoldoutDataset
	}
}

func printMetrics(label string, m *goseval.EvalMetrics) {
	fmt.Printf("\n--- %s ---\n", label)
	fmt.Printf("  总用例: %d\n", m.TotalCases)
	fmt.Printf("  成功: %d | 降级: %d | 失败: %d\n", m.Succeeded, m.Degraded, m.Failed)
	fmt.Printf("  准确率: %.2f%%\n", m.Accuracy*100)
	fmt.Printf("  根因准确率: %.2f%%\n", m.RootCauseAccuracy*100)
	fmt.Printf("  证据精确率: %.2f%%\n", m.EvidencePrecision*100)
	fmt.Printf("  证据覆盖率: %.2f%%\n", m.EvidenceCoverage*100)
	fmt.Printf("  回溯成功率: %.2f%% | 过早停止率: %.2f%% | 图有效率: %.2f%%\n", m.BacktrackSuccess*100, m.PrematureStopRate*100, m.GraphValidity*100)
	fmt.Printf("  延迟: avg=%v p50=%v p95=%v\n", m.AvgLatency, m.P50Latency, m.P95Latency)
	fmt.Printf("  平均调用: LLM=%.1f Tool=%.1f RAG=%.1f\n", m.AvgLLMCalls, m.AvgToolCalls, m.AvgRAGCalls)
	fmt.Printf("  降级率: %.2f%%\n", m.DegradationRate*100)
	fmt.Printf("  可追溯性: %.2f%%\n", m.Traceability*100)
	fmt.Printf("  行为契约符合率: %.2f%%\n", m.ContractCompliance*100)
	if len(m.FailuresByPhase) > 0 {
		fmt.Printf("  失败阶段: %v\n", m.FailuresByPhase)
	}
}

func printDetails(results []goseval.EvalResult) {
	fmt.Println("\n  详细结果:")
	for _, r := range results {
		match := "✓"
		if !r.Matched {
			match = "✗"
		}
		fmt.Printf("  %s %s: pred=%q truth=%q (%v)\n", match, r.CaseID, r.Prediction, r.GroundTruth, r.Latency)
	}
}

func buildGoSEngine(evalProfile bool, recorded *recordedEvidenceSource) (*gos_engine.GoSEngine, *gos_engine.Config, error) {
	return buildGoSEngineFromConfig(gos_engine.DefaultConfig(), evalProfile, recorded)
}

func buildGoSEngineFromConfig(cfg *gos_engine.Config, evalProfile bool, recorded *recordedEvidenceSource) (*gos_engine.GoSEngine, *gos_engine.Config, error) {
	if cfg == nil {
		cfg = gos_engine.DefaultConfig()
	}
	if recorded != nil {
		applyCompactEvalConfig(cfg)
	}
	if evalProfile {
		applyDeterministicEvalConfig(cfg)
		cfg.StructuredGenerate = func(context.Context, string) (string, error) {
			return "", fmt.Errorf("eval profile intentionally exercises deterministic rule fallback")
		}
	}

	logger := &testLogger{}
	engine := gos_engine.NewGoSEngine(cfg, logger)

	toolReg := experts.NewToolRegistry()
	if !evalProfile && recorded == nil {
		aiopsservice.RegisterAIOpsGOSTools(toolReg)
		for _, expertCfg := range cfg.Experts {
			aiopsservice.RegisterAIOpsGOSExpert(engine, cfg, toolReg, expertCfg)
		}
		return engine, cfg, nil
	}
	toolNames := []string{"query_logs", "query_internal_docs"}
	if recorded != nil {
		recordedTool, err := recorded.Tool()
		if err != nil {
			return nil, nil, fmt.Errorf("build recorded telemetry tool: %w", err)
		}
		toolReg.Register(recordedTelemetryToolName, recordedTool)
		toolNames = []string{recordedTelemetryToolName}
	} else if evalProfile {
		toolReg.Register("query_logs", newFakeLogTool())
		toolReg.Register("query_internal_docs", newFakeInternalDocsTool())
	}

	var ragFunc experts.RAGQueryFunc
	var contentFunc experts.GenerateContentFunc
	if recorded != nil {
		ragFunc = recorded.RAGQuery
	} else if evalProfile {
		ragFunc = experts.RAGQueryFunc(fakeRAGQuery)
		contentFunc = experts.GenerateContentFunc(evalGenerateContent)
	}

	expertCfg := experts.ExpertRuntimeConfig{
		Name:                "linux_sre",
		Description:         "Linux SRE expert",
		ToolNames:           toolNames,
		MaxRetrievalSteps:   3,
		EvidenceMaxChars:    cfg.EvidenceMaxChars,
		RAGQueryFunc:        ragFunc,
		GenerateContentFunc: contentFunc,
		CallTimeout:         time.Duration(cfg.CallTimeoutMs) * time.Millisecond,
		ChatModelFactory:    models.OpenAIChatModelFactory(cfg.ModelPath),
	}
	engine.RegisterExpert("linux_sre", experts.NewLinuxSREExpert(expertCfg, toolReg))

	expertCfg.Name = "network_sre"
	expertCfg.Description = "Network SRE expert"
	engine.RegisterExpert("network_sre", experts.NewNetworkSREExpert(expertCfg, toolReg))

	expertCfg.Name = "database_sre"
	expertCfg.Description = "Database SRE expert"
	engine.RegisterExpert("database_sre", experts.NewDatabaseSREExpert(expertCfg, toolReg))

	return engine, cfg, nil
}

func applyCompactEvalConfig(cfg *gos_engine.Config) {
	cfg.SessionMaxSteps = 3
	cfg.FSM.GapDelta = 0.2
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 3
	cfg.FSM.MinConfidence = 0.6
	cfg.CallTimeoutMs = 10000
	cfg.ModelPath = "chat_model_fast"
}

func applyDeterministicEvalConfig(cfg *gos_engine.Config) {
	applyCompactEvalConfig(cfg)
	cfg.StructuredCognition.PlanBudget.LLMCalls = 2
	cfg.StructuredCognition.PlanBudget.ToolCalls = 1
	for index := range cfg.Experts {
		cfg.Experts[index].Budget.LLMCalls = 2
		cfg.Experts[index].Budget.ToolCalls = 1
	}
	cfg.StructuredCognition.Enabled = true
	cfg.StateConversion.Enabled = true
}

func buildGoSRunner(profile, recordedRoot string, recordedTimeout time.Duration) (*goseval.Runner, *gos_engine.Config, error) {
	if profile == "recorded" {
		cfg := gos_engine.DefaultConfig()
		cfg.SessionMaxSteps = 3
		cfg.FSM.GapDelta = 0.2
		cfg.FSM.MinSupport = 1
		cfg.FSM.MaxSteps = 3
		cfg.FSM.MinConfidence = 0.6
		cfg.CallTimeoutMs = 10000
		cfg.ModelPath = "chat_model_fast"
		runner := goseval.NewCaseRunner(func(caseID string) (goseval.EngineRunner, error) {
			source, err := newRecordedEvidenceSource(recordedRoot, caseID, recordedTimeout)
			if err != nil {
				return nil, err
			}
			engine, _, err := buildGoSEngine(false, source)
			return engine, err
		})
		return runner, cfg, nil
	}
	if profile != "real" && profile != "eval" {
		return nil, nil, fmt.Errorf("unsupported GoS profile %q", profile)
	}
	if profile == "real" {
		engine, cfg, err := buildGoSEngineFromConfig(loadRealGoSConfig(context.Background()), false, nil)
		if err != nil {
			return nil, nil, err
		}
		return goseval.NewRunner(engine), cfg, nil
	}
	engine, cfg, err := buildGoSEngine(true, nil)
	if err != nil {
		return nil, nil, err
	}
	return goseval.NewRunner(engine), cfg, nil
}

func main() {
	mode := flag.String("mode", "gos", "运行模式: preflight|gos|baseline|regression-baseline|compare|smoke|gate|export-runs|judge")
	baselineFile := flag.String("baseline", "baseline_result.json", "baseline artifact 文件路径")
	holdoutPath := flag.String("holdout", "", "评测数据集路径（未指定时按模式选择 holdout 或 regression）")
	outputFile := flag.String("output", "eval_result.json", "输出文件路径")
	gosProfile := flag.String("gos-profile", "real", "GoS 配置: real|recorded|eval")
	recordedEvidenceRoot := flag.String("recorded-evidence-root", "", "recorded profile 的 case 隔离盲证据根目录")
	recordedTimeoutMs := flag.Int("recorded-timeout-ms", 2000, "recorded 证据单次只读超时（毫秒）")
	verbose := flag.Bool("verbose", false, "输出评测过程中的 DEBUG 级模型与工具明细")
	outputDir := flag.String("output-dir", "evals/runs", "export-runs 模式的输出目录")
	inputDir := flag.String("input", "evals/runs", "judge 输入目录（diag JSONL 文件）")
	flag.Parse()
	datasetPath := resolveDatasetPath(*mode, *holdoutPath)
	if strings.TrimSpace(*holdoutPath) == "" && *gosProfile == "recorded" {
		datasetPath = defaultRecordedDataset
	}
	recordedTimeout := time.Duration(*recordedTimeoutMs) * time.Millisecond

	if err := common.LoadPreferredEnvFile(); err != nil {
		fmt.Printf("加载 env 文件失败: %v\n", err)
		os.Exit(1)
	}
	if !*verbose {
		_ = g.Log().SetLevelStr("INFO")
	}
	ctx := context.Background()
	dependencyCfg := loadRealDependencyConfig(ctx)
	if err := validateProfileForMode(*mode, *gosProfile); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	if *mode == "preflight" {
		if err := printRealDependencyPreflight(dependencyCfg); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("静态配置已就绪；请在获准的真实环境中执行 baseline/compare 完成连通性验证。")
		return
	}
	if requiresRealDependencies(*mode, *gosProfile) {
		if err := validateRealDependencyConfig(dependencyCfg); err != nil {
			fmt.Printf("ERROR: 真实评测依赖未就绪: %v\n", err)
			fmt.Println("先运行 --mode=preflight；不要用降级结果生成 real baseline/compare artifact。")
			os.Exit(1)
		}
		if requiresVerifiedTelemetry(*mode, *gosProfile) {
			if err := errors.Join(
				validateVerifiedTelemetry(dependencyCfg.TelemetryProfile),
				validateTelemetryProvenance(dependencyCfg.TelemetrySource),
			); err != nil {
				fmt.Printf("ERROR: 真实评测遥测未就绪: %v\n", err)
				fmt.Println("本地 synthetic/unverified 遥测只能用于连通性与开发验证，不能生成 real baseline/compare artifact。")
				os.Exit(1)
			}
		}
		if err := bootstrapRealRAG(ctx); err != nil {
			fmt.Printf("ERROR: 真实 RAG 初始化失败: %v\n", err)
			os.Exit(1)
		}
		defer func() {
			if err := inframv.CloseAllClients(); err != nil {
				fmt.Printf("WARNING: 关闭 Milvus client 失败: %v\n", err)
			}
		}()
	}
	if requiresRecordedReplay(*mode, *gosProfile) {
		if err := errors.Join(
			validateRecordedDependencyConfig(dependencyCfg),
			validateRecordedReplayConfig(*recordedEvidenceRoot, recordedTimeout),
		); err != nil {
			fmt.Printf("ERROR: recorded 评测依赖未就绪: %v\n", err)
			os.Exit(1)
		}
		if err := validateRecordedCorpus(ctx, datasetPath, *recordedEvidenceRoot, recordedTimeout); err != nil {
			fmt.Printf("ERROR: recorded corpus 校验失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Recorded profile: 真实 LLM + case 隔离 recorded_blind 遥测；仅限 development_only 评测。")
	}

	switch *mode {
	case "gos":
		runGoSOnly(datasetPath, *outputFile, *gosProfile, *recordedEvidenceRoot, recordedTimeout)
	case "baseline":
		runBaseline(datasetPath, *outputFile, *gosProfile, *recordedEvidenceRoot, recordedTimeout)
	case "regression-baseline":
		runRegressionBaseline(datasetPath, *outputFile)
	case "compare":
		runCompare(datasetPath, *baselineFile, *outputFile, *gosProfile, *recordedEvidenceRoot, recordedTimeout)
	case "smoke":
		runSmoke(datasetPath, *outputFile)
	case "gate":
		runGate(datasetPath, *baselineFile, *outputFile)
	case "export-runs":
		runExportRuns(datasetPath, *outputDir, *gosProfile)
	case "judge":
		runJudge(*inputDir, *outputDir)
	default:
		fmt.Printf("未知模式: %s\n", *mode)
		fmt.Println("可用模式: preflight, gos, baseline, regression-baseline, compare, smoke, gate, export-runs, judge")
		os.Exit(1)
	}
}

func runGoSOnly(holdoutPath, outputFile, gosProfile, recordedRoot string, recordedTimeout time.Duration) {
	fmt.Println("=== GoS 评测 (gos 模式) ===")
	fmt.Println("注意: 此模式不判定 gate，需要 --mode=compare 对照 baseline")

	if _, err := loadDatasetForMode(holdoutPath, "gos", gosProfile); err != nil {
		fmt.Printf("ERROR: dataset 与模式不兼容: %v\n", err)
		os.Exit(1)
	}
	runner, cfg, err := buildGoSRunner(gosProfile, recordedRoot, recordedTimeout)
	if err != nil {
		fmt.Printf("ERROR: 创建 GoS runner 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("GoS profile: %s\n", gosProfile)
	fmt.Printf("配置: SessionMaxSteps=%d, GapDelta=%.2f, MinSupport=%d, MinConfidence=%.2f\n",
		cfg.SessionMaxSteps, cfg.FSM.GapDelta, cfg.FSM.MinSupport, cfg.FSM.MinConfidence)

	start := time.Now()
	metrics, results, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("GoS 评测失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("耗时: %v\n", time.Since(start))

	printMetrics("GoS", metrics)
	printDetails(results)
	codeHash, err := codeContentSHA256()
	if err != nil {
		fmt.Printf("ERROR: 计算当前代码内容指纹失败: %v\n", err)
		os.Exit(1)
	}

	resultJSON, err := json.MarshalIndent(map[string]interface{}{
		"mode":                     "gos",
		"profile":                  gosProfile,
		"evaluation_eligibility":   evaluationEligibility(gosProfile),
		"commit":                   gitCommit(),
		"code_sha256":              codeHash,
		"code_fingerprint_scope":   runtimeCodeFingerprintScope,
		"evidence_metric_contract": evidenceMetricContract,
		"metrics":                  metrics,
		"results":                  results,
	}, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: 序列化失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputFile, resultJSON, 0644); err != nil {
		fmt.Printf("ERROR: 写入结果文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n结果已保存到 %s\n", outputFile)
}

func runBaseline(holdoutPath, outputFile, profile, recordedRoot string, recordedTimeout time.Duration) {
	fmt.Println("=== Baseline 采集 (baseline 模式) ===")
	fmt.Println("使用真实 Plan-Execute-Replan (BuildPlanAgent)")

	dataset, err := loadDatasetForMode(holdoutPath, "baseline", profile)
	if err != nil {
		fmt.Printf("ERROR: 加载 holdout 失败: %v\n", err)
		os.Exit(1)
	}
	cases := dataset.Cases

	metrics := goseval.NewEvalMetrics()
	var results []goseval.EvalResult

	for i, c := range cases {
		fmt.Printf("  [%d/%d] %s: %s\n", i+1, len(cases), c.ID, truncateSymptom(c.Symptom, 50))

		runCtx := context.Background()
		var recordedSource *recordedEvidenceSource
		if profile == "recorded" {
			source, sourceErr := newRecordedEvidenceSource(recordedRoot, c.ID, recordedTimeout)
			if sourceErr != nil {
				fmt.Printf("ERROR: 创建 case %s 的 recorded 证据源失败: %v\n", c.ID, sourceErr)
				os.Exit(1)
			}
			recordedTool, toolErr := source.Tool()
			if toolErr != nil {
				fmt.Printf("ERROR: 创建 case %s 的 recorded 工具失败: %v\n", c.ID, toolErr)
				os.Exit(1)
			}
			recordedSource = source
			runCtx = plan_execute_replan.WithExecutorTools(runCtx, []tool.BaseTool{recordedTool})
		}
		start := time.Now()
		prediction, detail, runStats, err := plan_execute_replan.BuildPlanAgentWithStats(runCtx, c.Symptom)
		latency := time.Since(start)

		status := "succeeded"
		if err != nil {
			status = "degraded"
			prediction = degradedBaselinePrediction(prediction, err)
		}

		matched := goseval.MatchPrediction(prediction, c.GroundTruth, c.ExpectedKeywords)
		evidence := make([]protocol.EvidenceItem, 0, len(detail))
		if recordedSource != nil && runStats.ToolCalls > 0 {
			recordedDocument, loadErr := recordedSource.Load(context.Background())
			if loadErr != nil {
				fmt.Printf("ERROR: 读取 case %s 的 source-backed recorded 证据失败: %v\n", c.ID, loadErr)
				os.Exit(1)
			}
			evidence = append(evidence, protocol.EvidenceItem{
				SourceType: "recorded_replay",
				SourceID:   "recorded://" + c.ID,
				Title:      "Case-scoped recorded telemetry",
				Snippet:    recordedDocument,
			})
		} else {
			for index, step := range detail {
				evidence = append(evidence, protocol.EvidenceItem{
					SourceType: "plan_trace",
					SourceID:   fmt.Sprintf("%s-step-%d", c.ID, index+1),
					Title:      "Plan-Execute-Replan trace",
					Snippet:    step,
				})
			}
		}
		evidenceCount, relevantEvidence, expectedEvidence, coveredEvidence := goseval.EvaluateEvidenceMetrics(evidence, c.ExpectedEvidenceKeywords)
		statusMatches := c.ExpectedStatus == "" || c.ExpectedStatus == status
		prematureStop := goseval.IsPrematureStop(
			status,
			statusMatches,
			c.RequireRefine,
			false,
			c.RequireBacktrack,
			false,
		)
		failurePhase := ""
		if err != nil {
			failurePhase = baselineFailurePhase(err)
		} else if !matched || !statusMatches || c.RequireRefine || c.RequireBacktrack {
			failurePhase = "report"
		}
		failurePhaseMatches := c.ExpectedFailurePhase == "" || c.ExpectedFailurePhase == failurePhase

		r := &goseval.EvalResult{
			CaseID:               c.ID,
			Scenario:             c.Scenario,
			Symptom:              c.Symptom,
			GroundTruth:          c.GroundTruth,
			Prediction:           prediction,
			Status:               status,
			ExpectedStatus:       c.ExpectedStatus,
			StatusMatched:        statusMatches,
			Latency:              latency,
			LLMCalls:             runStats.LLMCalls,
			ToolCalls:            runStats.ToolCalls,
			RAGCalls:             runStats.RAGCalls,
			EvidenceCount:        evidenceCount,
			RelevantEvidence:     relevantEvidence,
			ExpectedEvidence:     expectedEvidence,
			CoveredEvidence:      coveredEvidence,
			Matched:              matched,
			TraceComplete:        len(detail) > 0,
			GraphValid:           true,
			BacktrackRequired:    c.RequireBacktrack,
			PrematureStop:        prematureStop,
			FailurePhase:         failurePhase,
			ExpectedFailurePhase: c.ExpectedFailurePhase,
			FailurePhaseMatched:  failurePhaseMatches,
			ContractMatched:      statusMatches && failurePhaseMatches && !c.RequireRefine && !c.RequireBacktrack,
		}
		metrics.AddResult(r)
		results = append(results, *r)

		matchStr := "✓"
		if !matched {
			matchStr = "✗"
		}
		fmt.Printf("         %s pred=%q truth=%q (%v)\n", matchStr, truncateSymptom(prediction, 60), c.GroundTruth, latency)
	}

	metrics.Finalize()

	printMetrics("Baseline (Plan-Execute-Replan)", metrics)
	if err := validateBaselineRunQuality(BaselineArtifact{
		BaselineQualityContract: baselineQualityContract,
		Profile:                 profile,
		Metrics:                 metrics,
		Results:                 results,
	}); err != nil {
		fmt.Printf("ERROR: baseline 运行质量不满足准入，拒绝生成 artifact: %v\n", err)
		os.Exit(1)
	}

	datasetHash, err := fileSHA256(holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: 计算 dataset SHA256 失败: %v\n", err)
		os.Exit(1)
	}
	configHash, err := fileSHA256(evalConfigPath)
	if err != nil {
		fmt.Printf("ERROR: 计算配置 SHA256 失败: %v\n", err)
		os.Exit(1)
	}
	codeHash, err := baselineCodeContentSHA256()
	if err != nil {
		fmt.Printf("ERROR: 计算当前代码内容指纹失败: %v\n", err)
		os.Exit(1)
	}
	evidenceCorpusHash := ""
	if profile == "recorded" {
		evidenceCorpusHash, err = recordedCorpusSHA256(context.Background(), holdoutPath, recordedRoot, recordedTimeout)
		if err != nil {
			fmt.Printf("ERROR: 计算 recorded evidence corpus SHA256 失败: %v\n", err)
			os.Exit(1)
		}
	}
	dependencyState := map[string]string{
		"llm":                  "real",
		"tools":                "real",
		"rag":                  "real",
		"telemetry":            "real",
		"telemetry_provenance": loadTelemetryProvenance(context.Background()),
	}
	toolConfig := "plan_execute_replan"
	if profile == "recorded" {
		dependencyState = map[string]string{
			"llm":                    "real",
			"tools":                  "case_scoped_recorded_blind",
			"rag":                    "case_scoped_recorded_blind",
			"telemetry":              "recorded_blind",
			"telemetry_provenance":   "recorded_blind",
			"evaluation_eligibility": "development_only",
		}
		toolConfig = "plan_execute_replan+case_scoped_recorded_blind"
	}
	artifact := BaselineArtifact{
		Commit:                  gitCommit(),
		CodeSHA256:              codeHash,
		CodeFingerprintScope:    baselineCodeFingerprintScope,
		EvidenceMetricContract:  evidenceMetricContract,
		BaselineQualityContract: baselineQualityContract,
		Model:                   "OpenAIForGLM",
		ToolConfig:              toolConfig,
		Profile:                 profile,
		PromptVersion:           "plan-execute-replan-v1",
		DependencyState:         dependencyState,
		DatasetSchemaVersion:    dataset.SchemaVersion,
		DatasetRole:             dataset.Role,
		DatasetPath:             holdoutPath,
		DatasetSHA256:           datasetHash,
		ConfigPath:              evalConfigPath,
		ConfigSHA256:            configHash,
		ConfigFingerprintScope:  fullConfigFingerprintScope,
		ConfigSummary: map[string]interface{}{
			"engine":               "plan_execute_replan",
			"mode":                 "baseline",
			"profile":              profile,
			"telemetry_provenance": dependencyState["telemetry_provenance"],
			"call_counting":        "eino_callbacks_v1",
		},
		EvidenceCorpusSHA256:  evidenceCorpusHash,
		EvidenceProvenance:    dependencyState["telemetry_provenance"],
		EvaluationEligibility: evaluationEligibility(profile),
		Timestamp:             time.Now().Format(time.RFC3339),
		Metrics:               metrics,
		Results:               results,
	}
	if profile == "real" {
		artifact.EvaluationEligibility = realEvaluationEligibility(artifact.EvidenceProvenance)
	}

	resultJSON, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: 序列化失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputFile, resultJSON, 0644); err != nil {
		fmt.Printf("ERROR: 写入结果文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nBaseline artifact 已保存到 %s\n", outputFile)
	fmt.Printf("用 --mode=compare --baseline=%s 对照 GoS 结果\n", outputFile)
}

func degradedBaselinePrediction(prediction string, err error) string {
	prediction = strings.TrimSpace(prediction)
	if prediction != "" {
		return "[DEGRADED] " + prediction
	}
	return fmt.Sprintf("[ERROR] %v", err)
}

func baselineFailurePhase(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "analysis conclusion") || strings.Contains(message, "final report") || strings.Contains(message, "report") {
		return "report"
	}
	return "act"
}

func truncateSymptom(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}

func runCompare(holdoutPath, baselineFile, outputFile, gosProfile, recordedRoot string, recordedTimeout time.Duration) {
	if gosProfile != "real" && gosProfile != "recorded" {
		fmt.Println("ERROR: compare 模式只允许 --gos-profile=real|recorded；eval profile 只能用于 smoke/gos 开发回归")
		os.Exit(1)
	}
	dataset, err := loadDatasetForMode(holdoutPath, "compare", gosProfile)
	if err != nil {
		fmt.Printf("ERROR: dataset 与 compare 模式不兼容: %v\n", err)
		os.Exit(1)
	}

	data, err := os.ReadFile(baselineFile)
	if err != nil {
		fmt.Printf("ERROR: 无法读取 baseline 文件 %s: %v\n", baselineFile, err)
		fmt.Println("请先用 --mode=baseline 生成 baseline artifact")
		fmt.Println("或用 --mode=gos 只看 GoS metrics")
		os.Exit(1)
	}

	var artifact BaselineArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		fmt.Printf("ERROR: baseline 文件格式错误: %v\n", err)
		os.Exit(1)
	}

	if err := validateArtifactUse(artifact, "compare", gosProfile); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	if err := validateArtifactFingerprints(artifact, holdoutPath, evalConfigPath); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	if gosProfile == "recorded" {
		if err := validateRecordedArtifactFingerprint(context.Background(), artifact, holdoutPath, recordedRoot, recordedTimeout); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			os.Exit(1)
		}
	}
	if err := validateArtifactCases(artifact, dataset); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("=== GoS vs Baseline 对比 (compare 模式) ===")
	fmt.Printf("Baseline 来源: %s\n", baselineFile)
	fmt.Printf("Baseline commit: %s\n", artifact.Commit)
	fmt.Printf("Baseline model: %s\n", artifact.Model)
	fmt.Printf("Baseline 时间: %s\n", artifact.Timestamp)
	fmt.Println()

	runner, cfg, err := buildGoSRunner(gosProfile, recordedRoot, recordedTimeout)
	if err != nil {
		fmt.Printf("ERROR: 创建 GoS runner 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("GoS profile: %s\n", gosProfile)
	fmt.Printf("GoS 配置: SessionMaxSteps=%d, GapDelta=%.2f, MinSupport=%d, MinConfidence=%.2f\n",
		cfg.SessionMaxSteps, cfg.FSM.GapDelta, cfg.FSM.MinSupport, cfg.FSM.MinConfidence)

	start := time.Now()
	gosMetrics, gosResults, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("GoS 评测失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("GoS 耗时: %v\n", time.Since(start))

	printMetrics("GoS", gosMetrics)
	printDetails(gosResults)

	printMetrics("Baseline", artifact.Metrics)
	printDetails(artifact.Results)

	gateReport := goseval.CheckGate(gosMetrics, artifact.Metrics)

	fmt.Println("\n=== Gate 检查 (GoS vs Baseline) ===")
	for _, g := range gateReport.Gates {
		status := "✓ PASS"
		if !g.Passed {
			status = "✗ FAIL"
		}
		fmt.Printf("  %s %s: expected %s, actual %s\n", status, g.Name, g.Expected, g.Actual)
	}
	fmt.Println()
	if gateReport.AllPassed {
		fmt.Println("✓ 所有 Gate 通过，GoS 不劣于 Baseline")
	} else {
		fmt.Println("✗ 部分 Gate 未通过")
		for _, g := range gateReport.Gates {
			if !g.Passed {
				fmt.Printf("  需优化: %s (%s)\n", g.Name, g.Actual)
			}
		}
	}

	runtimeCodeHash, err := codeContentSHA256()
	if err != nil {
		fmt.Printf("ERROR: 计算当前 GoS 代码内容指纹失败: %v\n", err)
		os.Exit(1)
	}
	resultJSON, err := json.MarshalIndent(map[string]interface{}{
		"mode":                     "compare",
		"profile":                  gosProfile,
		"evaluation_eligibility":   artifact.EvaluationEligibility,
		"commit":                   gitCommit(),
		"code_sha256":              runtimeCodeHash,
		"code_fingerprint_scope":   runtimeCodeFingerprintScope,
		"evidence_metric_contract": artifact.EvidenceMetricContract,
		"baseline_commit":          artifact.Commit,
		"baseline_model":           artifact.Model,
		"gos_metrics":              gosMetrics,
		"gos_results":              gosResults,
		"baseline":                 artifact,
		"gate":                     gateReport,
	}, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: 序列化失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputFile, resultJSON, 0644); err != nil {
		fmt.Printf("ERROR: 写入结果文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n详细结果已保存到 %s\n", outputFile)

	if !gateReport.AllPassed {
		os.Exit(1)
	}
}

func runRegressionBaseline(datasetPath, outputFile string) {
	fmt.Println("=== Regression Baseline 采集 (确定性 eval profile) ===")
	dataset, err := loadDatasetForMode(datasetPath, "regression-baseline", "eval")
	if err != nil {
		fmt.Printf("ERROR: 加载 regression dataset 失败: %v\n", err)
		os.Exit(1)
	}
	runner, cfg, err := buildGoSRunner("eval", "", 0)
	if err != nil {
		fmt.Printf("ERROR: 创建 regression runner 失败: %v\n", err)
		os.Exit(1)
	}
	metrics, results, err := runner.RunFromCases(context.Background(), dataset.Cases)
	if err != nil {
		fmt.Printf("ERROR: 运行 regression baseline 失败: %v\n", err)
		os.Exit(1)
	}
	datasetHash, err := fileSHA256(datasetPath)
	if err != nil {
		fmt.Printf("ERROR: 计算 dataset SHA256 失败: %v\n", err)
		os.Exit(1)
	}
	configSummary := evalConfigSummary(cfg)
	configHash, err := configSummarySHA256(configSummary)
	if err != nil {
		fmt.Printf("ERROR: 计算 eval 配置指纹失败: %v\n", err)
		os.Exit(1)
	}
	codeHash, err := gateCodeContentSHA256()
	if err != nil {
		fmt.Printf("ERROR: 计算当前代码内容指纹失败: %v\n", err)
		os.Exit(1)
	}
	artifact := BaselineArtifact{
		Commit:                  gitCommit(),
		CodeSHA256:              codeHash,
		CodeFingerprintScope:    gateCodeFingerprintScope,
		EvidenceMetricContract:  evidenceMetricContract,
		BaselineQualityContract: baselineQualityContract,
		Model:                   "deterministic-eval",
		ToolConfig:              "fake-log+fake-docs+fake-rag",
		Profile:                 "eval",
		PromptVersion:           "gos-deterministic-eval-v1",
		DependencyState: map[string]string{
			"llm":       "deterministic",
			"tools":     "deterministic",
			"rag":       "deterministic",
			"telemetry": "deterministic",
		},
		DatasetSchemaVersion:   dataset.SchemaVersion,
		DatasetRole:            dataset.Role,
		DatasetPath:            datasetPath,
		DatasetSHA256:          datasetHash,
		ConfigPath:             evalConfigPath,
		ConfigSHA256:           configHash,
		ConfigFingerprintScope: evalConfigFingerprintScope,
		ConfigSummary:          configSummary,
		Timestamp:              time.Now().Format(time.RFC3339),
		Metrics:                metrics,
		Results:                results,
	}
	if err := validateBaselineRunQuality(artifact); err != nil {
		fmt.Printf("ERROR: regression baseline 运行质量不满足准入，拒绝生成 artifact: %v\n", err)
		os.Exit(1)
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: 序列化 regression baseline 失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputFile, data, 0o644); err != nil {
		fmt.Printf("ERROR: 写入 regression baseline 失败: %v\n", err)
		os.Exit(1)
	}
	printMetrics("Regression Baseline", metrics)
	fmt.Printf("\nRegression baseline 已保存到 %s\n", outputFile)
}

func runSmoke(holdoutPath, outputFile string) {
	fmt.Println("=== Smoke 评测 (smoke 模式) ===")
	fmt.Println("注意: baseline 是确定性模拟，仅用于开发回归，不能作为 Phase 3 gate")

	dataset, err := loadDatasetForMode(holdoutPath, "smoke", "eval")
	if err != nil {
		fmt.Printf("ERROR: regression dataset 无效: %v\n", err)
		os.Exit(1)
	}
	runner, _, err := buildGoSRunner("eval", "", 0)
	if err != nil {
		fmt.Printf("GoS runner 创建失败: %v\n", err)
		os.Exit(1)
	}
	start := time.Now()
	gosMetrics, gosResults, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("GoS 评测失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("GoS 耗时: %v\n", time.Since(start))

	printMetrics("GoS", gosMetrics)
	printDetails(gosResults)

	smoke := newSmokeBaselineRunner()
	cases := dataset.Cases

	smokeMetrics := goseval.NewEvalMetrics()
	var smokeResults []goseval.EvalResult
	for _, c := range cases {
		r := smoke.runCase(context.Background(), c)
		smokeMetrics.AddResult(r)
		smokeResults = append(smokeResults, *r)
	}
	smokeMetrics.Finalize()

	printMetrics("Smoke Baseline (确定性模拟)", smokeMetrics)
	printDetails(smokeResults)

	gateReport := goseval.CheckGate(gosMetrics, smokeMetrics)

	fmt.Println("\n=== Gate 检查 (仅开发参考) ===")
	for _, g := range gateReport.Gates {
		status := "✓ PASS"
		if !g.Passed {
			status = "✗ FAIL"
		}
		fmt.Printf("  %s %s: expected %s, actual %s\n", status, g.Name, g.Expected, g.Actual)
	}
	fmt.Println()
	fmt.Println("注意: 此 gate 仅用于开发回归，不能作为 Phase 3 准入依据")

	resultJSON, err := json.MarshalIndent(map[string]interface{}{
		"mode":          "smoke",
		"commit":        gitCommit(),
		"gos_metrics":   gosMetrics,
		"gos_results":   gosResults,
		"smoke_metrics": smokeMetrics,
		"smoke_results": smokeResults,
		"gate":          gateReport,
	}, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: 序列化失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outputFile, resultJSON, 0644); err != nil {
		fmt.Printf("ERROR: 写入结果文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n结果已保存到 %s\n", outputFile)
}

func runGate(holdoutPath, baselineFile, outputFile string) {
	fmt.Println("=== Eval Gate (确定性回归检查) ===")
	if _, err := loadDatasetForMode(holdoutPath, "gate", "eval"); err != nil {
		fmt.Printf("ERROR: regression dataset 无效: %v\n", err)
		os.Exit(1)
	}

	// 1. Read baseline artifact
	data, err := os.ReadFile(baselineFile)
	if err != nil {
		fmt.Printf("ERROR: 读取 baseline 失败: %v\n", err)
		os.Exit(1)
	}
	var artifact BaselineArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		fmt.Printf("ERROR: 解析 baseline 失败: %v\n", err)
		os.Exit(1)
	}
	if err := validateArtifactUse(artifact, "gate", "eval"); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	if err := validateArtifactFingerprints(artifact, holdoutPath, evalConfigPath); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}
	dataset, err := goseval.LoadDataset(holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: 加载 regression dataset 失败: %v\n", err)
		os.Exit(1)
	}
	if err := validateArtifactCases(artifact, dataset); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		os.Exit(1)
	}

	// 2. Run GoS with eval profile (deterministic, no LLM)
	runner, _, err := buildGoSRunner("eval", "", 0)
	if err != nil {
		fmt.Printf("ERROR: 创建 GoS runner 失败: %v\n", err)
		os.Exit(1)
	}
	start := time.Now()
	gosMetrics, gosResults, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: GoS 运行失败: %v\n", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)

	// 3. Check gate
	gateReport := goseval.CheckGate(gosMetrics, artifact.Metrics)

	// 4. Print results
	printMetrics("GoS", gosMetrics)
	fmt.Printf("\n--- Gate 结果 ---\n")
	for _, g := range gateReport.Gates {
		status := "PASS"
		if !g.Passed {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %s: expected=%s, actual=%s\n", status, g.Name, g.Expected, g.Actual)
	}
	fmt.Printf("\n总耗时: %v\n", elapsed)

	// 5. Write output
	output := map[string]interface{}{
		"mode":          "gate",
		"commit":        gitCommit(),
		"baseline_file": baselineFile,
		"gos_metrics":   gosMetrics,
		"gos_results":   gosResults,
		"baseline":      artifact.Metrics,
		"gate":          gateReport,
		"elapsed_ms":    elapsed.Milliseconds(),
	}
	outData, _ := json.MarshalIndent(output, "", "  ")
	if err := os.WriteFile(outputFile, outData, 0o644); err != nil {
		fmt.Printf("WARNING: 写入输出文件失败: %v\n", err)
	}

	if !gateReport.AllPassed {
		fmt.Println("\n❌ Gate 未通过")
		os.Exit(1)
	}
	fmt.Println("\n✅ Gate 通过")
}

func runExportRuns(holdoutPath, outputDir, gosProfile string) {
	fmt.Println("=== Export Runs ===")
	if _, err := loadDatasetForMode(holdoutPath, "export-runs", gosProfile); err != nil {
		fmt.Printf("ERROR: dataset 与 export-runs 模式不兼容: %v\n", err)
		os.Exit(1)
	}

	runner, _, err := buildGoSRunner(gosProfile, "", 0)
	if err != nil {
		fmt.Printf("ERROR: 创建 GoS runner 失败: %v\n", err)
		os.Exit(1)
	}

	metrics, results, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: GoS 运行失败: %v\n", err)
		os.Exit(1)
	}

	printMetrics("GoS", metrics)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Printf("ERROR: 创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	ts := time.Now().Format("20060102150405")

	// Write GoS results
	gosOutput := map[string]interface{}{
		"mode":      "export-runs",
		"commit":    gitCommit(),
		"profile":   gosProfile,
		"holdout":   holdoutPath,
		"timestamp": time.Now().Format(time.RFC3339),
		"metrics":   metrics,
		"results":   results,
	}
	gosData, _ := json.MarshalIndent(gosOutput, "", "  ")
	gosFile := filepath.Join(outputDir, fmt.Sprintf("gos_%s.json", ts))
	if err := os.WriteFile(gosFile, gosData, 0o644); err != nil {
		fmt.Printf("WARNING: 写入 GoS 结果失败: %v\n", err)
	} else {
		fmt.Printf("GoS 结果: %s\n", gosFile)
	}

	// Write diag runs (JSONL, one case per line)
	diagFile := filepath.Join(outputDir, fmt.Sprintf("diag_%s.jsonl", ts))
	var diagLines []string
	for _, r := range results {
		line := map[string]interface{}{
			"case_id":          r.CaseID,
			"query":            r.Symptom,
			"actual_output":    r.Prediction,
			"tools_called":     []string{},
			"evidence_context": []string{},
			"latency_ms":       r.Latency.Milliseconds(),
			"llm_calls":        r.LLMCalls,
			"matched":          r.Matched,
			"status":           r.Status,
		}
		lineData, _ := json.Marshal(line)
		diagLines = append(diagLines, string(lineData))
	}
	if err := os.WriteFile(diagFile, []byte(strings.Join(diagLines, "\n")+"\n"), 0o644); err != nil {
		fmt.Printf("WARNING: 写入 diag runs 失败: %v\n", err)
	} else {
		fmt.Printf("Diag runs: %s\n", diagFile)
	}
}

func runJudge(inputDir, outputDir string) {
	fmt.Println("=== LLM Judge 评分 ===")

	judge, err := judgeeval.NewJudgeRunner(context.Background())
	if err != nil {
		fmt.Printf("ERROR: 创建 JudgeRunner 失败: %v\n", err)
		os.Exit(1)
	}

	// Find diag JSONL files
	files, err := filepath.Glob(filepath.Join(inputDir, "diag_*.jsonl"))
	if err != nil || len(files) == 0 {
		fmt.Printf("ERROR: 未找到 diag_*.jsonl 文件在 %s\n", inputDir)
		os.Exit(1)
	}

	type judgeEntry struct {
		CaseID string               `json:"case_id"`
		Query  string               `json:"query"`
		Scores judgeeval.DiagScores `json:"scores"`
		Error  string               `json:"error,omitempty"`
	}

	var allResults []judgeEntry
	totalScored := 0
	totalErrors := 0

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			fmt.Printf("WARNING: 读取 %s 失败: %v\n", f, err)
			continue
		}

		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			var run struct {
				CaseID          string   `json:"case_id"`
				Query           string   `json:"query"`
				ActualOutput    string   `json:"actual_output"`
				EvidenceContext []string `json:"evidence_context"`
			}
			if err := json.Unmarshal([]byte(line), &run); err != nil {
				fmt.Printf("WARNING: 解析 JSONL 行失败: %v\n", err)
				continue
			}

			fmt.Printf("  评分: %s ... ", run.CaseID)
			scores, err := judge.Score(context.Background(), run.Query, run.ActualOutput, run.EvidenceContext)
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
				allResults = append(allResults, judgeEntry{CaseID: run.CaseID, Query: run.Query, Error: err.Error()})
				totalErrors++
			} else {
				fmt.Printf("C=%d Co=%d Ch=%d A=%d O=%d\n", scores.Correctness, scores.Completeness, scores.Coherence, scores.Actionability, scores.Overall)
				allResults = append(allResults, judgeEntry{CaseID: run.CaseID, Query: run.Query, Scores: *scores})
				totalScored++
			}
		}
	}

	fmt.Printf("\n评分完成: %d 成功, %d 失败\n", totalScored, totalErrors)

	// Write output
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Printf("ERROR: 创建输出目录失败: %v\n", err)
		os.Exit(1)
	}

	ts := time.Now().Format("20060102150405")
	output := map[string]interface{}{
		"mode":      "judge",
		"commit":    gitCommit(),
		"input_dir": inputDir,
		"timestamp": time.Now().Format(time.RFC3339),
		"total":     len(allResults),
		"scored":    totalScored,
		"errors":    totalErrors,
		"results":   allResults,
	}
	outData, _ := json.MarshalIndent(output, "", "  ")
	outFile := filepath.Join(outputDir, fmt.Sprintf("judge_%s.json", ts))
	if err := os.WriteFile(outFile, outData, 0o644); err != nil {
		fmt.Printf("WARNING: 写入输出文件失败: %v\n", err)
	} else {
		fmt.Printf("结果: %s\n", outFile)
	}
}
