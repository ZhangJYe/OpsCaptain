package resilience

import (
	"SuperBizAgent/utility/metrics"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type CallOption struct {
	Timeout    time.Duration
	MaxRetries int
	RetryDelay time.Duration
	Name       string
}

var DefaultCallOption = CallOption{
	Timeout:    30 * time.Second,
	MaxRetries: 2,
	RetryDelay: time.Second,
	Name:       "unknown",
}

type CircuitState int

const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu              sync.Mutex
	name            string
	state           CircuitState
	failures        int
	successes       int
	threshold       int
	halfOpenMax     int
	resetTimeout    time.Duration
	lastFailureTime time.Time
}

func NewCircuitBreaker(threshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:        StateClosed,
		threshold:    threshold,
		halfOpenMax:  2,
		resetTimeout: resetTimeout,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.successes = 0
			cb.observeStateLocked()
			return true
		}
		return false
	case StateHalfOpen:
		return true
	}
	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.halfOpenMax {
			cb.state = StateClosed
			cb.failures = 0
			cb.observeStateLocked()
		}
	case StateClosed:
		cb.failures = 0
		cb.observeStateLocked()
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.threshold {
			cb.state = StateOpen
			cb.observeStateLocked()
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.observeStateLocked()
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

var (
	breakers   = make(map[string]*CircuitBreaker)
	breakersMu sync.RWMutex
)

func GetBreaker(name string) *CircuitBreaker {
	breakersMu.RLock()
	cb, ok := breakers[name]
	breakersMu.RUnlock()
	if ok {
		return cb
	}

	breakersMu.Lock()
	defer breakersMu.Unlock()
	if cb, ok := breakers[name]; ok {
		return cb
	}
	cb = NewCircuitBreaker(5, 30*time.Second)
	cb.name = name
	breakers[name] = cb
	metrics.SetCircuitBreakerState(name, breakerMetricValue(cb.State()))
	return cb
}

func Execute[T any](ctx context.Context, opt CallOption, fn func(ctx context.Context) (T, error)) (T, error) {
	opt = normalizeCallOption(opt)
	cb := GetBreaker(opt.Name)

	if !cb.Allow() {
		var zero T
		return zero, fmt.Errorf("[%s] circuit breaker open, service temporarily unavailable", opt.Name)
	}

	var lastErr error
	for attempt := 0; attempt <= opt.MaxRetries; attempt++ {
		if attempt > 0 {
			g.Log().Infof(ctx, "[resilience][%s] retry attempt %d/%d", opt.Name, attempt, opt.MaxRetries)
			if !sleepWithContext(ctx, opt.RetryDelay*time.Duration(attempt)) {
				var zero T
				return zero, ctx.Err()
			}
		}

		callCtx, cancel := context.WithTimeout(ctx, opt.Timeout)
		result, err := fn(callCtx)
		cancel()

		if err == nil {
			cb.RecordSuccess()
			return result, nil
		}

		lastErr = err
		g.Log().Warningf(ctx, "[resilience][%s] attempt %d failed: %v", opt.Name, attempt+1, err)
	}

	cb.RecordFailure()
	var zero T
	return zero, fmt.Errorf("[%s] all %d attempts failed, last error: %w", opt.Name, opt.MaxRetries+1, lastErr)
}

func ExecuteVoid(ctx context.Context, opt CallOption, fn func(ctx context.Context) error) error {
	_, err := Execute(ctx, opt, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, fn(ctx)
	})
	return err
}

func (cb *CircuitBreaker) observeStateLocked() {
	metrics.SetCircuitBreakerState(cb.name, breakerMetricValue(cb.state))
}

func breakerMetricValue(state CircuitState) float64 {
	switch state {
	case StateOpen:
		return 1
	case StateHalfOpen:
		return 2
	default:
		return 0
	}
}

func normalizeCallOption(opt CallOption) CallOption {
	if opt.Timeout <= 0 {
		opt.Timeout = DefaultCallOption.Timeout
	}
	if opt.MaxRetries < 0 {
		opt.MaxRetries = DefaultCallOption.MaxRetries
	}
	if opt.RetryDelay <= 0 {
		opt.RetryDelay = DefaultCallOption.RetryDelay
	}
	if opt.Name == "" {
		opt.Name = DefaultCallOption.Name
	}
	return opt
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
