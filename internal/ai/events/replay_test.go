package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestReplayCase_PaymentServiceAlert 模拟: "支付服务最近有没有告警"
// 预期事件链: model_start → model_end → tool_start(metrics) → tool_end(metrics) → tool_start(logs) → tool_end(logs) → tool_start(knowledge) → tool_end(knowledge) → model_start → model_end
func TestReplayCase_PaymentServiceAlert(t *testing.T) {
	recorder := &sequenceRecorder{}
	hc := NewHealthCollector(100)
	emitter := NewMultiEmitter(recorder, hc) // 工具事件同时发给 recorder 和 hc

	// 1. 模型推理（triage）
	recorder.Emit(context.Background(), NewEvent(EventModelStart, "replay-1", "deepseek-v3", map[string]any{"model_name": "deepseek-v3"}))
	recorder.Emit(context.Background(), NewEvent(EventModelEnd, "replay-1", "deepseek-v3", map[string]any{"model_name": "deepseek-v3", "duration_ms": int64(1200), "total_tokens": 350, "success": true}))

	// 2. 查询 Prometheus 告警（事件来自真实 wrapper）
	metricsTool := WrapTool(&mockTool{name: "query_metrics", result: "payment_service: 3 alerts (P99 latency > 500ms)"}, emitter, "replay-1", nil, nil)
	metricsTool.InvokableRun(context.Background(), `{"service":"payment","time_range":"1h"}`)

	// 3. 查询日志（事件来自真实 wrapper）
	logsTool := WrapTool(&mockTool{name: "query_logs", result: "payment_service: 42 error logs (connection timeout)"}, emitter, "replay-1", nil, nil)
	logsTool.InvokableRun(context.Background(), `{"service":"payment","level":"error","time_range":"1h"}`)

	// 4. 检索知识库（事件来自真实 wrapper）
	docsTool := WrapTool(&mockTool{name: "query_internal_docs", result: "SOP-PAY-001: 支付超时处理流程"}, emitter, "replay-1", nil, nil)
	docsTool.InvokableRun(context.Background(), `{"query":"payment timeout"}`)

	// 5. 模型生成报告
	recorder.Emit(context.Background(), NewEvent(EventModelStart, "replay-1", "deepseek-v3", map[string]any{"model_name": "deepseek-v3"}))
	recorder.Emit(context.Background(), NewEvent(EventModelEnd, "replay-1", "deepseek-v3", map[string]any{"model_name": "deepseek-v3", "duration_ms": int64(2000), "total_tokens": 800, "success": true}))

	// 验证事件序列
	types := recorder.Types()
	expected := []AgentEventType{
		EventModelStart, EventModelEnd, // triage
		EventToolCallStart, EventToolCallEnd, // metrics (来自 wrapper)
		EventToolCallStart, EventToolCallEnd, // logs (来自 wrapper)
		EventToolCallStart, EventToolCallEnd, // knowledge (来自 wrapper)
		EventModelStart, EventModelEnd, // report
	}
	assertEventSequence(t, types, expected)

	// 验证健康度
	reports := hc.Reports()
	if len(reports) != 3 {
		t.Fatalf("expected 3 tool reports, got %d", len(reports))
	}
	for _, r := range reports {
		if r.SuccessRate != 1.0 {
			t.Errorf("tool %s: expected 100%% success rate, got %.1f%%", r.ToolName, r.SuccessRate*100)
		}
	}

	t.Logf("Replay case 'payment alert': %d events, 3 tools all successful", len(types))
}

// TestReplayCase_LogSearchFailure 模拟: 日志查询超时降级
// 预期: 日志工具失败后 agent 仍能生成降级报告
func TestReplayCase_LogSearchFailure(t *testing.T) {
	recorder := &sequenceRecorder{}
	hc := NewHealthCollector(100)
	emitter := NewMultiEmitter(recorder, hc)

	// 1. 模型推理
	recorder.Emit(context.Background(), NewEvent(EventModelStart, "replay-2", "deepseek-v3", nil))
	recorder.Emit(context.Background(), NewEvent(EventModelEnd, "replay-2", "deepseek-v3", map[string]any{"success": true}))

	// 2. 查询指标（成功，事件来自 wrapper）
	metricsTool := WrapTool(&mockTool{name: "query_metrics", result: "cpu: 95%"}, emitter, "replay-2", nil, nil)
	metricsTool.InvokableRun(context.Background(), `{}`)

	// 3. 查询日志（失败 - 超时，事件来自 wrapper）
	logsTool := WrapTool(&mockTool{name: "query_logs", err: errors.New("connection timeout")}, emitter, "replay-2", nil, nil)
	logsTool.InvokableRun(context.Background(), `{}`)

	// 4. 模型生成降级报告
	recorder.Emit(context.Background(), NewEvent(EventModelStart, "replay-2", "deepseek-v3", nil))
	recorder.Emit(context.Background(), NewEvent(EventModelEnd, "replay-2", "deepseek-v3", map[string]any{"success": true}))

	// 验证事件序列
	types := recorder.Types()
	expected := []AgentEventType{
		EventModelStart, EventModelEnd, // model
		EventToolCallStart, EventToolCallEnd, // metrics (成功)
		EventToolCallStart, EventToolCallEnd, // logs (失败)
		EventModelStart, EventModelEnd, // report
	}
	assertEventSequence(t, types, expected)

	// 验证 logs 失败事件
	var logsEndEvent *AgentEvent
	for i := range recorder.events {
		if recorder.events[i].Type == EventToolCallEnd {
			if recorder.events[i].Payload["tool_name"] == "query_logs" {
				logsEndEvent = &recorder.events[i]
				break
			}
		}
	}
	if logsEndEvent == nil {
		t.Fatal("expected logs tool_call_end event")
	}
	if logsEndEvent.Payload["success"] != false {
		t.Errorf("expected logs success=false, got %v", logsEndEvent.Payload["success"])
	}

	// 验证健康度
	logsReport := hc.Report("query_logs")
	if logsReport == nil {
		t.Fatal("expected logs report")
	}
	if logsReport.SuccessRate != 0 {
		t.Errorf("expected 0%% success rate for logs, got %.1f%%", logsReport.SuccessRate*100)
	}

	t.Logf("Replay case 'log failure': logs tool failed, metrics tool succeeded")
}

// TestReplayCase_AfterHookDesensitization 模拟: after hook 脱敏失败
// 预期: 工具成功但脱敏失败 → 返回空结果，事件无 summary
func TestReplayCase_AfterHookDesensitization(t *testing.T) {
	recorder := &sequenceRecorder{}
	hc := NewHealthCollector(100)
	multiEmitter := NewMultiEmitter(recorder, hc) // 同时发给 recorder 和 hc

	// 模型推理
	recorder.Emit(context.Background(), NewEvent(EventModelStart, "replay-3", "deepseek-v3", nil))
	recorder.Emit(context.Background(), NewEvent(EventModelEnd, "replay-3", "deepseek-v3", map[string]any{"success": true}))

	// 查询指标（成功，但 after hook 脱敏失败）
	desensitizeFail := func(ctx context.Context, toolName, args, result string, execErr error) (string, error) {
		return "", errors.New("contains PII, desensitization failed")
	}
	metricsTool := WrapTool(&mockTool{name: "query_metrics", result: "user john@ex.com: payment failed"}, multiEmitter, "replay-3", nil, desensitizeFail)
	result, err := metricsTool.InvokableRun(context.Background(), `{}`)

	// 验证：返回空结果，不泄露原始数据
	if err == nil {
		t.Fatal("expected error from desensitization failure")
	}
	if result != "" {
		t.Fatalf("expected empty result, got %q", result)
	}

	// 验证健康度：应标记为失败
	report := hc.Report("query_metrics")
	if report == nil {
		t.Fatal("expected metrics report")
	}
	if report.SuccessRate != 0 {
		t.Errorf("expected 0%% success rate, got %.1f%%", report.SuccessRate*100)
	}

	// 验证事件：recorder 收到了 tool_call_end，且无 summary
	var toolEndEvent *AgentEvent
	for i := range recorder.events {
		if recorder.events[i].Type == EventToolCallEnd {
			toolEndEvent = &recorder.events[i]
			break
		}
	}
	if toolEndEvent == nil {
		t.Fatal("expected recorder to receive tool_call_end event")
	}
	if toolEndEvent.Payload["summary"] != nil {
		t.Errorf("expected no summary on desensitization failure, got %v", toolEndEvent.Payload["summary"])
	}
	if toolEndEvent.Payload["after_error"] != true {
		t.Errorf("expected after_error true, got %v", toolEndEvent.Payload["after_error"])
	}

	t.Logf("Replay case 'desensitization': tool succeeded but after hook failed, result masked")
}

// TestReplayCase_GradualDegradation 模拟: 工具逐渐变慢
// 预期: P95 应反映慢请求
func TestReplayCase_GradualDegradation(t *testing.T) {
	hc := NewHealthCollector(100)

	// 模拟 10 次调用，前 8 次正常，后 2 次变慢
	durations := []int64{100, 120, 110, 130, 105, 115, 125, 108, 3000, 5000}
	for _, d := range durations {
		hc.Emit(context.Background(), AgentEvent{
			Type: EventToolCallEnd,
			Payload: map[string]any{
				"tool_name":   "query_metrics",
				"duration_ms": d,
				"success":     d < 4000, // 超过 4s 视为超时
			},
			Timestamp: time.Now(),
		})
	}

	report := hc.Report("query_metrics")
	if report == nil {
		t.Fatal("expected report")
	}

	// P95 应该接近 5000ms（第 10 个值）
	if report.P95DurationMs < 3000 {
		t.Errorf("expected P95 >= 3000ms, got %dms", report.P95DurationMs)
	}

	// 成功率应为 90%（9/10）
	if report.SuccessRate < 0.89 || report.SuccessRate > 0.91 {
		t.Errorf("expected ~90%% success rate, got %.1f%%", report.SuccessRate*100)
	}

	t.Logf("Replay case 'gradual degradation': P95=%dms, success_rate=%.1f%%",
		report.P95DurationMs, report.SuccessRate*100)
}

func assertEventSequence(t *testing.T, actual, expected []AgentEventType) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("expected %d events, got %d\nactual:   %v\nexpected: %v",
			len(expected), len(actual), actual, expected)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf("event[%d]: expected %s, got %s\nactual:   %v\nexpected: %v",
				i, expected[i], actual[i], actual, expected)
		}
	}
}
