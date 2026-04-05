package mem

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestExtractFacts(t *testing.T) {
	facts := extractFacts(
		"我们的服务名是order-service，运行在端口8080",
		"好的，我了解了order-service运行在端口8080上",
	)
	if len(facts) == 0 {
		t.Fatal("expected to extract facts about 服务名/端口")
	}
	foundService := false
	for _, f := range facts {
		if strings.Contains(f, "服务名") || strings.Contains(f, "端口") {
			foundService = true
		}
	}
	if !foundService {
		t.Fatal("expected service/port related fact")
	}
}

func TestExtractFacts_NoMatch(t *testing.T) {
	facts := extractFacts("你好", "你好，有什么可以帮你的？")
	if len(facts) != 0 {
		t.Fatalf("expected 0 facts for casual greeting, got %d", len(facts))
	}
}

func TestExtractPreferences(t *testing.T) {
	prefs := extractPreferences("我喜欢简洁的回答，请用中文回复")
	if len(prefs) == 0 {
		t.Fatal("expected to extract preferences")
	}
}

func TestExtractPreferences_NoMatch(t *testing.T) {
	prefs := extractPreferences("查一下告警")
	if len(prefs) != 0 {
		t.Fatalf("expected 0 preferences, got %d", len(prefs))
	}
}

func TestBuildEnrichedContext_NoMemories(t *testing.T) {
	resetLTM()
	ctx := context.Background()
	shortTerm := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}

	result := BuildEnrichedContext(ctx, "empty-session", "test query", shortTerm)
	if len(result) != 2 {
		t.Fatalf("with no long-term memories, should return short-term only, got %d", len(result))
	}
}

func TestBuildEnrichedContext_WithMemories(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess-enrich", MemoryTypeFact, "服务名是payment-service", "test")
	ltm.Store(ctx, "sess-enrich", MemoryTypePreference, "用户偏好详细回答", "test")

	shortTerm := []*schema.Message{
		schema.UserMessage("hello"),
	}

	result := BuildEnrichedContext(ctx, "sess-enrich", "服务信息", shortTerm)
	if len(result) <= len(shortTerm) {
		t.Fatalf("expected enriched context to be longer than short-term, got %d", len(result))
	}

	hasMemoryMarker := false
	for _, msg := range result {
		if strings.Contains(msg.Content, "[关键记忆]") {
			hasMemoryMarker = true
		}
	}
	if !hasMemoryMarker {
		t.Fatal("expected [关键记忆] marker in enriched context")
	}
}

func TestExtractMemories_Integration(t *testing.T) {
	resetLTM()
	ctx := context.Background()

	ExtractMemories(ctx, "integ-sess",
		"我们的服务名是payment-service，IP地址是10.0.0.1",
		"好的，我已记录payment-service的IP地址为10.0.0.1。该服务名对应的是支付服务模块。",
	)

	ltm := GetLongTermMemory()
	count := ltm.CountBySession("integ-sess")
	if count == 0 {
		t.Fatal("expected memories to be extracted from conversation")
	}

	all := ltm.GetAllBySession("integ-sess")
	hasFact := false
	for _, m := range all {
		if m.Type == MemoryTypeFact {
			hasFact = true
		}
	}
	if !hasFact {
		t.Fatal("expected at least one fact to be extracted")
	}
}

func TestExtractMemoriesWithReportDropsBoilerplate(t *testing.T) {
	resetLTM()
	ctx := context.Background()

	report := ExtractMemoriesWithReport(ctx, "report-sess",
		"我们的服务名是什么？",
		"抱歉，作为AI助手我无法直接确认，但服务名是payment-service。",
	)
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.Dropped) == 0 {
		t.Fatal("expected boilerplate candidate to be dropped")
	}
	foundBoilerplate := false
	for _, item := range report.Dropped {
		if item.Reason == "assistant_boilerplate" {
			foundBoilerplate = true
			break
		}
	}
	if !foundBoilerplate {
		t.Fatalf("expected assistant_boilerplate drop reason, got %+v", report.Dropped)
	}
}

func TestSplitSentences(t *testing.T) {
	text := "第一句。第二句；第三句！第四句？\n第五句"
	result := splitSentences(text)
	if len(result) < 5 {
		t.Fatalf("expected >= 5 sentences, got %d: %v", len(result), result)
	}
}

func TestDedup(t *testing.T) {
	items := []string{"a", "b", "a", "c", "b"}
	result := dedup(items)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique items, got %d", len(result))
	}
}

func TestTruncate(t *testing.T) {
	short := truncate("hi", 10)
	if short != "hi" {
		t.Fatalf("expected 'hi', got '%s'", short)
	}

	long := truncate("this is a very long string", 10)
	if !strings.HasSuffix(long, "...") {
		t.Fatal("expected truncated string to end with ...")
	}
	if len(long) != 13 {
		t.Fatalf("expected length 13 (10+3), got %d", len(long))
	}
}
