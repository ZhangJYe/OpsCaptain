package main

import (
	"SuperBizAgent/internal/ai/agent/knowledge_index_pipeline"
	loader2 "SuperBizAgent/internal/ai/loader"
	"SuperBizAgent/utility/client"
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/log_call_back"
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
)

func main() {
	ctx := context.Background()
	r, err := knowledge_index_pipeline.BuildKnowledgeIndexing(ctx)
	if err != nil {
		panic(err)
	}
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
		loader, err := loader2.NewFileLoader(ctx)
		if err != nil {
			return err
		}
		docs, err := loader.Load(ctx, document.Source{URI: path})
		if err != nil {
			return err
		}
		cli, err := client.NewMilvusClient(ctx)
		if err != nil {
			return err
		}
		expr := fmt.Sprintf(`metadata["_source"] == "%s"`, docs[0].MetaData["_source"])
		queryResult, err := cli.Query(ctx, common.MilvusCollectionName, []string{}, expr, []string{"id"})
		if err != nil {
			return err
		} else if len(queryResult) > 0 {
			var idsToDelete []string
			for _, column := range queryResult {
				if column.Name() == "id" {
					for i := 0; i < column.Len(); i++ {
						id, err := column.GetAsString(i)
						if err == nil {
							idsToDelete = append(idsToDelete, id)
						}
					}
				}
			}
			if len(idsToDelete) > 0 {
				deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(idsToDelete, `","`))
				err = cli.Delete(ctx, common.MilvusCollectionName, "", deleteExpr)
				if err != nil {
					g.Log().Warningf(ctx, "delete existing data failed: %v", err)
				} else {
					g.Log().Infof(ctx, "deleted %d existing records with _source: %s", len(idsToDelete), docs[0].MetaData["_source"])
				}
			}
		}
		ids, err := r.Invoke(ctx, document.Source{URI: path}, compose.WithCallbacks(log_call_back.LogCallback(nil)))
		if err != nil {
			return fmt.Errorf("invoke index graph failed: %w", err)
		}
		g.Log().Infof(ctx, "done indexing file: %s, len of parts: %d, %s", path, len(ids), ids)
		return nil
	})
	if err != nil {
		g.Log().Errorf(ctx, "walk dir error: %v", err)
	}
}
