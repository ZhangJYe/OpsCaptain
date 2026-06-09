package main

import (
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	"SuperBizAgent/internal/ai/indexer"
	"SuperBizAgent/internal/ai/rag"
	"SuperBizAgent/internal/ai/retriever"
	inframv "SuperBizAgent/internal/infra/milvus"
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:], rag.DefaultIndexingService(), os.Stdout); err != nil {
		g.Log().Errorf(ctx, "knowledge index failed: %v", err)
		os.Exit(1)
	}
}

func configureMilvusFactories(ctx context.Context) error {
	rag.SetDefaultVectorStore(inframv.NewMilvusVectorStore(inframv.NewMilvusClient))
	milvusClient, err := inframv.NewMilvusClient(ctx)
	if err != nil {
		return err
	}
	milvusCfg := inframv.MilvusConfigFromContext(ctx)
	knowledge_index_pipeline.NewIndexerFunc = indexer.NewMilvusIndexerWithConfig(indexer.MilvusIndexerConfig{
		Client:         milvusClient,
		CollectionName: milvusCfg.CollectionName,
		Fields:         inframv.BuildMilvusFields(milvusCfg),
	})
	rag.NewRetrieverFunc = retriever.NewMilvusRetrieverWithClient(milvusClient)
	return nil
}

type options struct {
	dir         string
	collection  string
	verifyQuery string
	dryRun      bool
}

func run(ctx context.Context, args []string, indexing *rag.IndexingService, out io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.collection) != "" {
		if err := os.Setenv("MILVUS_COLLECTION", strings.TrimSpace(opts.collection)); err != nil {
			return err
		}
	}

	paths, err := collectMarkdownFiles(opts.dir)
	if err != nil {
		return err
	}
	if opts.dryRun {
		_, _ = fmt.Fprintf(out, "dry_run=true collection=%s files=%d\n", opts.collection, len(paths))
		for _, path := range paths {
			_, _ = fmt.Fprintln(out, path)
		}
		return nil
	}
	if indexing == nil {
		return fmt.Errorf("indexing service is nil")
	}
	if err := configureMilvusFactories(ctx); err != nil {
		return fmt.Errorf("milvus client init failed: %w", err)
	}

	for _, path := range paths {
		g.Log().Infof(ctx, "start indexing file: %s", path)
		summary, err := indexing.IndexSource(ctx, path)
		if err != nil {
			return err
		}
		g.Log().Infof(ctx, "done indexing file: %s, deleted=%d, len of parts: %d, %s", path, summary.DeletedExisting, len(summary.ChunkIDs), summary.ChunkIDs)
	}

	if strings.TrimSpace(opts.verifyQuery) != "" {
		docs, _, err := rag.Query(ctx, rag.SharedPool(), strings.TrimSpace(opts.verifyQuery))
		if err != nil {
			return fmt.Errorf("verify query failed: %w", err)
		}
		if len(docs) == 0 {
			return fmt.Errorf("verify query returned no documents: %s", opts.verifyQuery)
		}
		_, _ = fmt.Fprintf(out, "verify_query=%q documents=%d\n", opts.verifyQuery, len(docs))
	}
	return nil
}

func parseOptions(args []string) (options, error) {
	fs := flag.NewFlagSet("knowledge-indexer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	opts := options{}
	fs.StringVar(&opts.dir, "dir", "./docs/knowledge", "directory containing markdown files to index")
	fs.StringVar(&opts.collection, "collection", "", "milvus collection override")
	fs.StringVar(&opts.verifyQuery, "verify-query", "", "query to verify indexed documents")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "scan files without indexing")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	return opts, nil
}

func collectMarkdownFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk dir failed: %w", err)
		}
		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		if strings.EqualFold(d.Name(), "README.md") {
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
