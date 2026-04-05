package main

import (
	"SuperBizAgent/internal/ai/rag"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

func main() {
	ctx := context.Background()
	indexing := rag.DefaultIndexingService()
	var err error
	err = filepath.WalkDir("./docs", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk dir failed: %w", err)
		}
		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".md") {
			g.Log().Infof(ctx, "skip not a markdown file: %s", path)
			return nil
		}

		g.Log().Infof(ctx, "start indexing file: %s", path)
		summary, err := indexing.IndexSource(ctx, path)
		if err != nil {
			return err
		}
		g.Log().Infof(ctx, "done indexing file: %s, deleted=%d, len of parts: %d, %s", path, summary.DeletedExisting, len(summary.ChunkIDs), summary.ChunkIDs)
		return nil
	})
	if err != nil {
		g.Log().Errorf(ctx, "walk dir error: %v", err)
	}
}
