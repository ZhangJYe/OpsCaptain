package indexer

import (
	embedder2 "SuperBizAgent/internal/ai/embedder"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	einoindexer "github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// MilvusFieldBuilder is a function type that builds Milvus entity fields from config.
// Injected from the infra layer to avoid direct import.
type MilvusFieldBuilder func(collectionName string) []*entity.Field

// MilvusIndexerConfig holds the configuration needed to create a Milvus indexer.
type MilvusIndexerConfig struct {
	Client         client.Client
	CollectionName string
	Fields         []*entity.Field
}

// NewMilvusIndexerWithConfig creates a Milvus indexer using pre-built config.
// This avoids a direct import of the infra layer.
func NewMilvusIndexerWithConfig(cfg MilvusIndexerConfig) func(ctx context.Context) (einoindexer.Indexer, error) {
	return func(ctx context.Context) (einoindexer.Indexer, error) {
		eb, err := embedder2.DoubaoEmbedding(ctx)
		if err != nil {
			return nil, err
		}
		config := &milvus.IndexerConfig{
			Client:            cfg.Client,
			Collection:        cfg.CollectionName,
			Fields:            cfg.Fields,
			Embedding:         eb,
			DocumentConverter: buildFloatVectorRows,
		}
		indexer, err := milvus.NewIndexer(ctx, config)
		if err != nil {
			return nil, err
		}
		return indexer, nil
	}
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
