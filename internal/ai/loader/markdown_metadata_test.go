package loader

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestParseMarkdownFields(t *testing.T) {
	t.Parallel()
	raw := []byte("# Kubernetes Deployments\n\n## Upstream Metadata\n\n- Source: https://example.com/deployment.md\n- Provider: kubernetes\n- Tags: kubernetes, deployment, rollout\n")
	got := ParseMarkdownFields("/knowledge/kubernetes-deployment.md", raw)
	if got.DocumentID != "kubernetes-deployment" || got.Title != "Kubernetes Deployments" || got.Provider != "kubernetes" || got.Source != "https://example.com/deployment.md" {
		t.Fatalf("unexpected fields: %+v", got)
	}
	wantTags := []string{"kubernetes", "deployment", "rollout"}
	if !reflect.DeepEqual(got.Tags, wantTags) {
		t.Fatalf("tags=%v want=%v", got.Tags, wantTags)
	}
}

func TestKnowledgeCorpusMarkdownFields(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "..", "docs", "knowledge"))
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 30 {
		t.Fatalf("knowledge corpus markdown count=%d want=30", len(paths))
	}
	seenIDs := make(map[string]struct{}, len(paths))
	providerCount, tagCount := 0, 0
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		fields := ParseMarkdownFields(path, raw)
		if fields.DocumentID == "" || fields.Title == "" {
			t.Fatalf("missing stable id/title for %s: %+v", path, fields)
		}
		if _, duplicate := seenIDs[fields.DocumentID]; duplicate {
			t.Fatalf("duplicate knowledge document id %s", fields.DocumentID)
		}
		seenIDs[fields.DocumentID] = struct{}{}
		if fields.Provider != "" {
			providerCount++
		}
		if len(fields.Tags) > 0 {
			tagCount++
		}
	}
	if providerCount != 15 || tagCount != 15 {
		t.Fatalf("structured upstream coverage provider=%d tags=%d want=15/15", providerCount, tagCount)
	}
}

func TestParseMarkdownFieldsFallsBackForMissingFields(t *testing.T) {
	t.Parallel()
	got := ParseMarkdownFields("/knowledge/redis-sentinel.md", []byte("plain content"))
	if got.DocumentID != "redis-sentinel" || got.Title != "redis-sentinel" || got.Source != "/knowledge/redis-sentinel.md" || got.Provider != "" || len(got.Tags) != 0 {
		t.Fatalf("unexpected fallback fields: %+v", got)
	}
}

func TestEnrichMarkdownDocumentPreservesExistingMetadata(t *testing.T) {
	t.Parallel()
	doc := &schema.Document{MetaData: map[string]any{"title": "sidecar title", "_source": "upload://runbook.md"}}
	EnrichMarkdownDocument("/knowledge/runbook.md", []byte("# Parsed title\n- Provider: internal\n- Tags: redis, sentinel"), doc)
	if doc.MetaData["title"] != "sidecar title" || doc.MetaData["_source"] != "upload://runbook.md" {
		t.Fatalf("existing metadata was overwritten: %+v", doc.MetaData)
	}
	if doc.MetaData["knowledge_doc_id"] != "runbook" || doc.MetaData["provider"] != "internal" {
		t.Fatalf("parsed metadata missing: %+v", doc.MetaData)
	}
}
