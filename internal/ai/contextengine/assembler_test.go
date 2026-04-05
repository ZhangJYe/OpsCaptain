package contextengine

import (
	"context"
	"strings"
	"testing"

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
	oldFactory := newContextRetriever
	defer func() {
		newContextRetriever = oldFactory
		resetContextRetrieverCache()
	}()
	resetContextRetrieverCache()
	newContextRetriever = func(context.Context) (retrieverapi.Retriever, error) {
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

func TestAssemblerBuildsAIOpsMemoryOnlyContext(t *testing.T) {
	resetLongTermMemory()
	oldFactory := newContextRetriever
	defer func() {
		newContextRetriever = oldFactory
		resetContextRetrieverCache()
	}()
	resetContextRetrieverCache()

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

func resetLongTermMemory() {
	ltm := mem.GetLongTermMemory()
	ltm.Forget(context.Background(), 10)
}
