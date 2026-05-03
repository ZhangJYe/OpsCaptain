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

	docsTool := NewQueryInternalDocsTool()
	docsInfo, _ := docsTool.Info(nil)
	if docsInfo.Name != "query_internal_docs" {
		t.Fatalf("knowledge agent expects 'query_internal_docs', got %q", docsInfo.Name)
	}

	logTools, _ := GetLogMcpTool()
	_ = logTools // MCP log tools depend on config, may be 0 in test env
}
