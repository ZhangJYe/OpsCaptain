package events

import (
	"context"
	"testing"
	"time"
)

func TestHealthCollector_BasicReport(t *testing.T) {
	hc := NewHealthCollector(100)

	// 模拟 3 次成功调用
	for i := 0; i < 3; i++ {
		hc.Emit(context.Background(), AgentEvent{
			Type: EventToolCallEnd,
			Payload: map[string]any{
				"tool_name":   "query_metrics",
				"duration_ms": int64(100 + i*50),
				"success":     true,
			},
			Timestamp: time.Now(),
		})
	}

	report := hc.Report("query_metrics")
	if report == nil {
		t.Fatal("expected report, got nil")
	}
	if report.TotalCalls != 3 {
		t.Fatalf("expected 3 calls, got %d", report.TotalCalls)
	}
	if report.SuccessCount != 3 {
		t.Fatalf("expected 3 successes, got %d", report.SuccessCount)
	}
	if report.FailCount != 0 {
		t.Fatalf("expected 0 failures, got %d", report.FailCount)
	}
	if report.SuccessRate != 1.0 {
		t.Fatalf("expected 1.0 success rate, got %f", report.SuccessRate)
	}
}

func TestHealthCollector_WithError(t *testing.T) {
	hc := NewHealthCollector(100)

	// 2 次成功 + 1 次失败
	hc.Emit(context.Background(), AgentEvent{
		Type: EventToolCallEnd,
		Payload: map[string]any{
			"tool_name":   "query_logs",
			"duration_ms": int64(200),
			"success":     true,
		},
		Timestamp: time.Now(),
	})
	hc.Emit(context.Background(), AgentEvent{
		Type: EventToolCallEnd,
		Payload: map[string]any{
			"tool_name":   "query_logs",
			"duration_ms": int64(5000),
			"success":     false,
			"error":       "connection timeout",
		},
		Timestamp: time.Now(),
	})
	hc.Emit(context.Background(), AgentEvent{
		Type: EventToolCallEnd,
		Payload: map[string]any{
			"tool_name":   "query_logs",
			"duration_ms": int64(150),
			"success":     true,
		},
		Timestamp: time.Now(),
	})

	report := hc.Report("query_logs")
	if report == nil {
		t.Fatal("expected report, got nil")
	}
	if report.TotalCalls != 3 {
		t.Fatalf("expected 3 calls, got %d", report.TotalCalls)
	}
	if report.SuccessCount != 2 {
		t.Fatalf("expected 2 successes, got %d", report.SuccessCount)
	}
	if report.FailCount != 1 {
		t.Fatalf("expected 1 failure, got %d", report.FailCount)
	}
	if report.SuccessRate < 0.66 || report.SuccessRate > 0.67 {
		t.Fatalf("expected ~0.67 success rate, got %f", report.SuccessRate)
	}
	if len(report.CommonErrors) != 1 || report.CommonErrors[0] != "connection timeout" {
		t.Fatalf("expected ['connection timeout'], got %v", report.CommonErrors)
	}
}

func TestHealthCollector_Reports(t *testing.T) {
	hc := NewHealthCollector(100)

	hc.Emit(context.Background(), AgentEvent{
		Type:      EventToolCallEnd,
		Payload:   map[string]any{"tool_name": "tool_a", "duration_ms": int64(100), "success": true},
		Timestamp: time.Now(),
	})
	hc.Emit(context.Background(), AgentEvent{
		Type:      EventToolCallEnd,
		Payload:   map[string]any{"tool_name": "tool_b", "duration_ms": int64(200), "success": true},
		Timestamp: time.Now(),
	})

	reports := hc.Reports()
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports, got %d", len(reports))
	}
	// 按名称排序
	if reports[0].ToolName != "tool_a" || reports[1].ToolName != "tool_b" {
		t.Fatalf("expected sorted by name, got %s, %s", reports[0].ToolName, reports[1].ToolName)
	}
}

func TestHealthCollector_IgnoresNonToolEvents(t *testing.T) {
	hc := NewHealthCollector(100)

	hc.Emit(context.Background(), AgentEvent{
		Type:      EventModelEnd,
		Payload:   map[string]any{"model_name": "deepseek"},
		Timestamp: time.Now(),
	})
	hc.Emit(context.Background(), AgentEvent{
		Type:      EventToolCallStart,
		Payload:   map[string]any{"tool_name": "test"},
		Timestamp: time.Now(),
	})

	reports := hc.Reports()
	if len(reports) != 0 {
		t.Fatalf("expected 0 reports, got %d", len(reports))
	}
}

func TestHealthCollector_MaxPer(t *testing.T) {
	hc := NewHealthCollector(5)

	for i := 0; i < 10; i++ {
		hc.Emit(context.Background(), AgentEvent{
			Type: EventToolCallEnd,
			Payload: map[string]any{
				"tool_name":   "test",
				"duration_ms": int64(i * 100),
				"success":     true,
			},
			Timestamp: time.Now(),
		})
	}

	report := hc.Report("test")
	if report.TotalCalls != 5 {
		t.Fatalf("expected 5 calls (maxPer), got %d", report.TotalCalls)
	}
}

func TestHealthCollector_Reset(t *testing.T) {
	hc := NewHealthCollector(100)

	hc.Emit(context.Background(), AgentEvent{
		Type:      EventToolCallEnd,
		Payload:   map[string]any{"tool_name": "test", "duration_ms": int64(100), "success": true},
		Timestamp: time.Now(),
	})

	hc.Reset()
	reports := hc.Reports()
	if len(reports) != 0 {
		t.Fatalf("expected 0 reports after reset, got %d", len(reports))
	}
}

func TestPercentile(t *testing.T) {
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
	}

	p50 := percentile(durations, 0.5)
	if p50 != 30*time.Millisecond {
		t.Fatalf("expected 30ms, got %v", p50)
	}

	p95 := percentile(durations, 0.95)
	if p95 != 50*time.Millisecond {
		t.Fatalf("expected 50ms, got %v", p95)
	}
}
