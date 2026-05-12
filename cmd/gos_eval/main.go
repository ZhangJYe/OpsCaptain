package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/agent/gos_engine/eval"

	"github.com/cloudwego/eino/components/tool"
	einoschema "github.com/cloudwego/eino/schema"
)

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
// fake tools — shared between GoS experts and baseline runner
// ---------------------------------------------------------------------------

// keywordResponse pairs keyword with response, checked in order (most specific first)
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
// fake RAG — injected into real experts via RAGQueryFunc
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
// Plan-Execute-Replan baseline — same tools, deterministic plan→execute→analyze
// ---------------------------------------------------------------------------

type baselineRunner struct {
	logTool *fakeLogTool
	docTool *fakeInternalDocsTool
}

func newBaselineRunner() *baselineRunner {
	return &baselineRunner{
		logTool: newFakeLogTool(),
		docTool: newFakeInternalDocsTool(),
	}
}

// baselineAnalysis maps symptom keywords to baseline predictions.
// This simulates what a Plan-Execute-Replan pipeline would produce given
// the same tool outputs — a single-pass plan→execute→analyze with no
// graph-based hypothesis tracking.
func (b *baselineRunner) runCase(ctx context.Context, c eval.EvalCase) *eval.EvalResult {
	start := time.Now()

	// Plan: select tools based on symptom keywords
	planTools := []string{"query_logs", "query_internal_docs"}

	// Execute: call each tool
	var toolOutputs []string
	llmCalls := 0
	evidenceCount := 0

	for _, toolName := range planTools {
		var output string
		var err error
		switch toolName {
		case "query_logs":
			output, err = b.logTool.InvokableRun(ctx, c.Symptom)
		case "query_internal_docs":
			output, err = b.docTool.InvokableRun(ctx, c.Symptom)
		}
		llmCalls++ // one LLM call per tool selection in plan-execute
		if err == nil {
			toolOutputs = append(toolOutputs, output)
			evidenceCount++
		}
	}

	// Analyze: single-pass LLM analysis (simulated)
	llmCalls++ // final analysis call

	prediction := b.analyzeSymptom(c.Symptom, toolOutputs)

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

func (b *baselineRunner) analyzeSymptom(symptom string, toolOutputs []string) string {
	// Single-pass analysis: no graph, no hypothesis tracking, no drill-down.
	// Keyword matching on symptom, most specific first.
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

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	cfg := gos_engine.DefaultConfig()
	cfg.SessionMaxSteps = 3
	cfg.FSM.GapDelta = 0.2
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 3
	cfg.FSM.MinConfidence = 0.6

	logger := &testLogger{}
	engine := gos_engine.NewGoSEngine(cfg, logger)

	// --- shared fake tools + fake RAG ---
	toolReg := experts.NewToolRegistry()
	toolReg.Register("query_logs", newFakeLogTool())
	toolReg.Register("query_internal_docs", newFakeInternalDocsTool())

	ragFunc := experts.RAGQueryFunc(fakeRAGQuery)

	// --- real experts with injected dependencies ---
	linuxSRE := experts.NewLinuxSREExpert(experts.ExpertRuntimeConfig{
		Name:              "linux_sre",
		Description:       "Linux SRE expert",
		ToolNames:         []string{"query_logs", "query_internal_docs"},
		MaxRetrievalSteps: 3,
		RAGQueryFunc:      ragFunc,
	}, toolReg)

	networkSRE := experts.NewNetworkSREExpert(experts.ExpertRuntimeConfig{
		Name:              "network_sre",
		Description:       "Network SRE expert",
		ToolNames:         []string{"query_logs", "query_internal_docs"},
		MaxRetrievalSteps: 3,
		RAGQueryFunc:      ragFunc,
	}, toolReg)

	databaseSRE := experts.NewDatabaseSREExpert(experts.ExpertRuntimeConfig{
		Name:              "database_sre",
		Description:       "Database SRE expert",
		ToolNames:         []string{"query_logs", "query_internal_docs"},
		MaxRetrievalSteps: 3,
		RAGQueryFunc:      ragFunc,
	}, toolReg)

	engine.RegisterExpert("linux_sre", linuxSRE)
	engine.RegisterExpert("network_sre", networkSRE)
	engine.RegisterExpert("database_sre", databaseSRE)

	// --- GoS eval ---
	runner := eval.NewRunner(engine)

	fmt.Println("=== GoS Engine 评测 ===")
	fmt.Printf("配置: SessionMaxSteps=%d, GapDelta=%.2f, MinSupport=%d, MinConfidence=%.2f\n",
		cfg.SessionMaxSteps, cfg.FSM.GapDelta, cfg.FSM.MinSupport, cfg.FSM.MinConfidence)
	fmt.Printf("专家: linux_sre, network_sre, database_sre (真实 BaseExpert + fake tool/RAG)\n")
	fmt.Println()

	start := time.Now()
	gosMetrics, gosResults, err := runner.RunFromFile(context.Background(), "internal/ai/agent/gos_engine/eval/testdata/holdout.json")
	if err != nil {
		fmt.Printf("GoS 评测失败: %v\n", err)
		os.Exit(1)
	}
	gosDuration := time.Since(start)

	fmt.Printf("GoS 评测完成，耗时: %v\n", gosDuration)
	printMetrics("GoS", gosMetrics)
	printDetails(gosResults)

	// --- Plan-Execute-Replan baseline (same tools, no graph) ---
	fmt.Println("=== Plan-Execute-Replan Baseline (同工具, 无因果图) ===")
	baseline := newBaselineRunner()

	cases, err := eval.LoadCases("internal/ai/agent/gos_engine/eval/testdata/holdout.json")
	if err != nil {
		fmt.Printf("加载 holdout 失败: %v\n", err)
		os.Exit(1)
	}

	baselineMetrics := eval.NewEvalMetrics()
	var baselineResults []eval.EvalResult
	for _, c := range cases {
		r := baseline.runCase(context.Background(), c)
		baselineMetrics.AddResult(r)
		baselineResults = append(baselineResults, *r)
	}
	baselineMetrics.Finalize()

	printMetrics("Baseline", baselineMetrics)
	printDetails(baselineResults)

	// --- Gate check ---
	gateReport := eval.CheckGate(gosMetrics, baselineMetrics)

	fmt.Println("=== Gate 检查 (GoS vs 真实 Baseline) ===")
	for _, g := range gateReport.Gates {
		status := "✓ PASS"
		if !g.Passed {
			status = "✗ FAIL"
		}
		fmt.Printf("  %s %s: expected %s, actual %s\n", status, g.Name, g.Expected, g.Actual)
	}
	fmt.Println()

	if gateReport.AllPassed {
		fmt.Println("✓ 所有 Gate 通过，GoS 不劣于 Baseline，可以进入灰度阶段")
	} else {
		fmt.Println("✗ 部分 Gate 未通过，GoS 需要继续优化")
	}

	// --- save results ---
	resultJSON, _ := json.MarshalIndent(map[string]interface{}{
		"gos_metrics":      gosMetrics,
		"gos_results":      gosResults,
		"baseline_metrics": baselineMetrics,
		"baseline_results": baselineResults,
		"gate":             gateReport,
	}, "", "  ")
	os.WriteFile("eval_result.json", resultJSON, 0644)
	fmt.Println("\n详细结果已保存到 eval_result.json")

	if !gateReport.AllPassed {
		os.Exit(1)
	}
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
