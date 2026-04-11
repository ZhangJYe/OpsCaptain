package indexer

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestBuildFloatVectorRows_UsesFloat32Vectors(t *testing.T) {
	docs := []*schema.Document{
		{
			ID:      "doc-1",
			Content: "hello",
			MetaData: map[string]any{
				"_source": "docs/test.md",
				"rank":    1,
			},
		},
	}
	vectors := [][]float64{{1.5, -2.25, 3}}

	rows, err := buildFloatVectorRows(context.Background(), docs, vectors)
	if err != nil {
		t.Fatalf("buildFloatVectorRows returned error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row, ok := rows[0].(*floatVectorRow)
	if !ok {
		t.Fatalf("expected *floatVectorRow, got %T", rows[0])
	}
	if got, want := len(row.Vector), 3; got != want {
		t.Fatalf("expected vector length %d, got %d", want, got)
	}
	if got := row.Vector; got[0] != float32(1.5) || got[1] != float32(-2.25) || got[2] != float32(3) {
		t.Fatalf("unexpected float32 vector: %#v", got)
	}
	if string(row.Metadata) == "" {
		t.Fatal("expected marshaled metadata")
	}
}

func TestBuildFloatVectorRows_LengthMismatch(t *testing.T) {
	docs := []*schema.Document{{ID: "doc-1"}}
	vectors := [][]float64{}

	if _, err := buildFloatVectorRows(context.Background(), docs, vectors); err == nil {
		t.Fatal("expected length mismatch error")
	}
}
