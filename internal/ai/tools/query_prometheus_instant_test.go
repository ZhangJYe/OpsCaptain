package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrometheusInstantQueryTool_SuccessVector(t *testing.T) {
	var gotPath string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		if r.URL.Query().Get("limit") != "20" {
			t.Fatalf("expected limit=20, got %q", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{
				"resultType":"vector",
				"result":[
					{
						"metric":{"__name__":"up","job":"backend"},
						"value":[1780212000,"1"]
					},
					{
						"metric":{"__name__":"up","job":"frontend"},
						"value":[1780212000,"0"]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusInstantQueryTool()
	out, err := tool.InvokableRun(nil, `{"query":"up","time":"2026-05-31T10:00:00Z"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusInstantOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !parsed.Success || parsed.Degraded {
		t.Fatalf("expected success output, got %s", out)
	}
	if gotPath != "/api/v1/query" {
		t.Fatalf("expected query path, got %q", gotPath)
	}
	if gotQuery != "up" {
		t.Fatalf("unexpected query param %q", gotQuery)
	}
	if parsed.ResultType != "vector" {
		t.Fatalf("expected vector result type, got %q", parsed.ResultType)
	}
	if parsed.Summary == nil || parsed.Summary.SeriesCount != 2 || parsed.Summary.NumericCount != 2 {
		t.Fatalf("unexpected summary: %+v", parsed.Summary)
	}
	if parsed.Summary.Min != 0 || parsed.Summary.Max != 1 || parsed.Summary.Avg != 0.5 {
		t.Fatalf("unexpected summary values: %+v", parsed.Summary)
	}
	if len(parsed.Evidence) != 2 || parsed.Evidence[0].Value == nil || *parsed.Evidence[0].Value != 1 {
		t.Fatalf("unexpected evidence: %+v", parsed.Evidence)
	}
}

func TestPrometheusInstantQueryTool_SuccessScalar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{
				"resultType":"scalar",
				"result":[1780212000,"42"]
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusInstantQueryTool()
	out, err := tool.InvokableRun(nil, `{"query":"scalar(42)","time":"2026-05-31T10:00:00Z"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusInstantOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !parsed.Success || parsed.Scalar == nil || parsed.Scalar.Value == nil {
		t.Fatalf("expected scalar success output, got %s", out)
	}
	if *parsed.Scalar.Value != 42 {
		t.Fatalf("expected scalar 42, got %+v", parsed.Scalar.Value)
	}
	if len(parsed.Evidence) != 1 || parsed.Evidence[0].Value == nil || *parsed.Evidence[0].Value != 42 {
		t.Fatalf("unexpected evidence: %+v", parsed.Evidence)
	}
}

func TestPrometheusInstantQueryTool_DegradedOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusInstantQueryTool()
	out, err := tool.InvokableRun(nil, `{"query":"up","time":"2026-05-31T10:00:00Z"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusInstantOutput
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
