package tools

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type fakeInvokableTool struct {
	info *schema.ToolInfo
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
