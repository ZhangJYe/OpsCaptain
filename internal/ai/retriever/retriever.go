package retriever

import (
	"SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

type safeRetriever struct {
	inner retriever.Retriever
}

func (s *safeRetriever) Retrieve(ctx context.Context, query string, opts ...retriever.Option) ([]*schema.Document, error) {
	docs, err := s.inner.Retrieve(ctx, query, opts...)
	if err != nil && strings.Contains(err.Error(), "extra output fields") && strings.Contains(err.Error(), "does not dynamic field") {
		g.Log().Debugf(ctx, "milvus retriever returned empty-collection error, treating as empty result: %v", err)
		return nil, nil
	}
	return docs, err
}

func NewMilvusRetriever(ctx context.Context) (rtr retriever.Retriever, err error) {
	cli, err := client.NewMilvusClient(ctx)
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
	r, err := milvus.NewRetriever(ctx, &milvus.RetrieverConfig{
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
