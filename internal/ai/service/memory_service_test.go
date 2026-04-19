package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/mem"
)

func TestPersistOutcomeUsesBoundedContext(t *testing.T) {
	oldExtract := extractMemoriesFunc
	oldTimeout := memoryExtractionTimeout
	oldMaxJobs := memoryExtractionMaxJobs
	oldWait := memoryExtractionWait
	oldEnqueue := enqueueMemoryExtraction
	defer func() {
		extractMemoriesFunc = oldExtract
		memoryExtractionTimeout = oldTimeout
		memoryExtractionMaxJobs = oldMaxJobs
		memoryExtractionWait = oldWait
		enqueueMemoryExtraction = oldEnqueue
		resetMemoryExtractionSemaphoreForTest()
	}()
	resetMemoryExtractionSemaphoreForTest()
	enqueueMemoryExtraction = func(context.Context, string, string, string) (bool, error) {
		return false, nil
	}

	ctxCh := make(chan context.Context, 1)
	extractMemoriesFunc = func(ctx context.Context, sessionID, userMsg, assistantMsg string) *mem.MemoryExtractionReport {
		ctxCh <- ctx
		<-ctx.Done()
		return &mem.MemoryExtractionReport{}
	}
	memoryExtractionTimeout = func(context.Context) time.Duration {
		return 20 * time.Millisecond
	}
	memoryExtractionMaxJobs = func(context.Context) int {
		return 1
	}
	memoryExtractionWait = func(context.Context) time.Duration {
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

func TestPersistOutcomeDropsWhenExtractionQueueBusy(t *testing.T) {
	oldExtract := extractMemoriesFunc
	oldTimeout := memoryExtractionTimeout
	oldMaxJobs := memoryExtractionMaxJobs
	oldWait := memoryExtractionWait
	oldEnqueue := enqueueMemoryExtraction
	defer func() {
		extractMemoriesFunc = oldExtract
		memoryExtractionTimeout = oldTimeout
		memoryExtractionMaxJobs = oldMaxJobs
		memoryExtractionWait = oldWait
		enqueueMemoryExtraction = oldEnqueue
		resetMemoryExtractionSemaphoreForTest()
	}()
	resetMemoryExtractionSemaphoreForTest()
	enqueueMemoryExtraction = func(context.Context, string, string, string) (bool, error) {
		return false, nil
	}

	var calls int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	extractMemoriesFunc = func(ctx context.Context, sessionID, userMsg, assistantMsg string) *mem.MemoryExtractionReport {
		atomic.AddInt32(&calls, 1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return &mem.MemoryExtractionReport{}
	}
	memoryExtractionTimeout = func(context.Context) time.Duration {
		return time.Second
	}
	memoryExtractionMaxJobs = func(context.Context) int {
		return 1
	}
	memoryExtractionWait = func(context.Context) time.Duration {
		return 10 * time.Millisecond
	}

	svc := NewMemoryService()
	svc.PersistOutcome(context.Background(), "session-busy", "q1", "a1")
	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected first extraction to start")
	}

	svc.PersistOutcome(context.Background(), "session-busy", "q2", "a2")
	time.Sleep(80 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly one extraction call while queue is busy, got %d", got)
	}
	close(release)
}

func TestPersistOutcomeEnqueuedSkipsLocalExtraction(t *testing.T) {
	oldExtract := extractMemoriesFunc
	oldTimeout := memoryExtractionTimeout
	oldMaxJobs := memoryExtractionMaxJobs
	oldWait := memoryExtractionWait
	oldEnqueue := enqueueMemoryExtraction
	defer func() {
		extractMemoriesFunc = oldExtract
		memoryExtractionTimeout = oldTimeout
		memoryExtractionMaxJobs = oldMaxJobs
		memoryExtractionWait = oldWait
		enqueueMemoryExtraction = oldEnqueue
		resetMemoryExtractionSemaphoreForTest()
	}()
	resetMemoryExtractionSemaphoreForTest()

	var calls int32
	extractMemoriesFunc = func(context.Context, string, string, string) *mem.MemoryExtractionReport {
		atomic.AddInt32(&calls, 1)
		return &mem.MemoryExtractionReport{}
	}
	enqueueMemoryExtraction = func(context.Context, string, string, string) (bool, error) {
		return true, nil
	}

	svc := NewMemoryService()
	svc.PersistOutcome(context.Background(), "session-enqueued", "q", "a")
	time.Sleep(50 * time.Millisecond)

	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("expected local extraction not to run when event is enqueued, got %d", got)
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

func TestResolveSessionIDPrefersExistingContextSession(t *testing.T) {
	svc := NewMemoryService()
	ctx := context.WithValue(context.Background(), consts.CtxKeySessionID, "approval-session")

	got := svc.ResolveSessionID(ctx)
	if got != "approval-session" {
		t.Fatalf("expected existing session id to be reused, got %q", got)
	}
}

func resetMemoryExtractionSemaphoreForTest() {
	memoryExtractSemaphoreMu.Lock()
	defer memoryExtractSemaphoreMu.Unlock()
	memoryExtractSemaphore = nil
	memoryExtractSemaphoreN = 0
}
