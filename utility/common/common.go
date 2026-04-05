package common

import (
	"context"
	"os"
	"regexp"

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
	if err != nil || val.String() == "" {
		return "localhost:19530"
	}
	return val.String()
}

func GetVectorDimension(ctx context.Context) int {
	val, err := g.Cfg().Get(ctx, "doubao_embedding_model.dimension")
	if err != nil || val.Int() == 0 {
		return 2048
	}
	return val.Int()
}
