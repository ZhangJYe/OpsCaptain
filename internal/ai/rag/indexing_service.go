package rag

import (
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	loader2 "SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/log_call_back"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

type IndexBuildSummary struct {
	SourcePath      string
	ResolvedSource  string
	DeletedExisting int
	ChunkIDs        []string
}

type IndexingService struct {
	buildPipeline func(context.Context) (compose.Runnable[document.Source, []string], error)
	newLoader     func(context.Context) (document.Loader, error)
	vectorStore   VectorStore
}

var defaultIndexingService = NewIndexingService()

func NewIndexingService() *IndexingService {
	return &IndexingService{
		buildPipeline: knowledge_index_pipeline.BuildKnowledgeIndexing,
		newLoader:     loader2.NewFileLoader,
	}
}

func NewIndexingServiceWithStore(store VectorStore) *IndexingService {
	return &IndexingService{
		buildPipeline: knowledge_index_pipeline.BuildKnowledgeIndexing,
		newLoader:     loader2.NewFileLoader,
		vectorStore:   store,
	}
}

func (s *IndexingService) SetVectorStore(store VectorStore) {
	s.vectorStore = store
}

func SetDefaultVectorStore(store VectorStore) {
	defaultIndexingService.SetVectorStore(store)
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
	ids, err := graph.Invoke(ctx, document.Source{URI: path}, compose.WithCallbacks(log_call_back.LogCallback(nil)))
	if err != nil {
		return IndexBuildSummary{}, fmt.Errorf("invoke index graph failed: %w", err)
	}

	deleted, err := s.deleteExistingSourceExcept(ctx, sourceValue, ids)
	if err != nil {
		return IndexBuildSummary{}, err
	}

	s.SyncBM25Index(ctx)

	return IndexBuildSummary{
		SourcePath:      path,
		ResolvedSource:  sourceValue,
		DeletedExisting: deleted,
		ChunkIDs:        ids,
	}, nil
}

func (s *IndexingService) SyncBM25Index(ctx context.Context) {
	loader, err := s.newLoader(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "SyncBM25Index: newLoader failed, BM25 index not rebuilt: %v", err)
		return
	}

	paths, err := collectBM25SourcePaths(common.FileDir)
	if err != nil {
		g.Log().Warningf(ctx, "collect bm25 source paths failed: %v", err)
		return
	}

	idx := NewBM25Index()
	totalDocs := 0
	for _, path := range paths {
		docs, err := loader.Load(ctx, document.Source{URI: path})
		if err != nil || len(docs) == 0 {
			if err != nil {
				g.Log().Warningf(ctx, "load bm25 source failed: %s, err=%v", path, err)
			}
			continue
		}
		for _, doc := range docs {
			AddDocToBM25Index(idx, doc)
			totalDocs++
		}
	}
	SetSharedBM25Index(idx)
	g.Log().Infof(ctx, "rebuilt BM25 index from %d files, docs=%d, total=%d", len(paths), totalDocs, idx.Size())
}

func (s *IndexingService) DeleteSource(ctx context.Context, sourceValue string) (int, error) {
	if s == nil {
		return 0, fmt.Errorf("indexing service is nil")
	}
	if strings.TrimSpace(sourceValue) == "" {
		return 0, fmt.Errorf("document source is empty")
	}
	if s.vectorStore == nil {
		return 0, fmt.Errorf("vector store not configured")
	}
	deleted, err := s.vectorStore.DeleteBySource(ctx, common.GetMilvusCollectionName(ctx), sourceValue)
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *IndexingService) deleteExistingSourceExcept(ctx context.Context, sourceValue string, keepIDs []string) (int, error) {
	if s.vectorStore == nil {
		return 0, fmt.Errorf("vector store not configured")
	}
	return s.vectorStore.DeleteBySourceExcept(ctx, common.GetMilvusCollectionName(ctx), sourceValue, keepIDs)
}

func resolveDocumentSource(path string, doc *schema.Document) string {
	if doc != nil && doc.MetaData != nil {
		if source, ok := doc.MetaData["_source"].(string); ok && strings.TrimSpace(source) != "" {
			return source
		}
	}
	return path
}

func collectBM25SourcePaths(root string) ([]string, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	paths := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && filepath.Base(path) == quarantineDirName() {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".metadata.json") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func quarantineDirName() string {
	return "quarantine"
}
