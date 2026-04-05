package rag

import (
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const DefaultRetrieverInitFailureTTL = 15 * time.Second

func RetrieverTopK(ctx context.Context) int {
	topK := 3
	if v, err := g.Cfg().Get(ctx, "retriever.top_k"); err == nil && v.Int() > 0 {
		topK = v.Int()
	}
	return topK
}

func DefaultRetrieverCacheKey(ctx context.Context) string {
	return fmt.Sprintf("%s|%d", common.GetMilvusAddr(ctx), RetrieverTopK(ctx))
}

func DefaultInitFailureTTL(context.Context) time.Duration {
	return DefaultRetrieverInitFailureTTL
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
