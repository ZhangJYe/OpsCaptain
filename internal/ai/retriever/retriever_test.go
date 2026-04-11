package retriever

import (
	"context"
	"testing"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func TestFloatVectorConverter_ProducesMilvusFloatVectors(t *testing.T) {
	vectors, err := floatVectorConverter(context.Background(), [][]float64{{1.5, -2.25, 3}})
	if err != nil {
		t.Fatalf("floatVectorConverter returned error: %v", err)
	}
	if len(vectors) != 1 {
		t.Fatalf("expected 1 vector, got %d", len(vectors))
	}

	floatVector, ok := vectors[0].(entity.FloatVector)
	if !ok {
		t.Fatalf("expected entity.FloatVector, got %T", vectors[0])
	}
	if len(floatVector) != 3 {
		t.Fatalf("expected len 3, got %d", len(floatVector))
	}
	if floatVector[0] != float32(1.5) || floatVector[1] != float32(-2.25) || floatVector[2] != float32(3) {
		t.Fatalf("unexpected float vector: %#v", floatVector)
	}
}

func TestResolveMilvusSearchParam_HNSW(t *testing.T) {
	param, err := resolveMilvusSearchParam(context.Background(), 5)
	if err != nil {
		t.Fatalf("resolveMilvusSearchParam returned error: %v", err)
	}
	if _, ok := param.(*entity.IndexHNSWSearchParam); !ok {
		t.Fatalf("expected *entity.IndexHNSWSearchParam, got %T", param)
	}
}
