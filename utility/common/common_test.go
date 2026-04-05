package common

import (
	"context"
	"testing"
)

func TestGetMilvusAddr_Default(t *testing.T) {
	ctx := context.Background()
	addr := GetMilvusAddr(ctx)
	if addr == "" {
		t.Fatal("expected non-empty address")
	}
	if addr != "localhost:19530" {
		t.Logf("milvus address from config: %s", addr)
	}
}

func TestGetVectorDimension_Default(t *testing.T) {
	ctx := context.Background()
	dim := GetVectorDimension(ctx)
	if dim <= 0 {
		t.Fatalf("expected positive dimension, got %d", dim)
	}
	if dim != 2048 {
		t.Logf("vector dimension from config: %d", dim)
	}
}

func TestConstants(t *testing.T) {
	if MilvusDBName != "agent" {
		t.Fatalf("expected MilvusDBName 'agent', got '%s'", MilvusDBName)
	}
	if MilvusCollectionName != "biz" {
		t.Fatalf("expected MilvusCollectionName 'biz', got '%s'", MilvusCollectionName)
	}
}
