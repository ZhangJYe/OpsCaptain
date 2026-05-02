package events

import (
	"context"
	"strings"
	"testing"
)

func TestSchemaGate_HasAnswer(t *testing.T) {
	gate := NewSchemaGate()

	r := gate.Validate("hi")
	if r.Passed {
		t.Error("expected fail for short output")
	}
	found := false
	for _, c := range r.Checks {
		if c.Field == "has_answer" && !c.Passed {
			found = true
		}
	}
	if !found {
		t.Error("expected has_answer to fail")
	}

	r = gate.Validate("payment_service 的 P99 延迟是 2300ms，需要关注连接池状态")
	if !r.Passed {
		t.Errorf("expected pass, got: %s", r.Summary())
	}
}

func TestSchemaGate_NoContradiction(t *testing.T) {
	gate := NewSchemaGate()

	r := gate.Validate("系统状态正常，但存在异常错误率升高，需要排查故障原因")
	found := false
	for _, c := range r.Checks {
		if c.Field == "no_contradiction" && !c.Passed {
			found = true
		}
	}
	if !found {
		t.Error("expected contradiction warning")
	}
}

func TestSchemaGate_Actionable(t *testing.T) {
	gate := NewSchemaGate()

	r := gate.Validate("payment_service 的 P99 延迟升高到 2300ms，错误率 5.2%")
	found := false
	for _, c := range r.Checks {
		if c.Field == "actionable" && !c.Passed {
			found = true
		}
	}
	if !found {
		t.Error("expected actionable warning for problem without recommendation")
	}

	r = gate.Validate("payment_service 的 P99 延迟升高到 2300ms，建议检查连接池配置")
	for _, c := range r.Checks {
		if c.Field == "actionable" && !c.Passed {
			t.Errorf("should pass when recommendation present: %s", c.Detail)
		}
	}
}

func TestSchemaGate_AllPass(t *testing.T) {
	gate := NewSchemaGate()

	r := gate.Validate("payment_service 的 P99 延迟是 2300ms，错误率 5.2%。建议排查数据库连接池和下游依赖。")
	if !r.Passed {
		t.Errorf("expected all pass, got: %s", r.Summary())
	}
}

func TestSchemaGate_Summary(t *testing.T) {
	gate := NewSchemaGate()

	r := gate.Validate("hi")
	s := r.Summary()
	if !strings.Contains(s, "FAILED") {
		t.Errorf("expected FAILED in summary: %s", s)
	}

	r = gate.Validate("payment_service 正常运行，所有指标稳定")
	s = r.Summary()
	if !strings.Contains(s, "passed") {
		t.Errorf("expected passed in summary: %s", s)
	}
}

func TestSchemaGateCollector(t *testing.T) {
	c := NewSchemaGateCollector()
	if c.ToolCallCount() != 0 {
		t.Error("expected 0 tool calls initially")
	}

	c.Emit(context.Background(), AgentEvent{Type: EventToolCallEnd, Payload: map[string]any{}})
	c.Emit(context.Background(), AgentEvent{Type: EventToolCallEnd, Payload: map[string]any{}})
	if c.ToolCallCount() != 2 {
		t.Errorf("expected 2 tool calls, got %d", c.ToolCallCount())
	}
}
