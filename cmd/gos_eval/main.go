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
	"SuperBizAgent/internal/ai/belief"
)

type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[INFO] %s %v\n", msg, keysAndValues)
}

func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {
	fmt.Printf("[ERROR] %s %v\n", msg, keysAndValues)
}

type mockExpert struct {
	name string
}

func newMockExpert(name string) *mockExpert {
	return &mockExpert{name: name}
}

func (m *mockExpert) Name() string {
	return m.name
}

func (m *mockExpert) Run(ctx context.Context, frontier *belief.Frontier, graph *belief.BeliefGraph) *experts.ExpertAnalysis {
	symptomNode := graph.Nodes[graph.StartSignalID]
	signalText := ""
	if symptomNode != nil {
		signalText = symptomNode.Label
	}

	analysis := "需要进一步诊断"
	confidence := 0.7
	evidence := []experts.EvidenceItem{
		{SourceType: "log", SourceID: "default-1", Title: "症状分析", Snippet: frontier.Why, Score: 0.7},
	}

	if contains(signalText, "CPU") {
		analysis = "CPU 资源耗尽导致服务超时"
		confidence = 0.85
		evidence = []experts.EvidenceItem{
			{SourceType: "metric", SourceID: "cpu-1", Title: "CPU 使用率 95%", Snippet: "CPU usage 95%", Score: 1.0},
			{SourceType: "metric", SourceID: "mem-1", Title: "内存使用率 80%", Snippet: "Memory usage 80%", Score: 0.8},
		}
	} else if contains(signalText, "数据库") || contains(signalText, "连接池") {
		analysis = "数据库连接池耗尽"
		confidence = 0.9
		evidence = []experts.EvidenceItem{
			{SourceType: "log", SourceID: "db-1", Title: "连接池已满", Snippet: "Connection pool exhausted", Score: 1.0},
			{SourceType: "metric", SourceID: "db-2", Title: "慢查询增加", Snippet: "Slow queries increased", Score: 0.9},
		}
	} else if contains(signalText, "网络") || contains(signalText, "跨区域") {
		analysis = "网络链路问题"
		confidence = 0.75
		evidence = []experts.EvidenceItem{
			{SourceType: "metric", SourceID: "net-1", Title: "跨区域延迟升高", Snippet: "Cross-region latency high", Score: 1.0},
		}
	} else if contains(signalText, "缓存") || contains(signalText, "Redis") {
		analysis = "缓存失效导致后端压力"
		confidence = 0.8
		evidence = []experts.EvidenceItem{
			{SourceType: "metric", SourceID: "cache-1", Title: "缓存命中率下降", Snippet: "Cache hit rate decreased", Score: 1.0},
		}
	} else if contains(signalText, "Kafka") || contains(signalText, "消息堆积") {
		analysis = "Kafka 消费者处理能力不足"
		confidence = 0.85
		evidence = []experts.EvidenceItem{
			{SourceType: "metric", SourceID: "kafka-1", Title: "消费延迟增加", Snippet: "Consumer lag increased", Score: 1.0},
			{SourceType: "log", SourceID: "kafka-2", Title: "消息堆积", Snippet: "Messages堆积", Score: 0.9},
		}
	}

	return &experts.ExpertAnalysis{
		ExpertName: m.name,
		Analysis:   analysis,
		Confidence: confidence,
		Status:     "succeeded",
		Evidence:   evidence,
		ToolCalls:  1,
		RAGCalls:   0,
		LLMCalls:   2,
	}
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

func main() {
	cfg := gos_engine.DefaultConfig()
	cfg.SessionMaxSteps = 3
	cfg.FSM.GapDelta = 0.2
	cfg.FSM.MinSupport = 1
	cfg.FSM.MaxSteps = 3

	logger := &testLogger{}
	engine := gos_engine.NewGoSEngine(cfg, logger)

	engine.RegisterExpert("linux_sre", newMockExpert("linux_sre"))
	engine.RegisterExpert("network_sre", newMockExpert("network_sre"))
	engine.RegisterExpert("database_sre", newMockExpert("database_sre"))

	runner := eval.NewRunner(engine)

	fmt.Println("=== GoS Engine 评测 ===")
	fmt.Printf("配置: SessionMaxSteps=%d, GapDelta=%.2f, MinSupport=%d\n",
		cfg.SessionMaxSteps, cfg.FSM.GapDelta, cfg.FSM.MinSupport)
	fmt.Printf("专家: linux_sre, network_sre, database_sre\n")
	fmt.Println()

	start := time.Now()
	metrics, results, err := runner.RunFromFile(context.Background(), "internal/ai/agent/gos_engine/eval/testdata/holdout.json")
	if err != nil {
		fmt.Printf("评测失败: %v\n", err)
		os.Exit(1)
	}
	duration := time.Since(start)

	fmt.Printf("评测完成，耗时: %v\n", duration)
	fmt.Println()

	fmt.Println("=== 评测结果 ===")
	fmt.Printf("总用例数: %d\n", metrics.TotalCases)
	fmt.Printf("成功: %d\n", metrics.Succeeded)
	fmt.Printf("降级: %d\n", metrics.Degraded)
	fmt.Printf("失败: %d\n", metrics.Failed)
	fmt.Printf("准确率: %.2f%%\n", metrics.Accuracy*100)
	fmt.Printf("证据覆盖率: %.2f%%\n", metrics.EvidenceCoverage*100)
	fmt.Printf("平均延迟: %v\n", metrics.AvgLatency)
	fmt.Printf("平均 LLM 调用: %.1f\n", metrics.AvgLLMCalls)
	fmt.Printf("降级率: %.2f%%\n", metrics.DegradationRate*100)
	fmt.Printf("可追溯性: %.2f%%\n", metrics.Traceability*100)
	fmt.Println()

	fmt.Println("=== 详细结果 ===")
	for _, r := range results {
		fmt.Printf("用例 %s:\n", r.CaseID)
		fmt.Printf("  症状: %s\n", r.Symptom)
		fmt.Printf("  预测: %s\n", r.Prediction)
		fmt.Printf("  真实: %s\n", r.GroundTruth)
		fmt.Printf("  匹配: %v\n", r.Matched)
		fmt.Printf("  状态: %s\n", r.Status)
		fmt.Printf("  延迟: %v\n", r.Latency)
		fmt.Printf("  LLM 调用: %d\n", r.LLMCalls)
		fmt.Printf("  证据数: %d\n", r.EvidenceCount)
		fmt.Println()
	}

	baseline := &eval.EvalMetrics{
		Accuracy:         0.6,
		EvidenceCoverage: 0.8,
		AvgLatency:       10 * time.Second,
		AvgLLMCalls:      10.0,
		DegradationRate:  0.3,
		Traceability:     1.0,
	}

	gateReport := eval.CheckGate(metrics, baseline)

	fmt.Println("=== Gate 检查 ===")
	fmt.Printf("基准: Accuracy>=%.0f%%, Coverage>=%.0f%%, Latency<=%v, LLMCalls<=%.0f, Degradation<=%.0f%%, Traceability=100%%\n",
		baseline.Accuracy*100, baseline.EvidenceCoverage*100, baseline.AvgLatency*3/2, baseline.AvgLLMCalls*2, baseline.DegradationRate*100)
	fmt.Println()

	for _, g := range gateReport.Gates {
		status := "✓ PASS"
		if !g.Passed {
			status = "✗ FAIL"
		}
		fmt.Printf("  %s %s: expected %s, actual %s\n", status, g.Name, g.Expected, g.Actual)
	}

	fmt.Println()
	if gateReport.AllPassed {
		fmt.Println("✓ 所有 Gate 通过，可以进入灰度阶段")
	} else {
		fmt.Println("✗ 部分 Gate 未通过，需要继续优化")
	}

	resultJSON, _ := json.MarshalIndent(map[string]interface{}{
		"metrics": metrics,
		"results": results,
		"gate":    gateReport,
	}, "", "  ")
	os.WriteFile("eval_result.json", resultJSON, 0644)
	fmt.Println("\n详细结果已保存到 eval_result.json")
}
