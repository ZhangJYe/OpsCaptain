package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudwego/eino/components/document"
)

func TestNewFileLoader_MergesMetadataSidecar(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	docPath := filepath.Join(dir, "case-a.md")
	sidecarPath := filepath.Join(dir, "case-a.metadata.json")

	if err := os.WriteFile(docPath, []byte("# Case A\n\nhello"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	if err := os.WriteFile(sidecarPath, []byte(`{"_source":"upload://case-a.md","case_id":"case-a","service":"checkoutservice","instance_type":"service"}`), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	ldr, err := NewFileLoader(context.Background())
	if err != nil {
		t.Fatalf("NewFileLoader returned error: %v", err)
	}

	docs, err := ldr.Load(context.Background(), document.Source{URI: docPath})
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(docs) == 0 {
		t.Fatal("expected at least one loaded document")
	}

	if got := docs[0].MetaData["case_id"]; got != "case-a" {
		t.Fatalf("expected case_id sidecar metadata, got %#v", got)
	}
	if got := docs[0].MetaData["service"]; got != "checkoutservice" {
		t.Fatalf("expected service sidecar metadata, got %#v", got)
	}
	if got := docs[0].MetaData["instance_type"]; got != "service" {
		t.Fatalf("expected instance_type sidecar metadata, got %#v", got)
	}
	if got := docs[0].MetaData["_source"]; got != "upload://case-a.md" {
		t.Fatalf("expected sidecar _source override, got %#v", got)
	}
}

func TestNewFileLoader_InvalidMetadataSidecarFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	docPath := filepath.Join(dir, "case-b.md")
	sidecarPath := filepath.Join(dir, "case-b.metadata.json")

	if err := os.WriteFile(docPath, []byte("# Case B\n\nhello"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	if err := os.WriteFile(sidecarPath, []byte(`{"case_id":`), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	ldr, err := NewFileLoader(context.Background())
	if err != nil {
		t.Fatalf("NewFileLoader returned error: %v", err)
	}

	if _, err := ldr.Load(context.Background(), document.Source{URI: docPath}); err == nil {
		t.Fatal("expected invalid metadata sidecar error")
	}
}
