package indexer

import (
	embedder2 "SuperBizAgent/internal/ai/embedder"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	"github.com/cloudwego/eino/schema"
)

func NewMilvusIndexer(ctx context.Context) (*milvus.Indexer, error) {
	cli, err := client.NewMilvusClient(ctx)
	if err != nil {
		return nil, err
	}
	eb, err := embedder2.DoubaoEmbedding(ctx)
	if err != nil {
		return nil, err
	}
	config := &milvus.IndexerConfig{
		Client:            cli,
		Collection:        common.GetMilvusCollectionName(ctx),
		Fields:            client.BuildMilvusFields(ctx),
		Embedding:         eb,
		DocumentConverter: buildFloatVectorRows,
	}
	indexer, err := milvus.NewIndexer(ctx, config)
	if err != nil {
		return nil, err
	}
	return indexer, nil
}

type floatVectorRow struct {
	ID       string    `json:"id" milvus:"name:id"`
	Content  string    `json:"content" milvus:"name:content"`
	Vector   []float32 `json:"vector" milvus:"name:vector"`
	Metadata []byte    `json:"metadata" milvus:"name:metadata"`
}

func buildFloatVectorRows(ctx context.Context, docs []*schema.Document, vectors [][]float64) ([]interface{}, error) {
	if len(docs) != len(vectors) {
		return nil, fmt.Errorf("document/vector length mismatch: docs=%d vectors=%d", len(docs), len(vectors))
	}

	rows := make([]interface{}, 0, len(docs))
	for i, doc := range docs {
		metadata, err := json.Marshal(doc.MetaData)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata for doc %s: %w", doc.ID, err)
		}
		rows = append(rows, &floatVectorRow{
			ID:       doc.ID,
			Content:  doc.Content,
			Vector:   toFloat32Vector(vectors[i]),
			Metadata: metadata,
		})
	}
	return rows, nil
}

func toFloat32Vector(vector []float64) []float32 {
	out := make([]float32, len(vector))
	for i, v := range vector {
		out[i] = float32(v)
	}
	return out
}
