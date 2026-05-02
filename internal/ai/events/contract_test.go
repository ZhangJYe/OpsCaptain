package events

import (
	"context"
	"strings"
	"testing"
)

func TestContract_MustUseTools(t *testing.T) {
	r := ValidateContract("payment_service 延迟升高", nil, nil, false)
	if r.Passed {
		t.Error("expected violation for no tool calls")
	}
	found := false
	for _, v := range r.Violations {
		if v.Rule == "must_use_tools" {
			found = true
		}
	}
	if !found {
		t.Error("expected must_use_tools violation")
	}
}

func TestContract_MustReportFailure(t *testing.T) {
	r := ValidateContract("payment_service 一切正常", nil, []string{"query_metrics"}, true)
	if r.Passed {
		t.Error("expected violation for unreported failure")
	}
	found := false
	for _, v := range r.Violations {
		if v.Rule == "must_report_failure" && strings.Contains(v.Detail, "query_metrics") {
			found = true
		}
	}
	if !found {
		t.Error("expected must_report_failure violation for query_metrics")
	}
}

func TestContract_ReportedFailure_Passes(t *testing.T) {
	r := ValidateContract("query_metrics 工具调用失败，无法获取数据", nil, []string{"query_metrics"}, true)
	for _, v := range r.Violations {
		if v.Rule == "must_report_failure" {
			t.Errorf("should not report failure violation when failure is mentioned: %s", v.Detail)
		}
	}
}

func TestContract_ShouldReferenceData(t *testing.T) {
	toolResults := []string{"payment_service P99 延迟 2300ms，错误率 5.2%"}
	r := ValidateContract("系统运行正常，一切良好", toolResults, nil, true)
	found := false
	for _, v := range r.Violations {
		if v.Rule == "should_reference_data" {
			found = true
		}
	}
	if !found {
		t.Error("expected should_reference_data warning")
	}
}

func TestContract_ReferencesData_Passes(t *testing.T) {
	toolResults := []string{"payment_service P99 延迟 2300ms，错误率 5.2%"}
	r := ValidateContract("payment_service 的 P99 延迟是 2300ms，错误率 5.2%，需要关注", toolResults, nil, true)
	for _, v := range r.Violations {
		if v.Rule == "should_reference_data" {
			t.Errorf("should not warn when data is referenced: %s", v.Detail)
		}
	}
}

func TestContract_NoFabricatedAlerts(t *testing.T) {
	r := ValidateContract("发现告警: connection_pool_exhausted 触发于 10:30", nil, nil, true)
	if r.Passed {
		t.Error("expected violation for fabricated alert")
	}
	found := false
	for _, v := range r.Violations {
		if v.Rule == "no_fabricated_alerts" {
			found = true
		}
	}
	if !found {
		t.Error("expected no_fabricated_alerts violation")
	}
}

func TestContract_ShouldHedge(t *testing.T) {
	r := ValidateContract("根因是数据库连接池满", nil, nil, true)
	found := false
	for _, v := range r.Violations {
		if v.Rule == "should_hedge" {
			found = true
		}
	}
	if !found {
		t.Error("expected should_hedge warning for confident claim without data")
	}
}

func TestContract_HedgePresent_Passes(t *testing.T) {
	r := ValidateContract("推测根因可能是数据库连接池满，需要进一步确认", nil, nil, true)
	for _, v := range r.Violations {
		if v.Rule == "should_hedge" {
			t.Errorf("should not warn when hedge words present: %s", v.Detail)
		}
	}
}

func TestContract_AllPass(t *testing.T) {
	toolResults := []string{"payment_service P99 延迟 2300ms"}
	r := ValidateContract("payment_service P99 延迟 2300ms，建议排查", toolResults, nil, true)
	if !r.Passed {
		t.Errorf("expected passed, got: %s", r.Summary())
	}
}

func TestContractCollector(t *testing.T) {
	cc := NewContractCollector()

	cc.Emit(context.Background(), AgentEvent{Type: EventToolCallEnd, Payload: map[string]any{
		"tool_name": "query_metrics", "summary": "P99: 2300ms",
	}})
	cc.Emit(context.Background(), AgentEvent{Type: EventToolCallEnd, Payload: map[string]any{
		"tool_name": "search_logs", "error": "timeout",
	}})

	if !cc.HasToolCalls() {
		t.Error("expected hasToolCalls=true")
	}
	if len(cc.ToolResults()) != 1 || cc.ToolResults()[0] != "P99: 2300ms" {
		t.Errorf("unexpected tool results: %v", cc.ToolResults())
	}
	if len(cc.FailedTools()) != 1 || cc.FailedTools()[0] != "search_logs" {
		t.Errorf("unexpected failed tools: %v", cc.FailedTools())
	}
}

func TestContractResult_Summary(t *testing.T) {
	r := &ContractResult{Passed: true}
	if r.Summary() != "contract: passed" {
		t.Errorf("unexpected summary: %s", r.Summary())
	}

	r.AddWarn("test_rule", "test detail")
	if !strings.Contains(r.Summary(), "passed with warnings") {
		t.Errorf("unexpected summary: %s", r.Summary())
	}

	r.AddViolation("test_rule2", "test detail 2")
	if !strings.Contains(r.Summary(), "FAILED") {
		t.Errorf("unexpected summary: %s", r.Summary())
	}
}
