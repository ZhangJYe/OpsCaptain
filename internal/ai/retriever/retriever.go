package retriever

import (
	"SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/internal/consts"
	inframv "SuperBizAgent/internal/infra/milvus"
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"strings"

	milvusretriever "github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

type safeRetriever struct {
	inner retriever.Retriever
}

func (s *safeRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	filter := ownerScopeFilterFromContext(ctx)
	scopedOpts := opts
	if filter != "" {
		scopedOpts = append(append([]retriever.Option{}, opts...), milvusretriever.WithFilter(filter))
	}
	docs, err := s.inner.Retrieve(ctx, query, scopedOpts...)
	if err != nil && filter != "" && shouldRetryWithoutOwnerScopeFilter(err) {
		g.Log().Warningf(ctx, "milvus owner scope filter failed, retrying without server filter: %v", err)
		docs, err = s.inner.Retrieve(ctx, query, opts...)
	}
	if err != nil && strings.Contains(err.Error(), "extra output fields") && strings.Contains(err.Error(), "does not dynamic field") {
		g.Log().Warningf(ctx, "milvus retriever schema mismatch: %v", err)
		return nil, fmt.Errorf("milvus collection schema mismatch: %w", err)
	}
	return docs, err
}

func NewMilvusRetriever(ctx context.Context) (rtr retriever.Retriever, err error) {
	cli, err := inframv.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}
	eb, err := embedder.DoubaoEmbedding(ctx)
	if err != nil {
		return nil, err
	}
	topK := common.GetRetrieverTopK(ctx)
	metricType, err := resolveMilvusMetricType(ctx)
	if err != nil {
		return nil, err
	}
	searchParam, err := resolveMilvusSearchParam(ctx, topK)
	if err != nil {
		return nil, err
	}
	r, err := milvusretriever.NewRetriever(ctx, &milvusretriever.RetrieverConfig{
		Client:      cli,
		Collection:  common.GetMilvusCollectionName(ctx),
		VectorField: "vector",
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		TopK:            topK,
		MetricType:      metricType,
		VectorConverter: floatVectorConverter,
		Sp:              searchParam,
		Embedding:       eb,
	})
	if err != nil {
		return nil, err
	}
	return &safeRetriever{inner: r}, nil
}

func floatVectorConverter(ctx context.Context, vectors [][]float64) ([]entity.Vector, error) {
	out := make([]entity.Vector, 0, len(vectors))
	for _, vector := range vectors {
		out = append(out, entity.FloatVector(toFloat32Vector(vector)))
	}
	return out, nil
}

func toFloat32Vector(vector []float64) []float32 {
	out := make([]float32, len(vector))
	for i, v := range vector {
		out[i] = float32(v)
	}
	return out
}

func resolveMilvusMetricType(ctx context.Context) (entity.MetricType, error) {
	switch strings.ToUpper(strings.TrimSpace(common.GetMilvusMetricType(ctx))) {
	case "IP":
		return entity.IP, nil
	case "L2":
		return entity.L2, nil
	case "COSINE":
		return entity.COSINE, nil
	default:
		return "", fmt.Errorf("unsupported milvus.metric_type: %s", common.GetMilvusMetricType(ctx))
	}
}

func resolveMilvusSearchParam(ctx context.Context, topK int) (entity.SearchParam, error) {
	switch strings.ToUpper(strings.TrimSpace(common.GetMilvusIndexType(ctx))) {
	case "HNSW":
		ef := topK * 16
		if ef < 64 {
			ef = 64
		}
		return entity.NewIndexHNSWSearchParam(ef)
	case "AUTOINDEX", "AUTO":
		return entity.NewIndexAUTOINDEXSearchParam(1)
	case "FLAT":
		return entity.NewIndexFlatSearchParam()
	default:
		return nil, fmt.Errorf("unsupported milvus.index_type: %s", common.GetMilvusIndexType(ctx))
	}
}

func ownerScopeFilterFromContext(ctx context.Context) string {
	if !ownerScopeServerFilterEnabled(ctx) {
		return ""
	}
	userID, _ := ctx.Value(consts.CtxKeyUserID).(string)
	prefix := common.KnowledgeSourcePrefixForUser(userID)
	if prefix == common.KnowledgeSourceBase {
		return ""
	}
	current := escapeMilvusStringLiteral(prefix + "%")
	userScoped := escapeMilvusStringLiteral(common.KnowledgeSourceBase + "users/%")
	return fmt.Sprintf(`(metadata["_source"] like "%s" or not metadata["_source"] like "%s")`, current, userScoped)
}

func ownerScopeServerFilterEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "rag.owner_scope_server_filter_enabled")
	if err != nil {
		return true
	}
	return v.Bool()
}

func shouldRetryWithoutOwnerScopeFilter(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "extra output fields") && strings.Contains(msg, "does not dynamic field") {
		return false
	}
	for _, marker := range []string{"filter", "expr", "expression", "query plan", "parse"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func escapeMilvusStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
