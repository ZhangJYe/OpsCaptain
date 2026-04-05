package rag

import (
	"context"
	"errors"
	"testing"
	"time"

	retrieverapi "github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
)

type fakeRetriever struct{}

func (f *fakeRetriever) Retrieve(context.Context, string, ...retrieverapi.Option) ([]*schema.Document, error) {
	return nil, nil
}

func TestRetrieverPoolReusesRetrieverByCacheKey(t *testing.T) {
	created := 0
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) {
			created++
			return &fakeRetriever{}, nil
		},
		func(context.Context) string { return "milvus|3" },
		func(context.Context) time.Duration { return time.Second },
	)

	for i := 0; i < 2; i++ {
		_, acquisition, err := pool.GetOrCreate(context.Background())
		if err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
		if i == 0 && acquisition.CacheHit {
			t.Fatalf("expected first acquisition to miss cache")
		}
		if i == 1 && !acquisition.CacheHit {
			t.Fatalf("expected second acquisition to hit cache")
		}
	}

	if created != 1 {
		t.Fatalf("expected one retriever creation, got %d", created)
	}
}

func TestRetrieverPoolCachesRecentInitFailures(t *testing.T) {
	created := 0
	pool := NewRetrieverPool(
		func(context.Context) (retrieverapi.Retriever, error) {
			created++
			return nil, errors.New("dial timeout")
		},
		func(context.Context) string { return "milvus|3" },
		func(context.Context) time.Duration { return time.Minute },
	)

	for i := 0; i < 2; i++ {
		_, acquisition, err := pool.GetOrCreate(context.Background())
		if err == nil {
			t.Fatalf("run %d: expected init failure", i+1)
		}
		if i == 1 && !acquisition.InitFailureCached {
			t.Fatalf("expected second acquisition to reuse cached init failure")
		}
	}

	if created != 1 {
		t.Fatalf("expected one failed creation, got %d", created)
	}
}
