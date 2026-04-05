package rag

import (
	"context"
	"errors"
	"sync"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
)

type RetrieverFactory func(context.Context) (retrieverapi.Retriever, error)

type CacheKeyFunc func(context.Context) string

type FailureTTLFunc func(context.Context) time.Duration

type RetrieverAcquisition struct {
	CacheKey          string
	CacheHit          bool
	InitFailureCached bool
	InitLatencyMs     int64
	FailureTTL        time.Duration
}

type RetrieverPool struct {
	mu         sync.Mutex
	factory    RetrieverFactory
	cacheKeyFn CacheKeyFunc
	ttlFn      FailureTTLFunc
	state      cachedRetriever
}

type cachedRetriever struct {
	key      string
	rr       retrieverapi.Retriever
	lastErr  error
	failedAt time.Time
}

func NewRetrieverPool(factory RetrieverFactory, cacheKeyFn CacheKeyFunc, ttlFn FailureTTLFunc) *RetrieverPool {
	if cacheKeyFn == nil {
		cacheKeyFn = DefaultRetrieverCacheKey
	}
	if ttlFn == nil {
		ttlFn = DefaultInitFailureTTL
	}
	return &RetrieverPool{
		factory:    factory,
		cacheKeyFn: cacheKeyFn,
		ttlFn:      ttlFn,
	}
}

func (p *RetrieverPool) GetOrCreate(ctx context.Context) (retrieverapi.Retriever, RetrieverAcquisition, error) {
	if p == nil {
		return nil, RetrieverAcquisition{}, errors.New("retriever pool is nil")
	}
	if p.factory == nil {
		return nil, RetrieverAcquisition{}, errors.New("retriever pool factory is nil")
	}

	cacheKey := p.cacheKeyFn(ctx)
	ttl := p.ttlFn(ctx)
	acquisition := RetrieverAcquisition{
		CacheKey:   cacheKey,
		FailureTTL: ttl,
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state.rr != nil && p.state.key == cacheKey {
		acquisition.CacheHit = true
		return p.state.rr, acquisition, nil
	}
	if p.state.key == cacheKey &&
		p.state.lastErr != nil &&
		time.Since(p.state.failedAt) < ttl {
		acquisition.InitFailureCached = true
		return nil, acquisition, p.state.lastErr
	}

	initStart := time.Now()
	rr, err := p.factory(ctx)
	acquisition.InitLatencyMs = time.Since(initStart).Milliseconds()
	if err != nil {
		p.state = cachedRetriever{
			key:      cacheKey,
			lastErr:  err,
			failedAt: time.Now(),
		}
		return nil, acquisition, err
	}

	p.state = cachedRetriever{
		key: cacheKey,
		rr:  rr,
	}
	return rr, acquisition, nil
}

func (p *RetrieverPool) Reset() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state = cachedRetriever{}
}
