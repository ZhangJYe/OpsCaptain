package rag

import (
	"context"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
)

type QueryTrace struct {
	CacheKey          string
	CacheHit          bool
	InitFailureCached bool
	InitLatencyMs     int64
	RetrieveLatencyMs int64
	ResultCount       int
}

func Query(ctx context.Context, pool *RetrieverPool, query string) ([]*schema.Document, QueryTrace, error) {
	if strings.TrimSpace(query) == "" {
		return nil, QueryTrace{}, nil
	}

	rr, acquisition, err := pool.GetOrCreate(ctx)
	trace := QueryTrace{
		CacheKey:          acquisition.CacheKey,
		CacheHit:          acquisition.CacheHit,
		InitFailureCached: acquisition.InitFailureCached,
		InitLatencyMs:     acquisition.InitLatencyMs,
	}
	if err != nil {
		return nil, trace, err
	}

	retrieveStart := time.Now()
	docs, err := rr.Retrieve(ctx, query)
	trace.RetrieveLatencyMs = time.Since(retrieveStart).Milliseconds()
	trace.ResultCount = len(docs)
	return docs, trace, err
}
