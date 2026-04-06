package resilience

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultLLMMaxConcurrentCalls = 10
	defaultLLMWaitTimeout        = 5 * time.Second
)

var ErrLLMConcurrencyLimited = errors.New("llm concurrency queue timeout")

var (
	llmSemaphoreMu sync.Mutex
	llmSemaphore   chan struct{}
	llmSemaphoreN  int

	llmMaxConcurrentCallsConfig = configuredMaxConcurrentCalls
	llmWaitTimeoutConfig        = configuredWaitTimeout
)

func AcquireLLMSlot(ctx context.Context) (func(), error) {
	maxCalls := llmMaxConcurrentCallsConfig(ctx)
	if maxCalls <= 0 {
		return func() {}, nil
	}

	sem := getOrCreateSemaphore(maxCalls)
	waitCtx, cancel := context.WithTimeout(ctx, llmWaitTimeoutConfig(ctx))
	defer cancel()

	select {
	case sem <- struct{}{}:
		return func() {
			select {
			case <-sem:
			default:
			}
		}, nil
	case <-waitCtx.Done():
		if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
			return nil, ErrLLMConcurrencyLimited
		}
		return nil, waitCtx.Err()
	}
}

func IsConcurrencyLimitError(err error) bool {
	return errors.Is(err, ErrLLMConcurrencyLimited)
}

func configuredMaxConcurrentCalls(ctx context.Context) int {
	v, err := g.Cfg().Get(ctx, "llm.max_concurrent_calls")
	if err != nil || v.Int() <= 0 {
		return defaultLLMMaxConcurrentCalls
	}
	return v.Int()
}

func configuredWaitTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "llm.concurrent_wait_timeout_ms")
	if err != nil || v.Int64() <= 0 {
		return defaultLLMWaitTimeout
	}
	return time.Duration(v.Int64()) * time.Millisecond
}

func getOrCreateSemaphore(size int) chan struct{} {
	llmSemaphoreMu.Lock()
	defer llmSemaphoreMu.Unlock()

	if llmSemaphore != nil && llmSemaphoreN == size {
		return llmSemaphore
	}
	llmSemaphore = make(chan struct{}, size)
	llmSemaphoreN = size
	return llmSemaphore
}
