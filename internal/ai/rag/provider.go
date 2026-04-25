package rag

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gogf/gf/v2/frame/g"
)

type Provider struct {
	mu            sync.RWMutex
	pool          *RetrieverPool
	bm25          *BM25Index
	pipeline      Pipeline
	configVersion atomic.Int64
	lastVersion   int64
}

var globalProvider = &Provider{}

func GetProvider() *Provider { return globalProvider }

func (p *Provider) Pool() *RetrieverPool {
	p.mu.RLock()
	pool := p.pool
	p.mu.RUnlock()
	if pool != nil {
		return pool
	}
	return p.initPool()
}

func (p *Provider) BM25() *BM25Index {
	p.mu.RLock()
	idx := p.bm25
	p.mu.RUnlock()
	if idx != nil {
		return idx
	}
	return p.initBM25()
}

func (p *Provider) Pipeline() Pipeline {
	p.mu.RLock()
	pl := p.pipeline
	p.mu.RUnlock()
	if pl != nil {
		return pl
	}
	return p.initPipeline()
}

func (p *Provider) Invalidate() {
	p.configVersion.Add(1)
	p.mu.Lock()
	if p.pool != nil {
		p.pool.Reset()
	}
	p.pool = nil
	p.bm25 = nil
	p.pipeline = nil
	p.mu.Unlock()
}

func (p *Provider) InvalidateIfVersionChanged() bool {
	current := p.configVersion.Load()
	p.mu.RLock()
	last := p.lastVersion
	p.mu.RUnlock()
	if current != last {
		p.mu.Lock()
		if p.pool != nil {
			p.pool.Reset()
		}
		p.pool = nil
		p.bm25 = nil
		p.pipeline = nil
		p.lastVersion = current
		p.mu.Unlock()
		return true
	}
	return false
}

func (p *Provider) SetPipeline(pl Pipeline) {
	p.mu.Lock()
	p.pipeline = pl
	p.mu.Unlock()
}

func (p *Provider) SetPool(pool *RetrieverPool) {
	p.mu.Lock()
	p.pool = pool
	p.mu.Unlock()
}

func (p *Provider) SetBM25(idx *BM25Index) {
	p.mu.Lock()
	p.bm25 = idx
	p.mu.Unlock()
}

func (p *Provider) initPool() *RetrieverPool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pool != nil {
		return p.pool
	}
	p.pool = NewRetrieverPool(NewRetrieverFunc, DefaultRetrieverCacheKey, sharedInitFailureTTL)
	return p.pool
}

func (p *Provider) initBM25() *BM25Index {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bm25 != nil {
		return p.bm25
	}
	p.bm25 = NewBM25Index()
	return p.bm25
}

func (p *Provider) initPipeline() Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pipeline != nil {
		return p.pipeline
	}
	p.pipeline = resolvePipeline(context.Background())
	return p.pipeline
}

func resolvePipeline(ctx context.Context) Pipeline {
	v, err := g.Cfg().Get(ctx, "rag.pipeline")
	if err == nil {
		switch strings.ToLower(strings.TrimSpace(v.String())) {
		case "dense", "dense_only", "dense-only":
			return &DenseOnlyPipeline{}
		}
	}
	return &HybridPipeline{}
}

func ResetProvider() {
	globalProvider.Invalidate()
}
