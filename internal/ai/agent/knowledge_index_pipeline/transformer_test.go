package knowledge_index_pipeline

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestCompactDocumentsDropsEmptyChunks(t *testing.T) {
	docs := []*schema.Document{
		nil,
		{ID: "empty", Content: " \n\t "},
		{ID: "ok", Content: "  hello  "},
	}

	got := compactDocuments(docs)
	if len(got) != 1 {
		t.Fatalf("expected 1 document, got %d", len(got))
	}
	if got[0].ID != "ok" || got[0].Content != "hello" {
		t.Fatalf("unexpected document: %#v", got[0])
	}
}
