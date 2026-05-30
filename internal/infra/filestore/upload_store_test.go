package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal.txt", "normal.txt"},
		{"../../../etc/passwd", "passwd"},
		{"file with spaces.md", "file_with_spaces.md"},
		{"<script>alert.js</script>.txt", "script_.txt"},
		{"", "unnamed"},
		{".", "unnamed"},
		{"hello世界.pdf", "hello__.pdf"},
		{"path/to/file.txt", "file.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := sanitizeFilename(tc.input)
			if result != tc.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestLocalUploadStoreSaveUploadDeduplicatesBySourceAndHash(t *testing.T) {
	store := NewLocalUploadStore(t.TempDir())
	ctx := context.Background()
	input := UploadSaveInput{
		Filename:     "hello world.md",
		MIMEType:     "text/markdown",
		Size:         int64(len("hello")),
		Content:      []byte("hello"),
		SourceKind:   "chat_upload",
		SourcePrefix: "upload://",
	}

	first, err := store.SaveUpload(ctx, input)
	if err != nil {
		t.Fatalf("save first upload: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first upload should not be duplicate")
	}
	if first.FileName != "hello_world.md" {
		t.Fatalf("unexpected safe filename: %q", first.FileName)
	}
	if err := store.MarkUploadStatus(ctx, first.FileID, "ready"); err != nil {
		t.Fatalf("mark ready: %v", err)
	}

	second, err := store.SaveUpload(ctx, input)
	if err != nil {
		t.Fatalf("save duplicate upload: %v", err)
	}
	if !second.Duplicate {
		t.Fatal("expected duplicate upload")
	}
	if second.FileID != first.FileID {
		t.Fatalf("expected duplicate file id %q, got %q", first.FileID, second.FileID)
	}
	if second.Status != "ready" {
		t.Fatalf("expected ready duplicate status, got %q", second.Status)
	}
}

func TestLocalUploadStoreCleanupReplacedUploadsFiltersSourceKind(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalUploadStore(dir)
	keepPath := filepath.Join(dir, "keep.md")
	oldPath := filepath.Join(dir, "old.md")
	manualPath := filepath.Join(dir, "manual.md")
	for _, path := range []string{keepPath, oldPath, manualPath} {
		if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := writeUploadMetadata(keepPath, uploadFileRecord{SourceKind: "chat_upload", SourceKey: "upload://same.md", StoredFilename: "keep.md", Version: 2}); err != nil {
		t.Fatalf("write keep metadata: %v", err)
	}
	if err := writeUploadMetadata(oldPath, uploadFileRecord{SourceKind: "chat_upload", SourceKey: "upload://same.md", StoredFilename: "old.md", Version: 1}); err != nil {
		t.Fatalf("write old metadata: %v", err)
	}
	if err := writeUploadMetadata(manualPath, uploadFileRecord{SourceKind: "manual", SourceKey: "upload://same.md", StoredFilename: "manual.md", Version: 1}); err != nil {
		t.Fatalf("write manual metadata: %v", err)
	}

	if err := store.CleanupReplacedUploads(context.Background(), "chat_upload", "upload://same.md", "keep.md"); err != nil {
		t.Fatalf("cleanup replaced uploads: %v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("keep file should remain: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed, err=%v", err)
	}
	if _, err := os.Stat(manualPath); err != nil {
		t.Fatalf("manual file should remain: %v", err)
	}
}

func TestNextUploadVersion(t *testing.T) {
	version := nextUploadVersion([]uploadFileRecord{
		{Version: 1},
		{Version: 3},
		{Version: 2},
	})
	if version != 4 {
		t.Fatalf("expected version 4, got %d", version)
	}
}
