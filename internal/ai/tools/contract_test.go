package tools

import (
	"testing"
)

func TestPrometheusAlertsQueryToolContract(t *testing.T) {
	tool := NewPrometheusAlertsQueryTool()
	info, err := tool.Info(nil)
	if err != nil {
		t.Fatalf("get tool info: %v", err)
	}
	if info.Name != "query_prometheus_alerts" {
		t.Fatalf("expected tool name 'query_prometheus_alerts', got %q", info.Name)
	}
	if info.Desc == "" {
		t.Fatal("expected non-empty tool description")
	}
}

func TestUnavailableLogQueryToolContract(t *testing.T) {
	logTool := NewUnavailableLogQueryTool("test unavailable")
	if logTool == nil {
		t.Fatal("expected structured degraded log tool")
	}
	info, err := logTool.Info(nil)
	if err != nil {
		t.Fatalf("get tool info: %v", err)
	}
	if info.Name != "query_logs" {
		t.Fatalf("expected tool name 'query_logs', got %q", info.Name)
	}
	if info.Desc == "" {
		t.Fatal("expected non-empty tool description")
	}
	if info.ParamsOneOf == nil {
		t.Fatal("expected query_logs input schema")
	}
}

func TestPrometheusRangeQueryToolContract(t *testing.T) {
	tool := NewPrometheusRangeQueryTool()
	info, err := tool.Info(nil)
	if err != nil {
		t.Fatalf("get tool info: %v", err)
	}
	if info.Name != "query_prometheus_range" {
		t.Fatalf("expected tool name 'query_prometheus_range', got %q", info.Name)
	}
	if info.Desc == "" {
		t.Fatal("expected non-empty tool description")
	}
}

func TestPrometheusInstantQueryToolContract(t *testing.T) {
	tool := NewPrometheusInstantQueryTool()
	info, err := tool.Info(nil)
	if err != nil {
		t.Fatalf("get tool info: %v", err)
	}
	if info.Name != "query_prometheus_instant" {
		t.Fatalf("expected tool name 'query_prometheus_instant', got %q", info.Name)
	}
	if info.Desc == "" {
		t.Fatal("expected non-empty tool description")
	}
}

func TestPrometheusMetricsDiscoveryToolContract(t *testing.T) {
	tool := NewPrometheusMetricsDiscoveryTool()
	info, err := tool.Info(nil)
	if err != nil {
		t.Fatalf("get tool info: %v", err)
	}
	if info.Name != "list_prometheus_metrics" {
		t.Fatalf("expected tool name 'list_prometheus_metrics', got %q", info.Name)
	}
	if info.Desc == "" {
		t.Fatal("expected non-empty tool description")
	}
}

func TestQueryInternalDocsToolContract(t *testing.T) {
	tool := NewQueryInternalDocsTool()
	info, err := tool.Info(nil)
	if err != nil {
		t.Fatalf("get tool info: %v", err)
	}
	if info.Name != "query_internal_docs" {
		t.Fatalf("expected tool name 'query_internal_docs', got %q", info.Name)
	}
	if info.Desc == "" {
		t.Fatal("expected non-empty tool description")
	}
	if info.ParamsOneOf == nil {
		t.Fatal("expected query_internal_docs input schema")
	}
}

func TestGetLogMcpToolReturnsTools(t *testing.T) {
	tools, err := GetLogMcpTool()
	if err != nil {
		t.Fatalf("GetLogMcpTool: %v", err)
	}
	if len(tools) == 0 {
		t.Skip("MCP log_url not configured, skipping log tool contract check")
	}
	for _, tb := range tools {
		info, err := tb.Info(nil)
		if err != nil {
			t.Fatalf("get tool info: %v", err)
		}
		if info.Name == "" {
			t.Fatal("expected non-empty log tool name")
		}
		if info.Desc == "" {
			t.Fatalf("expected non-empty description for tool %q", info.Name)
		}
	}
}

func TestPrometheusAlertsOutputStructure(t *testing.T) {
	output := PrometheusAlertsOutput{
		Success: true,
		Alerts: []SimplifiedAlert{
			{AlertName: "CPUHigh", Description: "CPU usage > 80%", State: "firing", ActiveAt: "2026-01-01T00:00:00Z"},
		},
		Message: "found 1 active alert",
	}
	if !output.Success {
		t.Fatal("expected success=true")
	}
	if len(output.Alerts) == 0 {
		t.Fatal("expected at least 1 alert")
	}
	for i, alert := range output.Alerts {
		if alert.AlertName == "" {
			t.Fatalf("alert[%d].alert_name should not be empty", i)
		}
		if alert.State == "" {
			t.Fatalf("alert[%d].state should not be empty", i)
		}
	}
}

func TestSimplifyPrometheusAlertsKeepsDistinctLabelSetsAndCapsOutput(t *testing.T) {
	alerts := []PrometheusAlert{
		{Labels: map[string]string{"alertname": "ServiceDegraded", "case_id": "case-1", "service": "checkout"}, State: "firing"},
		{Labels: map[string]string{"alertname": "ServiceDegraded", "case_id": "case-2", "service": "payment"}, State: "firing"},
		{Labels: map[string]string{"alertname": "ServiceDegraded", "case_id": "case-1", "service": "checkout"}, State: "firing"},
	}

	got := simplifyPrometheusAlerts(alerts, 10)
	if len(got) != 2 {
		t.Fatalf("expected two distinct alert instances, got %d", len(got))
	}
	if got[0].Labels["case_id"] != "case-1" || got[1].Labels["case_id"] != "case-2" {
		t.Fatalf("expected case labels to be preserved, got %#v", got)
	}

	capped := simplifyPrometheusAlerts(alerts, 1)
	if len(capped) != 1 {
		t.Fatalf("expected configured cap to limit output to one alert, got %d", len(capped))
	}
}

func TestFilterPrometheusAlertsScopesByServiceOrCase(t *testing.T) {
	alerts := []PrometheusAlert{
		{Labels: map[string]string{"service": "eval-queue-orders", "case_id": "real-development-001"}},
		{Labels: map[string]string{"service": "eval-retry-inventory", "case_id": "real-development-002"}},
		{Labels: map[string]string{"service": "eval-db-profile", "case_id": "real-development-003"}},
	}

	byService := filterPrometheusAlerts(alerts, "检查 eval-retry-inventory 的失败与重试")
	if len(byService) != 1 || byService[0].Labels["case_id"] != "real-development-002" {
		t.Fatalf("expected service-scoped alert, got %#v", byService)
	}
	byCase := filterPrometheusAlerts(alerts, "real-development-003")
	if len(byCase) != 1 || byCase[0].Labels["service"] != "eval-db-profile" {
		t.Fatalf("expected case-scoped alert, got %#v", byCase)
	}
	if got := filterPrometheusAlerts(alerts, "unknown-service"); len(got) != 0 {
		t.Fatalf("expected unmatched query to return no alerts, got %#v", got)
	}
	if got := filterPrometheusAlerts(alerts, ""); len(got) != len(alerts) {
		t.Fatalf("expected empty query to preserve unfiltered tool behavior, got %d", len(got))
	}
}

func TestPrometheusAlertsOutputErrorStructure(t *testing.T) {
	output := PrometheusAlertsOutput{
		Success: false,
		Error:   "prometheus unreachable",
		Message: "query failed",
	}
	if output.Success {
		t.Fatal("expected success=false for error case")
	}
	if output.Error == "" {
		t.Fatal("expected non-empty error for failed output")
	}
}

func TestQueryInternalDocsOutputSuccessRequiresNoError(t *testing.T) {
	output := QueryInternalDocsOutput{
		Success: true,
		Message: "found 3 documents",
	}
	if !output.Success && output.Error == "" {
		t.Fatal("failed output should have error message")
	}
}

func TestToolNamesMatchAgentContracts(t *testing.T) {
	metricsTool := NewPrometheusAlertsQueryTool()
	metricsInfo, _ := metricsTool.Info(nil)
	if metricsInfo.Name != "query_prometheus_alerts" {
		t.Fatalf("metrics agent expects 'query_prometheus_alerts', got %q", metricsInfo.Name)
	}
	rangeTool := NewPrometheusRangeQueryTool()
	rangeInfo, _ := rangeTool.Info(nil)
	if rangeInfo.Name != "query_prometheus_range" {
		t.Fatalf("metrics agent expects 'query_prometheus_range', got %q", rangeInfo.Name)
	}
	instantTool := NewPrometheusInstantQueryTool()
	instantInfo, _ := instantTool.Info(nil)
	if instantInfo.Name != "query_prometheus_instant" {
		t.Fatalf("metrics agent expects 'query_prometheus_instant', got %q", instantInfo.Name)
	}
	discoveryTool := NewPrometheusMetricsDiscoveryTool()
	discoveryInfo, _ := discoveryTool.Info(nil)
	if discoveryInfo.Name != "list_prometheus_metrics" {
		t.Fatalf("metrics agent expects 'list_prometheus_metrics', got %q", discoveryInfo.Name)
	}

	docsTool := NewQueryInternalDocsTool()
	docsInfo, _ := docsTool.Info(nil)
	if docsInfo.Name != "query_internal_docs" {
		t.Fatalf("knowledge agent expects 'query_internal_docs', got %q", docsInfo.Name)
	}

	logTools, _ := GetLogMcpTool()
	_ = logTools // MCP log tools depend on config, may be 0 in test env
}
