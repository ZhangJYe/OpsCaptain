package chat

import (
	"SuperBizAgent/utility/common"
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

func TestIsAllowedMIME(t *testing.T) {
	tests := []struct {
		mime    string
		allowed bool
	}{
		{"text/plain", true},
		{"text/markdown", true},
		{"application/pdf", true},
		{"application/json", true},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"application/msword", true},
		{"image/png", false},
		{"application/x-executable", false},
		{"application/javascript", false},
	}

	for _, tc := range tests {
		t.Run(tc.mime, func(t *testing.T) {
			result := isAllowedMIME(tc.mime)
			if result != tc.allowed {
				t.Errorf("isAllowedMIME(%q) = %v, want %v", tc.mime, result, tc.allowed)
			}
		})
	}
}

func TestAllowedExtensions(t *testing.T) {
	allowed := []string{".md", ".txt", ".pdf", ".doc", ".docx", ".csv", ".json", ".yaml", ".yml"}
	rejected := []string{".exe", ".sh", ".bat", ".js", ".html", ".php", ".go", ".py"}

	for _, ext := range allowed {
		if !allowedExtensions[ext] {
			t.Errorf("extension %s should be allowed", ext)
		}
	}
	for _, ext := range rejected {
		if allowedExtensions[ext] {
			t.Errorf("extension %s should not be allowed", ext)
		}
	}
}

func TestAllowedExtensionList(t *testing.T) {
	list := allowedExtensionList()
	if len(list) != len(allowedExtensions) {
		t.Errorf("expected %d extensions, got %d", len(allowedExtensions), len(list))
	}
}

func TestBuildUploadSourceKey(t *testing.T) {
	if got := buildUploadSourceKey("runbook.md"); got != "upload://runbook.md" {
		t.Fatalf("unexpected source key: %q", got)
	}
}

func TestListUploadRecordsBySourceKeyFiltersChatUploads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write ignore file: %v", err)
	}
	if err := writeUploadMetadata(filepath.Join(dir, "keep.md"), uploadFileRecord{
		SourceKind:       uploadSourceKind,
		SourceKey:        "upload://keep.md",
		Source:           "upload://keep.md",
		OriginalFilename: "keep.md",
		StoredFilename:   "keep.md",
		ContentHash:      "hash-1",
		UploadedAt:       "2026-04-24T09:00:00Z",
		Version:          2,
		FileSize:         5,
	}); err != nil {
		t.Fatalf("write keep metadata: %v", err)
	}
	if err := writeUploadMetadata(filepath.Join(dir, "ignore.md"), uploadFileRecord{
		SourceKind:       "manual",
		SourceKey:        "upload://keep.md",
		Source:           "upload://keep.md",
		OriginalFilename: "ignore.md",
		StoredFilename:   "ignore.md",
		ContentHash:      "hash-2",
		UploadedAt:       "2026-04-24T08:00:00Z",
		Version:          1,
		FileSize:         5,
	}); err != nil {
		t.Fatalf("write ignore metadata: %v", err)
	}

	records, err := listUploadRecordsBySourceKey(dir, "upload://keep.md")
	if err != nil {
		t.Fatalf("list upload records: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].StoredFilename != "keep.md" {
		t.Fatalf("unexpected stored filename: %q", records[0].StoredFilename)
	}
}

func TestFindDuplicateUploadRecordReturnsExistingFile(t *testing.T) {
	dir := t.TempDir()
	oldFileDir := commonFileDirForTest(t, dir)
	defer restoreCommonFileDir(oldFileDir)

	filePath := filepath.Join(dir, "keep.md")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	record := uploadFileRecord{
		StoredFilename: "keep.md",
		ContentHash:    "same-hash",
		filePath:       filePath,
	}
	duplicate, ok := findDuplicateUploadRecord([]uploadFileRecord{record}, "same-hash")
	if !ok {
		t.Fatal("expected duplicate record")
	}
	if duplicate.StoredFilename != "keep.md" {
		t.Fatalf("unexpected duplicate filename: %q", duplicate.StoredFilename)
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

func TestCleanupUploadRecordRemovesMetadataWithStoredFilenameOnly(t *testing.T) {
	dir := t.TempDir()
	oldFileDir := commonFileDirForTest(t, dir)
	defer restoreCommonFileDir(oldFileDir)

	filePath := filepath.Join(dir, "failed.md")
	metadataPath := uploadMetadataPath(filePath)
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.WriteFile(metadataPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	if err := cleanupUploadRecord(uploadFileRecord{StoredFilename: "failed.md"}); err != nil {
		t.Fatalf("cleanup upload record: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, err=%v", err)
	}
	if _, err := os.Stat(metadataPath); !os.IsNotExist(err) {
		t.Fatalf("expected metadata to be removed, err=%v", err)
	}
}

func commonFileDirForTest(t *testing.T, dir string) string {
	t.Helper()
	old := common.FileDir
	common.FileDir = dir
	return old
}

func restoreCommonFileDir(old string) {
	common.FileDir = old
}
