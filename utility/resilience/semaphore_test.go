package resilience

import (
	"context"
	"testing"
	"time"
)

func TestIsConcurrencyLimitError(t *testing.T) {
	if !IsConcurrencyLimitError(ErrLLMConcurrencyLimited) {
		t.Fatal("expected sentinel error to be recognized")
	}
}

func TestAcquireLLMSlotReleases(t *testing.T) {
	oldMaxConfig := llmMaxConcurrentCallsConfig
	oldWaitConfig := llmWaitTimeoutConfig
	defer func() {
		llmMaxConcurrentCallsConfig = oldMaxConfig
		llmWaitTimeoutConfig = oldWaitConfig
	}()
	llmMaxConcurrentCallsConfig = func(context.Context) int { return 1 }
	llmWaitTimeoutConfig = func(context.Context) time.Duration { return 50 * time.Millisecond }

	llmSemaphoreMu.Lock()
	llmSemaphore = make(chan struct{}, 1)
	llmSemaphoreN = 1
	llmSemaphoreMu.Unlock()

	release, err := AcquireLLMSlot(context.Background())
	if err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}
	release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	release, err = AcquireLLMSlot(ctx)
	if err != nil {
		t.Fatalf("expected slot to be reusable, got %v", err)
	}
	release()
}
