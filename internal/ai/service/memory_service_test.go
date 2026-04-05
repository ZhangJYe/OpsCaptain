package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"SuperBizAgent/utility/mem"
)

func TestPersistOutcomeUsesBoundedContext(t *testing.T) {
	oldExtract := extractMemoriesFunc
	oldTimeout := memoryExtractionTimeout
	defer func() {
		extractMemoriesFunc = oldExtract
		memoryExtractionTimeout = oldTimeout
	}()

	ctxCh := make(chan context.Context, 1)
	extractMemoriesFunc = func(ctx context.Context, sessionID, userMsg, assistantMsg string) *mem.MemoryExtractionReport {
		ctxCh <- ctx
		<-ctx.Done()
		return &mem.MemoryExtractionReport{}
	}
	memoryExtractionTimeout = func(context.Context) time.Duration {
		return 20 * time.Millisecond
	}

	svc := NewMemoryService()
	svc.PersistOutcome(context.Background(), "session-test", "用户问题", "系统回答")

	select {
	case extractCtx := <-ctxCh:
		if _, ok := extractCtx.Deadline(); !ok {
			t.Fatal("expected extraction context to have deadline")
		}
		select {
		case <-extractCtx.Done():
			if !errors.Is(extractCtx.Err(), context.DeadlineExceeded) {
				t.Fatalf("expected deadline exceeded, got %v", extractCtx.Err())
			}
		case <-time.After(200 * time.Millisecond):
			t.Fatal("expected extraction context to timeout promptly")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected extraction goroutine to start")
	}
}

func TestBuildChatPackageReturnsContextTraceDetails(t *testing.T) {
	mem.ClearSession("ctx-chat")
	mem.GetLongTermMemory().Forget(context.Background(), 10)
	sessionMem := mem.GetSimpleMemory("ctx-chat")
	sessionMem.AddUserAssistantPair("之前问了什么", "之前答了什么")
	mem.GetLongTermMemory().Store(context.Background(), "ctx-chat", mem.MemoryTypeFact, "服务名是payment-service", "test")

	svc := NewMemoryService()
	pkg, details := svc.BuildChatPackage(context.Background(), "ctx-chat", "请继续分析 payment-service", sessionMem.GetContextMessages())

	if pkg == nil {
		t.Fatal("expected package")
	}
	if len(pkg.HistoryMessages) == 0 {
		t.Fatal("expected history messages")
	}
	if len(details) == 0 {
		t.Fatal("expected context details")
	}
}
