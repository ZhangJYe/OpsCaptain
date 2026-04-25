package contextengine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/utility/mem"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type fakeContextRetriever struct {
	docs []*schema.Document
}

func (f *fakeContextRetriever) Retrieve(context.Context, string, ...retrieverapi.Option) ([]*schema.Document, error) {
	return f.docs, nil
}

func TestAssemblerBuildsStagedChatContext(t *testing.T) {
	resetLongTermMemory()
	oldFactory := rag.NewRetrieverFunc
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
	}()
	rag.ResetSharedPool()
	rag.NewRetrieverFunc = func(context.Context) (retrieverapi.Retriever, error) {
		return &fakeContextRetriever{
			docs: []*schema.Document{
				{
					ID:      "doc-1",
					Content: "payment-service 的 SOP 说明了排障步骤。",
					MetaData: map[string]any{
						"title": "payment sop",
					},
				},
			},
		}, nil
	}

	ltm := mem.GetLongTermMemory()
	ctx := context.Background()
	ltm.Store(ctx, "sess-chat", mem.MemoryTypeFact, "服务名是payment-service", "test")
	ltm.Store(ctx, "sess-chat", mem.MemoryTypePreference, "用户偏好简洁回答", "test")

	history := []*schema.Message{
		schema.UserMessage("[对话历史摘要] 用户之前关注过支付服务"),
		schema.AssistantMessage("好的，我记住了。", nil),
		schema.UserMessage("之前的提问 1"),
		schema.AssistantMessage("之前的回答 1", nil),
		schema.UserMessage("之前的提问 2"),
		schema.AssistantMessage("之前的回答 2", nil),
	}

	assembler := NewAssembler()
	pkg, err := assembler.Assemble(ctx, ContextRequest{
		SessionID: "sess-chat",
		Mode:      "chat",
		Query:     "请继续分析 payment-service",
	}, history)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pkg.MemoryItems) == 0 {
		t.Fatal("expected memory items in chat package")
	}
	if len(pkg.DocumentItems) == 0 {
		t.Fatal("expected document items in chat package")
	}
	var docStage *StageTrace
	for i := range pkg.Trace.Stages {
		if pkg.Trace.Stages[i].Name == "documents" {
			docStage = &pkg.Trace.Stages[i]
			break
		}
	}
	if docStage == nil || docStage.Retrieval == nil {
		t.Fatal("expected document retrieval trace")
	}
	if docStage.Retrieval.ResultCount != 1 {
		t.Fatalf("expected document retrieval hit count 1, got %+v", docStage.Retrieval)
	}
	if len(pkg.HistoryMessages) == 0 {
		t.Fatal("expected history messages in chat package")
	}
	if !strings.Contains(pkg.HistoryMessages[0].Content, "[关键记忆]") {
		t.Fatalf("expected staged memory marker, got %q", pkg.HistoryMessages[0].Content)
	}
	if len(TraceDetails(pkg.Trace)) == 0 {
		t.Fatal("expected trace details")
	}
}

func TestAssemblerDocumentTraceShowsCacheReuse(t *testing.T) {
	resetLongTermMemory()
	oldFactory := rag.NewRetrieverFunc
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
	}()
	rag.ResetSharedPool()
	rag.NewRetrieverFunc = func(context.Context) (retrieverapi.Retriever, error) {
		return &fakeContextRetriever{
			docs: []*schema.Document{
				{ID: "doc-1", Content: "same doc"},
			},
		}, nil
	}

	assembler := NewAssembler()
	for i := 0; i < 2; i++ {
		pkg, err := assembler.Assemble(context.Background(), ContextRequest{
			SessionID: "sess-cache",
			Mode:      "chat",
			Query:     "cache me",
		}, nil)
		if err != nil {
			t.Fatalf("assemble %d: %v", i+1, err)
		}
		for _, stage := range pkg.Trace.Stages {
			if stage.Name != "documents" || stage.Retrieval == nil {
				continue
			}
			if i == 0 && stage.Retrieval.CacheHit {
				t.Fatal("expected first document retrieval to miss cache")
			}
			if i == 1 && !stage.Retrieval.CacheHit {
				t.Fatal("expected second document retrieval to hit cache")
			}
		}
	}
}

func TestAssemblerBuildsAIOpsMemoryOnlyContext(t *testing.T) {
	resetLongTermMemory()
	oldFactory := rag.NewRetrieverFunc
	defer func() {
		rag.NewRetrieverFunc = oldFactory
		rag.ResetSharedPool()
	}()
	rag.ResetSharedPool()

	ltm := mem.GetLongTermMemory()
	ctx := context.Background()
	ltm.Store(ctx, "sess-aiops", mem.MemoryTypeFact, "支付服务最近出现连接超时", "test")

	assembler := NewAssembler()
	pkg, err := assembler.Assemble(ctx, ContextRequest{
		SessionID: "sess-aiops",
		Mode:      "aiops",
		Query:     "请排查支付服务日志异常",
	}, []*schema.Message{
		schema.UserMessage("不应被 aiops profile 使用的历史"),
	})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pkg.HistoryMessages) != 0 {
		t.Fatalf("expected aiops package to skip history, got %d messages", len(pkg.HistoryMessages))
	}
	if len(pkg.MemoryItems) == 0 {
		t.Fatal("expected aiops package to include memory")
	}
	if got := MemoryContext(pkg); !strings.Contains(got, "支付服务最近出现连接超时") {
		t.Fatalf("unexpected memory context: %q", got)
	}
}

func TestAssemblerDropsUnusableMemoriesWithTrace(t *testing.T) {
	resetLongTermMemory()
	ltm := mem.GetLongTermMemory()
	ctx := context.Background()
	ltm.StoreWithOptions(ctx, "sess-policy", mem.MemoryTypeFact, "payment-service 正常记忆", "test", mem.MemoryStoreOptions{
		Confidence: 0.90,
	})
	ltm.StoreWithOptions(ctx, "sess-policy", mem.MemoryTypeFact, "payment-service 低可信记忆", "test", mem.MemoryStoreOptions{
		Confidence: 0.20,
	})
	ltm.StoreWithOptions(ctx, "sess-policy", mem.MemoryTypeFact, "payment-service 过期记忆", "test", mem.MemoryStoreOptions{
		Confidence: 0.90,
		ExpiresAt:  1,
	})
	ltm.StoreWithOptions(ctx, "sess-policy", mem.MemoryTypeFact, "payment-service 敏感记忆", "test", mem.MemoryStoreOptions{
		Confidence:  0.90,
		SafetyLabel: "secret",
	})

	assembler := NewAssembler()
	pkg, err := assembler.Assemble(ctx, ContextRequest{
		SessionID: "sess-policy",
		Mode:      "aiops",
		Query:     "payment-service",
	}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pkg.MemoryItems) != 1 {
		t.Fatalf("expected only one usable memory, got %d", len(pkg.MemoryItems))
	}
	reasons := map[string]bool{}
	for _, item := range pkg.Trace.DroppedItems {
		reasons[item.DroppedReason] = true
	}
	for _, reason := range []string{"memory_confidence", "memory_safety"} {
		if !reasons[reason] {
			t.Fatalf("expected drop reason %s in trace, got %+v", reason, reasons)
		}
	}
}

func TestAssemblerExpiredMemoriesDoNotCrowdOutValidMemory(t *testing.T) {
	resetLongTermMemory()
	ltm := mem.GetLongTermMemory()
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		ltm.StoreWithOptions(ctx, "sess-expired-window", mem.MemoryTypeFact, fmt.Sprintf("payment-service 过期记忆-%d", i), "test", mem.MemoryStoreOptions{
			Confidence: 0.90,
			ExpiresAt:  1,
		})
	}
	ltm.StoreWithOptions(ctx, "sess-expired-window", mem.MemoryTypeFact, "payment-service 有效记忆", "test", mem.MemoryStoreOptions{
		Confidence: 0.90,
	})

	assembler := NewAssembler()
	pkg, err := assembler.Assemble(ctx, ContextRequest{
		SessionID: "sess-expired-window",
		Mode:      "aiops",
		Query:     "payment-service",
	}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(pkg.MemoryItems) != 1 {
		t.Fatalf("expected valid memory to survive expired candidates, got %d", len(pkg.MemoryItems))
	}
	if !strings.Contains(pkg.MemoryItems[0].Content, "有效记忆") {
		t.Fatalf("expected valid memory, got %q", pkg.MemoryItems[0].Content)
	}
}

func TestAssemblerLoadsLayeredMemoryScopes(t *testing.T) {
	resetLongTermMemory()
	ltm := mem.GetLongTermMemory()
	ctx := context.Background()
	ltm.Store(context.Background(), "sess-layer", mem.MemoryTypeFact, "session scoped payment-service memory", "test")
	ltm.StoreWithOptions(ctx, "user-layer", mem.MemoryTypeFact, "user scoped payment-service memory", "test", mem.MemoryStoreOptions{
		Scope:   mem.MemoryScopeUser,
		ScopeID: "user-layer",
	})
	ltm.StoreWithOptions(ctx, "project-layer", mem.MemoryTypeFact, "project scoped payment-service memory", "test", mem.MemoryStoreOptions{
		Scope:   mem.MemoryScopeProject,
		ScopeID: "project-layer",
	})
	ltm.StoreWithOptions(ctx, "global", mem.MemoryTypeFact, "global scoped payment-service memory", "test", mem.MemoryStoreOptions{
		Scope: mem.MemoryScopeGlobal,
	})

	assembler := NewAssembler()
	pkg, err := assembler.Assemble(ctx, ContextRequest{
		SessionID: "sess-layer",
		UserID:    "user-layer",
		ProjectID: "project-layer",
		Mode:      "aiops",
		Query:     "payment-service",
	}, nil)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	got := MemoryContext(pkg)
	for _, expected := range []string{"session scoped", "user scoped", "project scoped", "global scoped"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("expected %q in memory context, got %q", expected, got)
		}
	}
}

func TestNormalizeUnitFloatRejectsOutOfRangeValues(t *testing.T) {
	if got := normalizeUnitFloat(1.2, 0.5); got != 0.5 {
		t.Fatalf("expected fallback for value > 1, got %f", got)
	}
	if got := normalizeUnitFloat(-0.1, 0.5); got != 0.5 {
		t.Fatalf("expected fallback for value <= 0, got %f", got)
	}
	if got := normalizeUnitFloat(0.8, 0.5); got != 0.8 {
		t.Fatalf("expected valid value to pass through, got %f", got)
	}
}

func resetLongTermMemory() {
	ltm := mem.GetLongTermMemory()
	ltm.Forget(context.Background(), 10)
}
