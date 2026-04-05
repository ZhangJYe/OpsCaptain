package resilience

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestExecute_Success(t *testing.T) {
	opt := CallOption{
		Timeout:    5 * time.Second,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
		Name:       "test-success",
	}

	result, err := Execute(context.Background(), opt, func(ctx context.Context) (string, error) {
		return "hello", nil
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if result != "hello" {
		t.Fatalf("expected 'hello', got '%s'", result)
	}
}

func TestExecute_RetryThenSuccess(t *testing.T) {
	opt := CallOption{
		Timeout:    5 * time.Second,
		MaxRetries: 3,
		RetryDelay: 10 * time.Millisecond,
		Name:       "test-retry-success",
	}

	calls := 0
	result, err := Execute(context.Background(), opt, func(ctx context.Context) (string, error) {
		calls++
		if calls < 3 {
			return "", errors.New("temporary error")
		}
		return "recovered", nil
	})
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if result != "recovered" {
		t.Fatalf("expected 'recovered', got '%s'", result)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestExecute_AllRetriesFail(t *testing.T) {
	opt := CallOption{
		Timeout:    5 * time.Second,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
		Name:       "test-all-fail",
	}

	_, err := Execute(context.Background(), opt, func(ctx context.Context) (string, error) {
		return "", errors.New("persistent error")
	})
	if err == nil {
		t.Fatal("expected error after all retries failed")
	}
}

func TestExecute_Timeout(t *testing.T) {
	opt := CallOption{
		Timeout:    50 * time.Millisecond,
		MaxRetries: 0,
		RetryDelay: 10 * time.Millisecond,
		Name:       "test-timeout",
	}

	_, err := Execute(context.Background(), opt, func(ctx context.Context) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
			return "late", nil
		}
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen, got %d", cb.State())
	}

	if cb.Allow() {
		t.Fatal("expected circuit breaker to reject when open")
	}
}

func TestCircuitBreaker_RecoveryAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatal("expected open state")
	}

	time.Sleep(60 * time.Millisecond)

	if !cb.Allow() {
		t.Fatal("expected allow after reset timeout (half-open)")
	}
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected half-open, got %d", cb.State())
	}

	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Fatalf("expected closed after successes, got %d", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(60 * time.Millisecond)
	cb.Allow()

	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Fatalf("expected open after half-open failure, got %d", cb.State())
	}
}

func TestExecuteVoid_Success(t *testing.T) {
	opt := CallOption{
		Timeout:    5 * time.Second,
		MaxRetries: 0,
		RetryDelay: 10 * time.Millisecond,
		Name:       "test-void",
	}

	err := ExecuteVoid(context.Background(), opt, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestGetBreaker_SameInstance(t *testing.T) {
	breakersMu.Lock()
	delete(breakers, "same-test")
	breakersMu.Unlock()

	cb1 := GetBreaker("same-test")
	cb2 := GetBreaker("same-test")

	if cb1 != cb2 {
		t.Fatal("expected same circuit breaker instance for same name")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(100, time.Second)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow()
			cb.RecordSuccess()
		}()
	}

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cb.Allow()
			cb.RecordFailure()
		}()
	}

	wg.Wait()
}

func TestExecute_CircuitBreakerIntegration(t *testing.T) {
	breakersMu.Lock()
	delete(breakers, "cb-integration")
	breakersMu.Unlock()

	opt := CallOption{
		Timeout:    time.Second,
		MaxRetries: 0,
		RetryDelay: 10 * time.Millisecond,
		Name:       "cb-integration",
	}

	for i := 0; i < 5; i++ {
		Execute(context.Background(), opt, func(ctx context.Context) (string, error) {
			return "", errors.New("fail")
		})
	}

	cb := GetBreaker("cb-integration")
	if cb.State() != StateOpen {
		t.Fatalf("expected open state after 5 failures, got %d", cb.State())
	}

	_, err := Execute(context.Background(), opt, func(ctx context.Context) (string, error) {
		return "should not run", nil
	})
	if err == nil {
		t.Fatal("expected circuit breaker to reject")
	}
}
