package rag

import (
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	loader2 "SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/log_call_back"
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
)

type IndexBuildSummary struct {
	SourcePath      string
	ResolvedSource  string
	DeletedExisting int
	ChunkIDs        []string
}

type IndexingService struct {
	buildPipeline   func(context.Context) (compose.Runnable[document.Source, []string], error)
	newLoader       func(context.Context) (document.Loader, error)
	newMilvusClient func(context.Context) (milvusclient.Client, error)
}

var defaultIndexingService = NewIndexingService()

func NewIndexingService() *IndexingService {
	return &IndexingService{
		buildPipeline:   knowledge_index_pipeline.BuildKnowledgeIndexing,
		newLoader:       loader2.NewFileLoader,
		newMilvusClient: client.NewMilvusClient,
	}
}

func DefaultIndexingService() *IndexingService {
	return defaultIndexingService
}

func (s *IndexingService) IndexSource(ctx context.Context, path string) (IndexBuildSummary, error) {
	if s == nil {
		return IndexBuildSummary{}, fmt.Errorf("indexing service is nil")
	}

	graph, err := s.buildPipeline(ctx)
	if err != nil {
		return IndexBuildSummary{}, fmt.Errorf("build knowledge indexing failed: %w", err)
	}
	loader, err := s.newLoader(ctx)
	if err != nil {
		return IndexBuildSummary{}, err
	}
	docs, err := loader.Load(ctx, document.Source{URI: path})
	if err != nil {
		return IndexBuildSummary{}, err
	}
	if len(docs) == 0 {
		return IndexBuildSummary{}, fmt.Errorf("loader returned no documents for file: %s", path)
	}

	sourceValue := resolveDocumentSource(path, docs[0])
	deleted, err := s.deleteExistingSource(ctx, sourceValue)
	if err != nil {
		return IndexBuildSummary{}, err
	}

	ids, err := graph.Invoke(ctx, document.Source{URI: path}, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		return IndexBuildSummary{}, fmt.Errorf("invoke index graph failed: %w", err)
	}

	return IndexBuildSummary{
		SourcePath:      path,
		ResolvedSource:  sourceValue,
		DeletedExisting: deleted,
		ChunkIDs:        ids,
	}, nil
}

func (s *IndexingService) deleteExistingSource(ctx context.Context, sourceValue string) (int, error) {
	cli, err := s.newMilvusClient(ctx)
	if err != nil {
		return 0, err
	}
	expr := fmt.Sprintf(`metadata["_source"] == "%s"`, sourceValue)
	queryResult, err := cli.Query(ctx, common.MilvusCollectionName, []string{}, expr, []string{"id"})
	if err != nil {
		return 0, err
	}

	var idsToDelete []string
	for _, column := range queryResult {
		if column.Name() != "id" {
			continue
		}
		for i := 0; i < column.Len(); i++ {
			id, getErr := column.GetAsString(i)
			if getErr == nil && id != "" {
				idsToDelete = append(idsToDelete, id)
			}
		}
	}
	if len(idsToDelete) == 0 {
		return 0, nil
	}

	deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
	if err := cli.Delete(ctx, common.MilvusCollectionName, "", deleteExpr); err != nil {
		g.Log().Warningf(ctx, "delete existing data failed: %v", err)
		return 0, nil
	}
	g.Log().Infof(ctx, "deleted %d existing records with _source: %s", len(idsToDelete), sourceValue)
	return len(idsToDelete), nil
}

func resolveDocumentSource(path string, doc *schema.Document) string {
	if doc != nil && doc.MetaData != nil {
		if source, ok := doc.MetaData["_source"].(string); ok && strings.TrimSpace(source) != "" {
			return source
		}
	}
	return path
}
