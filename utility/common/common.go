package common

import (
	"context"
	"os"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	MilvusDBName         = "agent"
	MilvusCollectionName = "biz"
)

var (
	FileDir  = "./docs/"
	envVarRe = regexp.MustCompile(`^\$\{(\w+)\}$`)
)

func ResolveEnv(val string) string {
	m := envVarRe.FindStringSubmatch(val)
	if m == nil {
		return val
	}
	if v := os.Getenv(m[1]); v != "" {
		return v
	}
	return val
}

func GetMilvusAddr(ctx context.Context) string {
	val, err := g.Cfg().Get(ctx, "milvus.address")
	if err == nil {
		s := strings.TrimSpace(val.String())
		if s != "" && !IsEnvReference(s) {
			return normalizeMilvusAddr(s)
		}
	}
	// fallback: 直接读环境变量
	if env := strings.TrimSpace(os.Getenv("MILVUS_ADDRESS")); env != "" {
		return normalizeMilvusAddr(env)
	}
	return "localhost:19530"
}

func GetMilvusCollectionName(ctx context.Context) string {
	if v, err := g.Cfg().Get(ctx, "milvus.collection"); err == nil {
		if resolved, ok := ResolveOptionalEnv(v.String()); ok {
			return resolved
		}
		if trimmed := strings.TrimSpace(v.String()); trimmed != "" && !IsEnvReference(trimmed) {
			return trimmed
		}
	}
	if env := strings.TrimSpace(os.Getenv("MILVUS_COLLECTION")); env != "" {
		return env
	}
	return MilvusCollectionName
}

func normalizeMilvusAddr(raw string) string {
	if resolved, ok := ResolveOptionalEnv(raw); ok {
		return resolved
	}
	return "localhost:19530"
}

func GetVectorDimension(ctx context.Context) int {
	val, err := g.Cfg().Get(ctx, "doubao_embedding_model.dimension")
	if err != nil || val.Int() == 0 {
		return 2048
	}
	return val.Int()
}

func GetRetrieverTopK(ctx context.Context) int {
	topK := 3
	if v, err := g.Cfg().Get(ctx, "retriever.top_k"); err == nil && v.Int() > 0 {
		topK = v.Int()
	}
	return topK
}

func GetMilvusIndexType(ctx context.Context) string {
	if v, err := g.Cfg().Get(ctx, "milvus.index_type"); err == nil && v.String() != "" {
		return v.String()
	}
	return "HNSW"
}

func GetMilvusMetricType(ctx context.Context) string {
	if v, err := g.Cfg().Get(ctx, "milvus.metric_type"); err == nil && v.String() != "" {
		return v.String()
	}
	return "IP"
}

func GetMilvusHNSWM(ctx context.Context) int {
	if v, err := g.Cfg().Get(ctx, "milvus.hnsw.m"); err == nil && v.Int() > 0 {
		return v.Int()
	}
	return 16
}

func GetMilvusHNSWEfConstruction(ctx context.Context) int {
	if v, err := g.Cfg().Get(ctx, "milvus.hnsw.ef_construction"); err == nil && v.Int() > 0 {
		return v.Int()
	}
	return 200
}
