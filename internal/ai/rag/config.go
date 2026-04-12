package rag

import (
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const DefaultRetrieverInitFailureTTL = 15 * time.Second

func RetrieverTopK(ctx context.Context) int {
	return common.GetRetrieverTopK(ctx)
}

func RetrieverCandidateTopK(ctx context.Context) int {
	topK := RetrieverTopK(ctx)
	if v, err := g.Cfg().Get(ctx, "retriever.candidate_top_k"); err == nil && v.Int() > 0 {
		if v.Int() < topK {
			return topK
		}
		return v.Int()
	}
	candidate := topK * 4
	if candidate < 20 {
		candidate = 20
	}
	if candidate > 50 {
		candidate = 50
	}
	if candidate < topK {
		return topK
	}
	return candidate
}

func DefaultRetrieverCacheKey(ctx context.Context) string {
	return fmt.Sprintf("%s|%s|%d", common.GetMilvusAddr(ctx), common.GetMilvusCollectionName(ctx), RetrieverTopK(ctx))
}

func DefaultQueryMode(ctx context.Context) QueryMode {
	v, err := g.Cfg().Get(ctx, "rag.default_query_mode")
	if err == nil {
		if raw := strings.TrimSpace(v.String()); raw != "" {
			if mode, parseErr := ParseQueryMode(raw); parseErr == nil {
				return mode
			}
		}
	}
	return QueryModeRetrieveOnly
}

func DefaultInitFailureTTL(context.Context) time.Duration {
	return DefaultRetrieverInitFailureTTL
}

func HybridEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "rag.hybrid_enabled")
	if err == nil {
		return v.Bool()
	}
	return false
}

func DurationFromConfig(ctx context.Context, fallback time.Duration, keys ...string) time.Duration {
	for _, key := range keys {
		if key == "" {
			continue
		}
		v, err := g.Cfg().Get(ctx, key)
		if err == nil && v.Int64() > 0 {
			return time.Duration(v.Int64()) * time.Millisecond
		}
	}
	return fallback
}
