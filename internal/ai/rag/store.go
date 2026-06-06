package rag

import "context"

type VectorStore interface {
	DeleteBySource(ctx context.Context, collection string, sourceValue string) (int, error)
	DeleteBySourceExcept(ctx context.Context, collection string, sourceValue string, keepIDs []string) (int, error)
}
