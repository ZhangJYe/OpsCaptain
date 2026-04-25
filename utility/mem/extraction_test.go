package mem

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type fakeMemoryChatModel struct {
	content      string
	err          error
	beforeReturn func()
	input        []*schema.Message
}

func (f *fakeMemoryChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	f.input = input
	if f.beforeReturn != nil {
		f.beforeReturn()
	}
	if f.err != nil {
		return nil, f.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return schema.AssistantMessage(f.content, nil), nil
}

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

func TestExtractMemoryCandidatesDoesNotCreateEpisodeByDefault(t *testing.T) {
	candidates := ExtractMemoryCandidates("查一下 payment-service", strings.Repeat("详细回答", 80))
	for _, candidate := range candidates {
		if candidate.Type == MemoryTypeEpisode {
			t.Fatal("expected default extractor not to create episode memories")
		}
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

func TestRuleMemoryAgentPromotesPreferenceToUserScope(t *testing.T) {
	agent := NewRuleMemoryAgent()
	decision, err := agent.Decide(context.Background(), MemoryEvent{
		SessionID: "agent-session",
		UserID:    "user-42",
		Query:     "我喜欢简洁的中文回答",
		Answer:    "好的",
	})
	if err != nil {
		t.Fatalf("expected decision, got error: %v", err)
	}
	if len(decision.Actions) == 0 {
		t.Fatal("expected at least one action")
	}
	action := decision.Actions[0]
	if action.Op != MemoryActionUpsert {
		t.Fatalf("expected upsert action, got %s", action.Op)
	}
	if action.Scope != MemoryScopeUser || action.ScopeID != "user-42" {
		t.Fatalf("expected user-scoped preference, got scope=%s scope_id=%s", action.Scope, action.ScopeID)
	}
}

func TestMemoryApplierWritesAuditRecord(t *testing.T) {
	resetLTM()
	report := NewMemoryApplier(GetLongTermMemory()).Apply(context.Background(), MemoryEvent{
		SessionID: "agent-apply-session",
		UserID:    "user-42",
		TraceID:   "trace-1",
	}, &MemoryDecision{Actions: []MemoryAction{{
		Op:         MemoryActionUpsert,
		Type:       MemoryTypePreference,
		Content:    "我喜欢简洁的中文回答",
		Scope:      MemoryScopeUser,
		ScopeID:    "user-42",
		Confidence: 0.85,
		Reason:     "test",
	}}})
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.StoredIDs) != 1 {
		t.Fatalf("expected one stored memory, got %+v", report.StoredIDs)
	}
	if len(report.AuditRecords) != 1 {
		t.Fatalf("expected one audit record, got %+v", report.AuditRecords)
	}
	record := report.AuditRecords[0]
	if record.MemoryID != report.StoredIDs[0] || record.TraceID != "trace-1" || record.Op != MemoryActionUpsert {
		t.Fatalf("unexpected audit record: %+v", record)
	}
}

func TestMemoryApplierDropsSecretAction(t *testing.T) {
	resetLTM()
	report := NewMemoryApplier(GetLongTermMemory()).Apply(context.Background(), MemoryEvent{
		SessionID: "agent-secret-session",
	}, &MemoryDecision{Actions: []MemoryAction{{
		Op:         MemoryActionUpsert,
		Type:       MemoryTypeFact,
		Content:    "数据库 password 是 should-not-store",
		Scope:      MemoryScopeSession,
		Confidence: 0.90,
	}}})
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.StoredIDs) != 0 {
		t.Fatalf("expected no stored memory, got %+v", report.StoredIDs)
	}
	if len(report.Dropped) != 1 || report.Dropped[0].Reason != "contains_secret_marker" {
		t.Fatalf("expected secret marker drop, got %+v", report.Dropped)
	}
}

func TestMemoryApplierDoesNotSupersedeWithInvalidAction(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	id := ltm.Store(context.Background(), "agent-supersede-session", MemoryTypeFact, "服务名是 payment-service", "test")

	report := NewMemoryApplier(ltm).Apply(context.Background(), MemoryEvent{
		SessionID: "agent-supersede-session",
	}, &MemoryDecision{Actions: []MemoryAction{{
		Op:         MemoryActionSupersede,
		TargetID:   id,
		Type:       MemoryTypeFact,
		Content:    "数据库 password 是 should-not-store",
		Scope:      MemoryScopeSession,
		Confidence: 0.90,
	}}})
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.StoredIDs) != 0 {
		t.Fatalf("expected no stored memory, got %+v", report.StoredIDs)
	}
	current := ltm.Get(id)
	if current == nil {
		t.Fatal("expected original memory to remain")
	}
	if current.SafetyLabel == "disabled" {
		t.Fatalf("expected invalid supersede not to disable original memory: %+v", current)
	}
}

func TestLLMMemoryAgentAppliesStructuredDecision(t *testing.T) {
	resetLTM()
	chatModel := &fakeMemoryChatModel{content: `{
		"actions": [{
			"op": "upsert",
			"type": "preference",
			"content": "用户喜欢先给结论再展开细节",
			"scope": "user",
			"scope_id": "user-llm",
			"confidence": 0.91,
			"reason": "用户明确表达了回答偏好"
		}]
	}`}
	report := ProcessMemoryEventWithAgent(context.Background(), MemoryEvent{
		SessionID: "llm-session",
		UserID:    "user-llm",
		Query:     "以后先给结论再展开细节",
		Answer:    "好的",
	}, NewLLMMemoryAgent(chatModel, NewRuleMemoryAgent()))
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.StoredIDs) != 1 {
		t.Fatalf("expected one stored memory, got %+v", report.StoredIDs)
	}
	entry := GetLongTermMemory().Get(report.StoredIDs[0])
	if entry == nil {
		t.Fatal("expected stored memory")
	}
	if entry.Scope != MemoryScopeUser || entry.ScopeID != "user-llm" || entry.Type != MemoryTypePreference {
		t.Fatalf("unexpected stored memory: %+v", entry)
	}
	if len(chatModel.input) != 2 {
		t.Fatalf("expected llm prompt messages, got %d", len(chatModel.input))
	}
}

func TestLLMMemoryAgentSupersedesExistingMemory(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	oldID := ltm.StoreWithOptions(context.Background(), "llm-supersede", MemoryTypeFact, "服务名是 old-payment-service", "test", MemoryStoreOptions{
		Scope:         MemoryScopeSession,
		ScopeID:       "llm-supersede",
		Confidence:    0.80,
		ConflictGroup: "fact:服务名",
	})
	chatModel := &fakeMemoryChatModel{content: `{
		"actions": [{
			"op": "supersede",
			"target_id": "` + oldID + `",
			"type": "fact",
			"content": "服务名是 payment-service",
			"scope": "session",
			"scope_id": "llm-supersede",
			"confidence": 0.92,
			"conflict_group": "fact:服务名",
			"reason": "用户纠正了服务名"
		}]
	}`}
	report := ProcessMemoryEventWithAgent(context.Background(), MemoryEvent{
		SessionID: "llm-supersede",
		Query:     "刚才说错了，服务名是 payment-service",
		Answer:    "已记录",
	}, NewLLMMemoryAgent(chatModel, NewRuleMemoryAgent()))
	if report == nil || len(report.StoredIDs) != 1 {
		t.Fatalf("expected new stored memory, got %+v", report)
	}
	oldEntry := ltm.Get(oldID)
	if oldEntry == nil || oldEntry.SafetyLabel != "superseded" {
		t.Fatalf("expected old memory to be superseded, got %+v", oldEntry)
	}
}

func TestLLMMemoryAgentFallsBackToRuleOnBadJSON(t *testing.T) {
	resetLTM()
	chatModel := &fakeMemoryChatModel{content: "不是 JSON"}
	report := ProcessMemoryEventWithAgent(context.Background(), MemoryEvent{
		SessionID: "llm-fallback",
		UserID:    "user-fallback",
		Query:     "我喜欢简洁的中文回答",
		Answer:    "好的",
	}, NewLLMMemoryAgent(chatModel, NewRuleMemoryAgent()))
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.StoredIDs) != 1 {
		t.Fatalf("expected fallback rule memory, got %+v", report)
	}
	entry := GetLongTermMemory().Get(report.StoredIDs[0])
	if entry == nil || entry.Scope != MemoryScopeUser {
		t.Fatalf("expected fallback user-scoped preference, got %+v", entry)
	}
}

func TestLLMMemoryAgentFallsBackToRuleAfterContextCanceledByModel(t *testing.T) {
	resetLTM()
	ctx, cancel := context.WithCancel(context.Background())
	chatModel := &fakeMemoryChatModel{beforeReturn: cancel}
	report := ProcessMemoryEventWithAgent(ctx, MemoryEvent{
		SessionID: "llm-timeout-fallback",
		UserID:    "user-timeout",
		Query:     "我喜欢简洁的中文回答",
		Answer:    "好的",
	}, NewLLMMemoryAgent(chatModel, NewRuleMemoryAgent()))
	if report == nil {
		t.Fatal("expected report")
	}
	if len(report.StoredIDs) != 1 {
		t.Fatalf("expected fallback rule memory after canceled llm context, got %+v", report)
	}
	entry := GetLongTermMemory().Get(report.StoredIDs[0])
	if entry == nil || entry.Scope != MemoryScopeUser {
		t.Fatalf("expected fallback user-scoped preference, got %+v", entry)
	}
}

func TestValidateMemoryCandidateDropsSecretMarkers(t *testing.T) {
	ok, reason := ValidateMemoryCandidate(MemoryCandidate{
		Type:    MemoryTypeFact,
		Content: "数据库 password 是 should-not-store",
		Source:  "test",
	})
	if ok {
		t.Fatal("expected secret-like memory candidate to be dropped")
	}
	if reason != "contains_secret_marker" {
		t.Fatalf("expected contains_secret_marker, got %q", reason)
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
