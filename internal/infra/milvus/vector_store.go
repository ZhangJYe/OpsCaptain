package milvus

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
)

type milvusQueryDeleter interface {
	Query(ctx context.Context, collectionName string, partitionNames []string, expr string, outputFields []string, opts ...cli.SearchQueryOptionFunc) (cli.ResultSet, error)
	Delete(ctx context.Context, collName string, partitionName string, expr string) error
}

type MilvusVectorStore struct {
	newClient func(ctx context.Context) (milvusQueryDeleter, error)
}

func NewMilvusVectorStore(clientFactory func(ctx context.Context) (cli.Client, error)) *MilvusVectorStore {
	return &MilvusVectorStore{
		newClient: func(ctx context.Context) (milvusQueryDeleter, error) {
			return clientFactory(ctx)
		},
	}
}

func (s *MilvusVectorStore) DeleteBySource(ctx context.Context, collection string, sourceValue string) (int, error) {
	return s.DeleteBySourceExcept(ctx, collection, sourceValue, nil)
}

func (s *MilvusVectorStore) DeleteBySourceExcept(ctx context.Context, collection string, sourceValue string, keepIDs []string) (int, error) {
	c, err := s.newClient(ctx)
	if err != nil {
		return 0, err
	}

	expr := fmt.Sprintf(`metadata["_source"] == "%s"`, escapeMilvusStringLiteral(sourceValue))
	queryResult, err := c.Query(ctx, collection, []string{}, expr, []string{"id"})
	if err != nil {
		return 0, err
	}

	var idsToDelete []string
	keep := make(map[string]struct{}, len(keepIDs))
	for _, id := range keepIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			keep[id] = struct{}{}
		}
	}
	for _, column := range queryResult {
		if column.Name() != "id" {
			continue
		}
		for i := 0; i < column.Len(); i++ {
			id, getErr := column.GetAsString(i)
			if getErr == nil && id != "" {
				if _, ok := keep[id]; ok {
					continue
				}
				idsToDelete = append(idsToDelete, id)
			}
		}
	}
	if len(idsToDelete) == 0 {
		return 0, nil
	}

	escapedIDs := make([]string, len(idsToDelete))
	for i, id := range idsToDelete {
		escapedIDs[i] = escapeMilvusStringLiteral(id)
	}
	deleteExpr := fmt.Sprintf(`id in ["%s"]`, strings.Join(escapedIDs, `","`))
	if err := c.Delete(ctx, collection, "", deleteExpr); err != nil {
		return 0, fmt.Errorf("delete existing records failed: %w", err)
	}
	g.Log().Infof(ctx, "deleted %d existing records with _source: %s, keep=%d", len(idsToDelete), sourceValue, len(keep))
	return len(idsToDelete), nil
}

func escapeMilvusStringLiteral(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
