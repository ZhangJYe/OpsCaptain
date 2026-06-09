package contextcompression

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func testConfig(mode Mode) *CompressionConfig {
	return &CompressionConfig{
		Enabled:         true,
		Mode:            mode,
		MinTokens:       10, // 低阈值，方便测试
		PreserveFirst:   2,
		PreserveLast:    1,
		LogContextLines: 1,
		SourceTypes:     []string{"tool", "rag"},
	}
}

func TestCompress_DisabledMode(t *testing.T) {
	cfg := &CompressionConfig{Enabled: false, Mode: ModeOff}
	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		Content:    "some content here",
	}, cfg)

	if result.Report.Strategy != "disabled" {
		t.Errorf("expected strategy=disabled, got %s", result.Report.Strategy)
	}
	if result.Content != "some content here" {
		t.Error("disabled mode should return original content")
	}
}

func TestCompress_AuditModeReturnsOriginal(t *testing.T) {
	items := make([]map[string]string, 10)
	for i := range items {
		items[i] = map[string]string{"id": string(rune('A' + i)), "value": "test data content"}
	}
	data, _ := json.Marshal(items)

	cfg := testConfig(ModeAudit)
	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		SourceID:   "test_tool",
		Query:      "test query",
		Content:    string(data),
	}, cfg)

	// audit 模式应返回原文
	if result.Content != string(data) {
		t.Error("audit mode should return original content")
	}
	// 但应有有效的报告
	if result.Report.Strategy == "" || result.Report.Strategy == "disabled" {
		t.Error("audit mode should produce a valid report")
	}
	if result.Report.TokensBefore == 0 {
		t.Error("report should have tokens_before")
	}
}

func TestCompress_OptimizeModeCompresses(t *testing.T) {
	items := make([]map[string]string, 10)
	for i := range items {
		items[i] = map[string]string{
			"id":     string(rune('A' + i)),
			"value":  "this is a longer test data content to ensure enough tokens for compression to trigger",
			"status": "ok",
			"detail": "additional details about this particular log entry that makes it longer",
		}
	}
	data, _ := json.Marshal(items)

	cfg := testConfig(ModeOptimize)
	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		SourceID:   "test_tool",
		Query:      "unrelated query",
		Content:    string(data),
	}, cfg)

	t.Logf("Strategy: %s", result.Report.Strategy)
	t.Logf("Tokens Before: %d", result.Report.TokensBefore)
	t.Logf("Tokens After: %d", result.Report.TokensAfter)
	t.Logf("Compression Ratio: %.2f", result.Report.CompressionRatio)
	t.Logf("Content length before: %d, after: %d", len(string(data)), len(result.Content))
	t.Logf("Content before (first 200): %s", string(data)[:min(200, len(data))])
	t.Logf("Content after (first 200): %s", result.Content[:min(200, len(result.Content))])

	// optimize 模式应返回压缩内容
	if result.Content == string(data) {
		t.Error("optimize mode should return compressed content for large input")
	}
	if result.Report.CompressionRatio >= 1.0 {
		t.Errorf("compression ratio should be < 1.0, got %.2f", result.Report.CompressionRatio)
	}
}

func TestCompress_BelowMinTokens(t *testing.T) {
	cfg := testConfig(ModeOptimize)
	cfg.MinTokens = 1000 // 设置很高的阈值

	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		Content:    "short content",
	}, cfg)

	if result.Report.Strategy != "below_min_tokens" {
		t.Errorf("expected strategy=below_min_tokens, got %s", result.Report.Strategy)
	}
	if result.Content != "short content" {
		t.Error("below min tokens should return original content")
	}
}

func TestCompress_OptimizeNoSavingsKeepsOriginal(t *testing.T) {
	cfg := testConfig(ModeOptimize)
	cfg.MinTokens = 1

	content := `{"message":"error","status":"failed"}`
	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		Query:      "unrelated",
		Content:    content,
	}, cfg)

	if result.Content != content {
		t.Fatalf("optimize should keep original when there is no token saving, got %q", result.Content)
	}
	if result.Report.CompressionRatio != 1.0 {
		t.Fatalf("expected reported ratio=1.0 for no-savings passthrough, got %.2f", result.Report.CompressionRatio)
	}
}

func TestCompress_SourceTypeExcluded(t *testing.T) {
	cfg := testConfig(ModeOptimize)
	cfg.SourceTypes = []string{"rag"} // 只允许 rag

	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		Content:    "some content",
	}, cfg)

	if result.Report.Strategy != "source_type_excluded" {
		t.Errorf("expected strategy=source_type_excluded, got %s", result.Report.Strategy)
	}
}

func TestCompress_JSONArrayStrategy(t *testing.T) {
	items := make([]map[string]string, 10)
	for i := range items {
		items[i] = map[string]string{"id": string(rune('A' + i)), "value": "data"}
	}
	data, _ := json.Marshal(items)

	cfg := testConfig(ModeOptimize)
	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		Content:    string(data),
	}, cfg)

	if result.Report.Strategy != "json_array" {
		t.Errorf("expected strategy=json_array, got %s", result.Report.Strategy)
	}
}

func TestCompress_LogStrategy(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "INFO  some normal log line with enough content"
	}
	lines[10] = "ERROR critical failure happened"
	content := strings.Join(lines, "\n")

	cfg := testConfig(ModeOptimize)
	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		Content:    content,
	}, cfg)

	if result.Report.Strategy != "log" {
		t.Errorf("expected strategy=log, got %s", result.Report.Strategy)
	}
	if !strings.Contains(result.Content, "ERROR critical failure") {
		t.Error("compressed content should preserve error line")
	}
}

func TestCompress_ReportFields(t *testing.T) {
	items := make([]map[string]string, 8)
	for i := range items {
		items[i] = map[string]string{"id": string(rune('A' + i)), "value": "some test data"}
	}
	data, _ := json.Marshal(items)

	cfg := testConfig(ModeOptimize)
	result := Compress(context.Background(), Request{
		SourceType: SourceTool,
		SourceID:   "my_tool",
		Query:      "test",
		Content:    string(data),
	}, cfg)

	report := result.Report
	if report.SourceType != "tool" {
		t.Errorf("expected source_type=tool, got %s", report.SourceType)
	}
	if report.SourceID != "my_tool" {
		t.Errorf("expected source_id=my_tool, got %s", report.SourceID)
	}
	if report.Mode != "optimize" {
		t.Errorf("expected mode=optimize, got %s", report.Mode)
	}
	if report.TokensBefore <= 0 {
		t.Error("tokens_before should be > 0")
	}
	if report.TokensAfter <= 0 {
		t.Error("tokens_after should be > 0")
	}
	if report.CompressionRatio <= 0 || report.CompressionRatio > 1.0 {
		t.Errorf("compression_ratio should be in (0, 1], got %.2f", report.CompressionRatio)
	}
	if report.LatencyMs < 0 {
		t.Error("latency_ms should be >= 0")
	}
}

func TestIsJSONArray(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"[1,2,3]", true},
		{"[{\"a\":1}]", true},
		{"[]", true},
		{"  [1]  ", true},
		{"{\"a\":1}", false},
		{"not json", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isJSONArray(tt.input); got != tt.want {
			t.Errorf("isJSONArray(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsJSONObject(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"{\"a\":1}", true},
		{"  {\"a\":1}  ", true},
		{"[1,2,3]", false},
		{"not json", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isJSONObject(tt.input); got != tt.want {
			t.Errorf("isJSONObject(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
