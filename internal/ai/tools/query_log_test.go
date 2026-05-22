package tools

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeInvokableTool struct {
	info *schema.ToolInfo
}

func TestIsConnectionErrorRecognizesUnknownSession(t *testing.T) {
	if !isConnectionError(errors.New("request failed with status 404: unknown session")) {
		t.Fatal("expected unknown session 404 to be connection error")
	}
}

func TestResolveLogHTTPURLDerivesFromSSE(t *testing.T) {
	t.Setenv("MCP_LOG_HTTP_URL", "")
	got := resolveLogHTTPURL(context.Background(), "http://127.0.0.1:18088/sse")
	if got != "http://127.0.0.1:18088/tools/query_logs" {
		t.Fatalf("unexpected derived url: %q", got)
	}
}

func TestResolveLogHTTPURLUsesEnvOverride(t *testing.T) {
	t.Setenv("MCP_LOG_HTTP_URL", "http://127.0.0.1:18089/tools/query_logs")
	got := resolveLogHTTPURL(context.Background(), "http://127.0.0.1:18088/sse")
	if got != "http://127.0.0.1:18089/tools/query_logs" {
		t.Fatalf("unexpected override url: %q", got)
	}
}

func TestCallLogHTTPFallbackReturnsStructuredLogs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tools/query_logs" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		var input LogQueryInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		if input.Query != "checkout timeout" {
			t.Fatalf("unexpected query: %q", input.Query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"logs":[{"service":"checkout","message":"timeout"}]}`))
	}))
	defer server.Close()

	result, err := callLogHTTPFallback(context.Background(), server.URL+"/tools/query_logs", `{"query":"checkout timeout"}`, 1000000000)
	if err != nil {
		t.Fatalf("callLogHTTPFallback returned error: %v", err)
	}
	if !strings.Contains(result, `"success":true`) {
		t.Fatalf("expected success payload, got %s", result)
	}
}

func TestHTTPLogQueryToolDegradesWhenEndpointFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusBadGateway)
	}))
	defer server.Close()

	logTool := NewHTTPLogQueryTool(server.URL, "sse unavailable")
	result, err := logTool.InvokableRun(context.Background(), `{"query":"checkout timeout"}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	var payload LogQueryUnavailableOutput
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if !payload.Degraded || payload.Success {
		t.Fatalf("expected degraded=false success payload, got %#v", payload)
	}
	if !strings.Contains(payload.Error, "http fallback failed") {
		t.Fatalf("expected fallback error, got %q", payload.Error)
	}
}

func (f *fakeInvokableTool) Info(context.Context) (*schema.ToolInfo, error) {
	return f.info, nil
}

func (f *fakeInvokableTool) InvokableRun(context.Context, string, ...tool.Option) (string, error) {
	return "ok", nil
}

func TestNormalizeOptionalURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "whitespace", input: "   ", want: ""},
		{name: "placeholder", input: "${MCP_LOG_URL}", want: ""},
		{name: "url", input: "http://localhost:8081", want: "http://localhost:8081"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeOptionalURL(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeOptionalURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestPooledToolWrapperInfoUsesAliasWhenConfigured(t *testing.T) {
	wrapper := &pooledToolWrapper{
		inner: &fakeInvokableTool{
			info: &schema.ToolInfo{
				Name: "search_logs",
				Desc: "search logs from mcp",
			},
		},
		alias: "query_logs",
	}

	info, err := wrapper.Info(context.Background())
	if err != nil {
		t.Fatalf("Info returned error: %v", err)
	}
	if info == nil {
		t.Fatal("expected tool info")
	}
	if info.Name != "query_logs" {
		t.Fatalf("expected alias name query_logs, got %q", info.Name)
	}
	if info.Desc != "search logs from mcp" {
		t.Fatalf("expected desc to be preserved, got %q", info.Desc)
	}
}

func TestPooledToolWrapperInfoKeepsOriginalNameWithoutAlias(t *testing.T) {
	wrapper := &pooledToolWrapper{
		inner: &fakeInvokableTool{
			info: &schema.ToolInfo{
				Name: "query_logs",
				Desc: "search logs from mcp",
			},
		},
	}

	info, err := wrapper.Info(context.Background())
	if err != nil {
		t.Fatalf("Info returned error: %v", err)
	}
	if info == nil {
		t.Fatal("expected tool info")
	}
	if info.Name != "query_logs" {
		t.Fatalf("expected original name query_logs, got %q", info.Name)
	}
}

func TestUnavailableLogQueryToolReturnsDegradedPayload(t *testing.T) {
	logTool := NewUnavailableLogQueryTool("mcp.log_url is not configured")
	info, err := logTool.Info(context.Background())
	if err != nil {
		t.Fatalf("Info returned error: %v", err)
	}
	if info == nil || info.Name != "query_logs" {
		t.Fatalf("expected query_logs tool info, got %#v", info)
	}

	result, err := logTool.InvokableRun(context.Background(), `{"query":"checkout timeout","service":"checkout"}`)
	if err != nil {
		t.Fatalf("InvokableRun returned error: %v", err)
	}
	var payload LogQueryUnavailableOutput
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
	if payload.Success {
		t.Fatal("expected success=false")
	}
	if !payload.Degraded {
		t.Fatal("expected degraded=true")
	}
	if payload.Query != "checkout timeout" {
		t.Fatalf("unexpected query: %q", payload.Query)
	}
	if !strings.Contains(payload.Error, "mcp.log_url") {
		t.Fatalf("expected config reason, got %q", payload.Error)
	}
}
