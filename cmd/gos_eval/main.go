package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/agent/gos_engine/eval"
	"SuperBizAgent/internal/ai/agent/plan_execute_replan"
	"SuperBizAgent/internal/ai/belief"
	aitools "SuperBizAgent/internal/ai/tools"

	"github.com/cloudwego/eino/components/tool"
	einoschema "github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// types
// ---------------------------------------------------------------------------

type BaselineArtifact struct {
	Commit      string            `json:"commit"`
	Model       string            `json:"model"`
	ToolConfig  string            `json:"tool_config"`
	HoldoutPath string            `json:"holdout_path"`
	Timestamp   string            `json:"timestamp"`
	Metrics     *eval.EvalMetrics `json:"metrics"`
	Results     []eval.EvalResult `json:"results"`
}

// ---------------------------------------------------------------------------
// test logger
// ---------------------------------------------------------------------------

type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[INFO] %s %v\n", msg, keysAndValues)
}
func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[ERROR] %s %v\n", msg, keysAndValues)
}

// ---------------------------------------------------------------------------
// fake tools — shared between GoS experts and smoke baseline
// ---------------------------------------------------------------------------

type keywordResponse struct {
	keyword  string
	response string
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

// ---------------------------------------------------------------------------
// fake RAG
// ---------------------------------------------------------------------------

func fakeRAGQuery(ctx context.Context, query string) ([]*einoschema.Document, error) {
	ragData := map[string]string{
		"CPU":   "Runbook: CPU > 90% → check runaway processes, scale replicas",
		"数据库":   "Runbook: Connection pool exhaustion → increase max_connections, add read replicas",
		"连接池":   "Runbook: Connection pool exhaustion → increase max_connections, add read replicas",
		"网络":    "Runbook: Cross-region latency → check VPN/link health, switch to local endpoint",
		"跨区域":   "Runbook: Cross-region latency → check VPN/link health, switch to local endpoint",
		"缓存":    "Runbook: Cache miss spike → check TTL, warm cache, increase memory",
		"Redis": "Runbook: Cache miss spike → check TTL, warm cache, increase memory",
		"Kafka": "Runbook: Consumer lag → increase consumers, check partition count",
		"消息堆积":  "Runbook: Consumer lag → increase consumers, check partition count",
		"消费者":   "Runbook: Consumer lag → increase consumers, check partition count",
	}
	for keyword, content := range ragData {
		if contains(query, keyword) {
			return []*einoschema.Document{{Content: content}}, nil
		}
	}
	return []*einoschema.Document{{Content: "Generic troubleshooting: check logs, metrics, recent deployments"}}, nil
}

// ---------------------------------------------------------------------------
// eval-only content generation (simulates LLM analysis)
// ---------------------------------------------------------------------------

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
		var toolData string
		for _, h := range history {
			if h.Tool == "query_logs" || h.Tool == "query_internal_docs" {
				d := extractDataFieldEval(h.Output)
				if d != "" {
					toolData = d
				}
			}
		}
		conclusion := mapToolOutputToConclusion(toolData)
		if conclusion != "" {
			return conclusion, nil
		}
		return fmt.Sprintf("针对假设「%s」的分析：%s", frontier.Label, frontier.Why), nil
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

// ---------------------------------------------------------------------------
// smoke baseline (deterministic, for dev regression only)
// ---------------------------------------------------------------------------

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

func (b *smokeBaselineRunner) runCase(ctx context.Context, c eval.EvalCase) *eval.EvalResult {
	start := time.Now()
	llmCalls := 0
	evidenceCount := 0

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
			_ = output
			evidenceCount++
		}
	}
	llmCalls++

	prediction := b.analyzeSymptom(c.Symptom)

	return &eval.EvalResult{
		CaseID:        c.ID,
		Symptom:       c.Symptom,
		GroundTruth:   c.GroundTruth,
		Prediction:    prediction,
		Status:        "succeeded",
		Latency:       time.Since(start),
		LLMCalls:      llmCalls,
		EvidenceCount: evidenceCount,
		Matched:       eval.MatchPrediction(prediction, c.GroundTruth, c.ExpectedKeywords),
		TraceComplete: true,
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

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

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

func printMetrics(label string, m *eval.EvalMetrics) {
	fmt.Printf("\n--- %s ---\n", label)
	fmt.Printf("  总用例: %d\n", m.TotalCases)
	fmt.Printf("  成功: %d | 降级: %d | 失败: %d\n", m.Succeeded, m.Degraded, m.Failed)
	fmt.Printf("  准确率: %.2f%%\n", m.Accuracy*100)
	fmt.Printf("  证据覆盖率: %.2f%%\n", m.EvidenceCoverage*100)
	fmt.Printf("  平均延迟: %v\n", m.AvgLatency)
	fmt.Printf("  平均 LLM 调用: %.1f\n", m.AvgLLMCalls)
	fmt.Printf("  降级率: %.2f%%\n", m.DegradationRate*100)
	fmt.Printf("  可追溯性: %.2f%%\n", m.Traceability*100)
}

func printDetails(results []eval.EvalResult) {
	fmt.Println("\n  详细结果:")
	for _, r := range results {
		match := "✓"
		if !r.Matched {
			match = "✗"
		}
		fmt.Printf("  %s %s: pred=%q truth=%q (%v)\n", match, r.CaseID, r.Prediction, r.GroundTruth, r.Latency)
	}
}

// ---------------------------------------------------------------------------
// build GoS engine
// ---------------------------------------------------------------------------

// buildGoSEngine creates a GoS engine. When evalProfile is true, injects fake
// RAG and eval content generation (for smoke/eval only). When false, uses
// production defaults (no injection — real RAG and template content).
func buildGoSEngine(evalProfile bool) (*gos_engine.GoSEngine, *gos_engine.Config) {
	cfg := gos_engine.DefaultConfig()
	cfg.SessionMaxSteps = 3
	cfg.FSM.GapDelta = 0.2
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 3
	cfg.FSM.MinConfidence = 0.6
	cfg.CallTimeoutMs = 2000

	logger := &testLogger{}
	engine := gos_engine.NewGoSEngine(cfg, logger)

	toolReg := experts.NewToolRegistry()
	if evalProfile {
		toolReg.Register("query_logs", newFakeLogTool())
		toolReg.Register("query_internal_docs", newFakeInternalDocsTool())
	} else {
		registerProductionTools(toolReg)
	}

	var ragFunc experts.RAGQueryFunc
	var contentFunc experts.GenerateContentFunc
	if evalProfile {
		ragFunc = experts.RAGQueryFunc(fakeRAGQuery)
		contentFunc = experts.GenerateContentFunc(evalGenerateContent)
	}

	expertCfg := experts.ExpertRuntimeConfig{
		Name:                "linux_sre",
		Description:         "Linux SRE expert",
		ToolNames:           []string{"query_logs", "query_internal_docs"},
		MaxRetrievalSteps:   3,
		RAGQueryFunc:        ragFunc,
		GenerateContentFunc: contentFunc,
		CallTimeout:         time.Duration(cfg.CallTimeoutMs) * time.Millisecond,
	}
	engine.RegisterExpert("linux_sre", experts.NewLinuxSREExpert(expertCfg, toolReg))

	expertCfg.Name = "network_sre"
	expertCfg.Description = "Network SRE expert"
	engine.RegisterExpert("network_sre", experts.NewNetworkSREExpert(expertCfg, toolReg))

	expertCfg.Name = "database_sre"
	expertCfg.Description = "Database SRE expert"
	engine.RegisterExpert("database_sre", experts.NewDatabaseSREExpert(expertCfg, toolReg))

	return engine, cfg
}

func registerProductionTools(toolReg *experts.ToolRegistry) {
	toolReg.Register("query_internal_docs", aitools.NewQueryInternalDocsTool())

	registeredLog := false
	logTools, err := aitools.GetLogMcpTool()
	if err == nil {
		for _, t := range logTools {
			invokable, ok := t.(tool.InvokableTool)
			if !ok {
				continue
			}
			info, infoErr := invokable.Info(context.Background())
			if infoErr != nil || info == nil || info.Name == "" {
				continue
			}
			toolReg.Register(info.Name, invokable)
			if info.Name == "query_logs" {
				registeredLog = true
			}
		}
	}
	if !registeredLog {
		toolReg.Register("query_logs", aitools.NewUnavailableLogQueryTool(logToolUnavailableReason(err)))
	}
}

func logToolUnavailableReason(err error) string {
	if err != nil {
		return err.Error()
	}
	return "query_logs invokable tool is unavailable"
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	mode := flag.String("mode", "gos", "运行模式: gos|baseline|compare|smoke")
	baselineFile := flag.String("baseline", "baseline_result.json", "baseline artifact 文件路径")
	holdoutPath := flag.String("holdout", "internal/ai/agent/gos_engine/eval/testdata/holdout.json", "holdout 数据集路径")
	outputFile := flag.String("output", "eval_result.json", "输出文件路径")
	gosProfile := flag.String("gos-profile", "real", "GoS 配置: real|eval (real=生产行为, eval=fake deps)")
	flag.Parse()

	switch *mode {
	case "gos":
		runGoSOnly(*holdoutPath, *outputFile, *gosProfile)
	case "baseline":
		runBaseline(*holdoutPath, *outputFile)
	case "compare":
		runCompare(*holdoutPath, *baselineFile, *outputFile, *gosProfile)
	case "smoke":
		runSmoke(*holdoutPath, *outputFile)
	default:
		fmt.Printf("未知模式: %s\n", *mode)
		fmt.Println("可用模式: gos, baseline, compare, smoke")
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// gos: 只跑 GoS，输出 metrics，不判定 gate
// ---------------------------------------------------------------------------

func runGoSOnly(holdoutPath, outputFile, gosProfile string) {
	fmt.Println("=== GoS 评测 (gos 模式) ===")
	fmt.Println("注意: 此模式不判定 gate，需要 --mode=compare 对照 baseline")

	evalProfile := gosProfile == "eval"
	engine, cfg := buildGoSEngine(evalProfile)
	fmt.Printf("GoS profile: %s\n", gosProfile)
	fmt.Printf("配置: SessionMaxSteps=%d, GapDelta=%.2f, MinSupport=%d, MinConfidence=%.2f\n",
		cfg.SessionMaxSteps, cfg.FSM.GapDelta, cfg.FSM.MinSupport, cfg.FSM.MinConfidence)

	runner := eval.NewRunner(engine)
	start := time.Now()
	metrics, results, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("GoS 评测失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("耗时: %v\n", time.Since(start))

	printMetrics("GoS", metrics)
	printDetails(results)

	resultJSON, err := json.MarshalIndent(map[string]interface{}{
		"mode":    "gos",
		"commit":  gitCommit(),
		"metrics": metrics,
		"results": results,
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

// ---------------------------------------------------------------------------
// baseline: 跑真实 Plan-Execute-Replan，产出版本化 artifact
// ---------------------------------------------------------------------------

func runBaseline(holdoutPath, outputFile string) {
	fmt.Println("=== Baseline 采集 (baseline 模式) ===")
	fmt.Println("使用真实 Plan-Execute-Replan (BuildPlanAgent)")

	cases, err := eval.LoadCases(holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: 加载 holdout 失败: %v\n", err)
		os.Exit(1)
	}

	metrics := eval.NewEvalMetrics()
	var results []eval.EvalResult

	for i, c := range cases {
		fmt.Printf("  [%d/%d] %s: %s\n", i+1, len(cases), c.ID, truncateSymptom(c.Symptom, 50))

		start := time.Now()
		prediction, detail, err := plan_execute_replan.BuildPlanAgent(context.Background(), c.Symptom)
		latency := time.Since(start)

		status := "succeeded"
		if err != nil {
			status = "degraded"
			prediction = fmt.Sprintf("[ERROR] %v", err)
		}

		matched := eval.MatchPrediction(prediction, c.GroundTruth, c.ExpectedKeywords)

		r := &eval.EvalResult{
			CaseID:        c.ID,
			Symptom:       c.Symptom,
			GroundTruth:   c.GroundTruth,
			Prediction:    prediction,
			Status:        status,
			Latency:       latency,
			LLMCalls:      len(detail),
			EvidenceCount: len(detail),
			Matched:       matched,
			TraceComplete: len(detail) > 0,
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

	artifact := BaselineArtifact{
		Commit:      gitCommit(),
		Model:       "OpenAIForGLM",
		ToolConfig:  "plan_execute_replan",
		HoldoutPath: holdoutPath,
		Timestamp:   time.Now().Format(time.RFC3339),
		Metrics:     metrics,
		Results:     results,
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

func truncateSymptom(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// ---------------------------------------------------------------------------
// compare: 跑 GoS + 读取 baseline artifact，判定 gate
// ---------------------------------------------------------------------------

func runCompare(holdoutPath, baselineFile, outputFile, gosProfile string) {
	if gosProfile != "real" {
		fmt.Println("ERROR: compare 模式只允许 --gos-profile=real；eval profile 只能用于 smoke/gos 开发回归")
		os.Exit(1)
	}

	// 读取 baseline artifact
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

	// Validate artifact
	if artifact.Metrics == nil {
		fmt.Println("ERROR: baseline artifact 缺少 metrics 字段")
		os.Exit(1)
	}
	if len(artifact.Results) == 0 {
		fmt.Println("ERROR: baseline artifact results 为空")
		os.Exit(1)
	}
	if artifact.HoldoutPath != "" && artifact.HoldoutPath != holdoutPath {
		fmt.Printf("ERROR: baseline artifact holdout 路径不匹配\n")
		fmt.Printf("  artifact: %s\n", artifact.HoldoutPath)
		fmt.Printf("  current:  %s\n", holdoutPath)
		fmt.Println("请使用匹配的 baseline artifact，或重新采集 baseline")
		os.Exit(1)
	}

	// Validate case alignment
	cases, err := eval.LoadCases(holdoutPath)
	if err != nil {
		fmt.Printf("ERROR: 无法加载 holdout: %v\n", err)
		os.Exit(1)
	}
	if len(artifact.Results) != len(cases) {
		fmt.Printf("ERROR: baseline artifact 结果数量 (%d) 与 holdout (%d) 不匹配\n",
			len(artifact.Results), len(cases))
		os.Exit(1)
	}
	for i, r := range artifact.Results {
		if r.CaseID != cases[i].ID {
			fmt.Printf("ERROR: baseline artifact case_id 不匹配 (index %d: %s vs %s)\n",
				i, r.CaseID, cases[i].ID)
			os.Exit(1)
		}
	}

	fmt.Println("=== GoS vs Baseline 对比 (compare 模式) ===")
	fmt.Printf("Baseline 来源: %s\n", baselineFile)
	fmt.Printf("Baseline commit: %s\n", artifact.Commit)
	fmt.Printf("Baseline model: %s\n", artifact.Model)
	fmt.Printf("Baseline 时间: %s\n", artifact.Timestamp)
	fmt.Println()

	// 跑 GoS
	evalProfile := gosProfile == "eval"
	engine, cfg := buildGoSEngine(evalProfile)
	fmt.Printf("GoS profile: %s\n", gosProfile)
	fmt.Printf("GoS 配置: SessionMaxSteps=%d, GapDelta=%.2f, MinSupport=%d, MinConfidence=%.2f\n",
		cfg.SessionMaxSteps, cfg.FSM.GapDelta, cfg.FSM.MinSupport, cfg.FSM.MinConfidence)

	runner := eval.NewRunner(engine)
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

	// Gate check
	gateReport := eval.CheckGate(gosMetrics, artifact.Metrics)

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

	resultJSON, err := json.MarshalIndent(map[string]interface{}{
		"mode":            "compare",
		"commit":          gitCommit(),
		"baseline_commit": artifact.Commit,
		"baseline_model":  artifact.Model,
		"gos_metrics":     gosMetrics,
		"gos_results":     gosResults,
		"baseline":        artifact,
		"gate":            gateReport,
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

// ---------------------------------------------------------------------------
// smoke: 确定性 baseline，仅用于开发回归
// ---------------------------------------------------------------------------

func runSmoke(holdoutPath, outputFile string) {
	fmt.Println("=== Smoke 评测 (smoke 模式) ===")
	fmt.Println("注意: baseline 是确定性模拟，仅用于开发回归，不能作为 Phase 3 gate")

	engine, _ := buildGoSEngine(true) // smoke always uses eval profile
	runner := eval.NewRunner(engine)
	start := time.Now()
	gosMetrics, gosResults, err := runner.RunFromFile(context.Background(), holdoutPath)
	if err != nil {
		fmt.Printf("GoS 评测失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("GoS 耗时: %v\n", time.Since(start))

	printMetrics("GoS", gosMetrics)
	printDetails(gosResults)

	// Smoke baseline
	smoke := newSmokeBaselineRunner()
	cases, err := eval.LoadCases(holdoutPath)
	if err != nil {
		fmt.Printf("加载 holdout 失败: %v\n", err)
		os.Exit(1)
	}

	smokeMetrics := eval.NewEvalMetrics()
	var smokeResults []eval.EvalResult
	for _, c := range cases {
		r := smoke.runCase(context.Background(), c)
		smokeMetrics.AddResult(r)
		smokeResults = append(smokeResults, *r)
	}
	smokeMetrics.Finalize()

	printMetrics("Smoke Baseline (确定性模拟)", smokeMetrics)
	printDetails(smokeResults)

	gateReport := eval.CheckGate(gosMetrics, smokeMetrics)

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
