package tools

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrometheusRangeQueryTool_Success(t *testing.T) {
	var gotPath string
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		if r.URL.Query().Get("step") != "60" {
			t.Fatalf("expected step=60, got %q", r.URL.Query().Get("step"))
		}
		if r.URL.Query().Get("limit") != "20" {
			t.Fatalf("expected limit=20, got %q", r.URL.Query().Get("limit"))
		}
		_, _ = w.Write([]byte(`{
			"status":"success",
			"data":{
				"resultType":"matrix",
				"result":[
					{
						"metric":{"__name__":"http_requests_total","service":"checkout"},
						"values":[[1780212000,"1"],[1780212060,"3"],[1780212120,"5"]]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusRangeQueryTool()
	out, err := tool.InvokableRun(nil, `{"query":"rate(http_requests_total[5m])","start":"2026-05-31T10:00:00Z","end":"2026-05-31T10:02:00Z","step":"60s"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusRangeOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !parsed.Success || parsed.Degraded {
		t.Fatalf("expected success output, got %s", out)
	}
	if gotPath != "/api/v1/query_range" {
		t.Fatalf("expected query_range path, got %q", gotPath)
	}
	if gotQuery != "rate(http_requests_total[5m])" {
		t.Fatalf("unexpected query param %q", gotQuery)
	}
	if parsed.Summary == nil {
		t.Fatal("expected summary")
	}
	if parsed.Summary.SeriesCount != 1 || parsed.Summary.PointsCount != 3 {
		t.Fatalf("unexpected summary counts: %+v", parsed.Summary)
	}
	if parsed.Summary.Min != 1 || parsed.Summary.Max != 5 || parsed.Summary.Avg != 3 || parsed.Summary.Last != 5 {
		t.Fatalf("unexpected summary values: %+v", parsed.Summary)
	}
	if parsed.Summary.Trend != "up" {
		t.Fatalf("expected trend up, got %q", parsed.Summary.Trend)
	}
	if len(parsed.Evidence) != 1 || parsed.Evidence[0].Trend != "up" {
		t.Fatalf("expected evidence trend up, got %+v", parsed.Evidence)
	}
}

func TestPrometheusRangeQueryTool_DegradedOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	t.Setenv("PROMETHEUS_ADDRESS", server.URL)
	tool := NewPrometheusRangeQueryTool()
	out, err := tool.InvokableRun(nil, `{"query":"up","start":"2026-05-31T10:00:00Z","end":"2026-05-31T10:02:00Z","step":"60s"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusRangeOutput
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

func TestPrometheusRangeQueryTool_DegradedOnTooWideRange(t *testing.T) {
	tool := NewPrometheusRangeQueryTool()
	out, err := tool.InvokableRun(nil, `{"query":"up","start":"2026-05-30T00:00:00Z","end":"2026-05-31T00:00:00Z","step":"60s"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}

	var parsed PrometheusRangeOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if parsed.Success || !parsed.Degraded {
		t.Fatalf("expected degraded output, got %s", out)
	}
	if !strings.Contains(parsed.Error, "exceeds max window") {
		t.Fatalf("expected max window error, got %q", parsed.Error)
	}
}

func TestParsePrometheusStep(t *testing.T) {
	got, err := parsePrometheusStep("90")
	if err != nil {
		t.Fatalf("parse numeric step: %v", err)
	}
	if got.String() != "1m30s" {
		t.Fatalf("expected 1m30s, got %s", got)
	}

	got, err = parsePrometheusStep("2m")
	if err != nil {
		t.Fatalf("parse duration step: %v", err)
	}
	if got.String() != "2m0s" {
		t.Fatalf("expected 2m0s, got %s", got)
	}
}
