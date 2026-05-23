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
	if dim != 1024 {
		t.Logf("vector dimension from config: %d", dim)
	}
}

func TestConstants(t *testing.T) {
	if MilvusDBName != "agent" {
		t.Fatalf("expected MilvusDBName 'agent', got '%s'", MilvusDBName)
	}
	if MilvusCollectionName != "opscaption_knowledge_v2" {
		t.Fatalf("expected MilvusCollectionName 'opscaption_knowledge_v2', got '%s'", MilvusCollectionName)
	}
}

func TestGetMilvusCollectionName_Default(t *testing.T) {
	ctx := context.Background()
	if got := GetMilvusCollectionName(ctx); got != "opscaption_knowledge_v2" {
		t.Fatalf("expected default collection 'opscaption_knowledge_v2', got %q", got)
	}
}

func TestGetMilvusCollectionName_FromEnv(t *testing.T) {
	t.Setenv("MILVUS_COLLECTION", "aiops-evidence")
	ctx := context.Background()
	if got := GetMilvusCollectionName(ctx); got != "aiops-evidence" {
		t.Fatalf("expected env collection override, got %q", got)
	}
}

func TestGetMilvusAddrFallsBackWhenPlaceholderIsUnresolved(t *testing.T) {
	t.Setenv("MILVUS_ADDRESS", "")
	if got := normalizeMilvusAddr("${MILVUS_ADDRESS}"); got != "localhost:19530" {
		t.Fatalf("expected unresolved placeholder to fall back, got %q", got)
	}
}
