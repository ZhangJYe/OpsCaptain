package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrometheusMetricsDiscoveryTool_ListMetricNames(t *testing.T) {
	var gotPath string
	var gotStart string
	var gotEnd string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotStart = r.URL.Query().Get("start")
		gotEnd = r.URL.Query().Get("end")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":["up","http_requests_total","process_cpu_seconds_total"]
		}`))
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusMetricsDiscoveryTool()
	out, err := tool.InvokableRun(nil, `{"keyword":"http","start":"2026-05-31T10:00:00Z","end":"2026-05-31T10:30:00Z","limit":10}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusDiscoveryOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !parsed.Success || parsed.Degraded {
		t.Fatalf("expected success output, got %s", out)
	}
	if gotPath != "/api/v1/label/__name__/values" {
		t.Fatalf("expected label values path, got %q", gotPath)
	}
	if gotStart == "" || gotEnd == "" {
		t.Fatalf("expected start and end query params, got start=%q end=%q", gotStart, gotEnd)
	}
	if len(parsed.Metrics) != 1 || parsed.Metrics[0] != "http_requests_total" {
		t.Fatalf("unexpected metrics: %+v", parsed.Metrics)
	}
	if parsed.Summary == nil || parsed.Summary.MetricCount != 1 {
		t.Fatalf("unexpected summary: %+v", parsed.Summary)
	}
	if len(parsed.Evidence) != 1 || parsed.Evidence[0].Metric != "http_requests_total" {
		t.Fatalf("unexpected evidence: %+v", parsed.Evidence)
	}
}

func TestPrometheusMetricsDiscoveryTool_ListSeriesLabels(t *testing.T) {
	var gotPath string
	var gotMatch string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMatch = r.URL.Query().Get("match[]")
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":[
				{"__name__":"http_requests_total","service":"checkout","status":"200","job":"backend"},
				{"__name__":"http_requests_total","service":"checkout","status":"500","job":"backend"},
				{"__name__":"http_requests_total","service":"payment","status":"500","job":"backend"}
			]
		}`))
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusMetricsDiscoveryTool()
	out, err := tool.InvokableRun(nil, `{"match":"http_requests_total","keyword":"checkout","start":"2026-05-31T10:00:00Z","end":"2026-05-31T10:30:00Z","limit":10}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusDiscoveryOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !parsed.Success || parsed.Degraded {
		t.Fatalf("expected success output, got %s", out)
	}
	if gotPath != "/api/v1/series" {
		t.Fatalf("expected series path, got %q", gotPath)
	}
	if gotMatch != "http_requests_total" {
		t.Fatalf("expected match[] param, got %q", gotMatch)
	}
	if parsed.Summary == nil || parsed.Summary.SeriesCount != 2 || parsed.Summary.MetricCount != 1 {
		t.Fatalf("unexpected summary: %+v", parsed.Summary)
	}
	if !stringSliceContains(parsed.LabelKeys, "service") || !stringSliceContains(parsed.LabelKeys, "status") {
		t.Fatalf("expected label keys service/status, got %+v", parsed.LabelKeys)
	}
	if values := parsed.LabelValues["status"]; !stringSliceContains(values, "200") || !stringSliceContains(values, "500") {
		t.Fatalf("expected status label values, got %+v", values)
	}
	if len(parsed.Evidence) != 1 || parsed.Evidence[0].Metric != "http_requests_total" {
		t.Fatalf("unexpected evidence: %+v", parsed.Evidence)
	}
}

func TestPrometheusMetricsDiscoveryTool_DegradedOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusMetricsDiscoveryTool()
	out, err := tool.InvokableRun(nil, `{"keyword":"http","start":"2026-05-31T10:00:00Z","end":"2026-05-31T10:30:00Z"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusDiscoveryOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if parsed.Success || !parsed.Degraded {
		t.Fatalf("expected degraded output, got %s", out)
	}
	if !strings.Contains(parsed.Error, "HTTP 502") {
		t.Fatalf("expected HTTP 502 error, got %q", parsed.Error)
	}
}

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
