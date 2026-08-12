package app

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/internal/infra/filestore"
	"context"
	"fmt"
	"strings"
	"testing"
)

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

func TestAllowedExtensionList(t *testing.T) {
	list := allowedExtensionList()
	if len(list) != len(allowedExtensions) {
		t.Errorf("expected %d extensions, got %d", len(allowedExtensions), len(list))
	}
}

func TestUploadSourcePrefixForContextUsesUserNamespace(t *testing.T) {
	alice := context.WithValue(context.Background(), consts.CtxKeyUserID, "alice@example.com")
	bob := context.WithValue(context.Background(), consts.CtxKeyUserID, "bob@example.com")

	alicePrefix := uploadSourcePrefixForContext(alice)
	bobPrefix := uploadSourcePrefixForContext(bob)

	if alicePrefix == uploadSourcePrefix {
		t.Fatal("expected user scoped upload prefix")
	}
	if alicePrefix == bobPrefix {
		t.Fatal("expected different users to get different upload prefixes")
	}
	if strings.Contains(alicePrefix, "alice@example.com") {
		t.Fatalf("source prefix should not expose raw user id: %s", alicePrefix)
	}
	if !strings.HasPrefix(alicePrefix, "upload://users/") || !strings.HasSuffix(alicePrefix, "/") {
		t.Fatalf("unexpected user scoped prefix: %s", alicePrefix)
	}
}

func TestUploadSourcePrefixForContextKeepsLegacyWhenAnonymous(t *testing.T) {
	if got := uploadSourcePrefixForContext(context.Background()); got != uploadSourcePrefix {
		t.Fatalf("expected legacy prefix %s, got %s", uploadSourcePrefix, got)
	}
}

func TestKnowledgeAppHandleUploadPassesUserScopedSourcePrefix(t *testing.T) {
	store := &captureUploadStore{}
	k := NewKnowledgeApp(store)
	ctx := context.WithValue(context.Background(), consts.CtxKeyUserID, "operator-1")

	_, err := k.HandleUpload(ctx, &UploadInput{
		Filename: "runbook.md",
		MIMEType: "text/markdown",
		Size:     int64(len("hello")),
		Content:  []byte("hello"),
	})
	if err != nil {
		t.Fatalf("handle upload: %v", err)
	}
	if !strings.HasPrefix(store.input.SourcePrefix, "upload://users/") {
		t.Fatalf("expected user scoped source prefix, got %s", store.input.SourcePrefix)
	}
}

type captureUploadStore struct {
	input filestore.UploadSaveInput
}

func (s *captureUploadStore) SaveUpload(_ context.Context, input filestore.UploadSaveInput) (*filestore.UploadSaveResult, error) {
	s.input = input
	return &filestore.UploadSaveResult{
		FileName:  input.Filename,
		FileID:    "file-id",
		FileSize:  input.Size,
		SourceKey: input.SourcePrefix + input.Filename,
		Status:    "ready",
		Duplicate: true,
	}, nil
}

func (s *captureUploadStore) UploadStatus(context.Context, string) (string, error) {
	return "ready", nil
}

func (s *captureUploadStore) MarkUploadStatus(context.Context, string, string) error {
	return nil
}

func (s *captureUploadStore) CleanupReplacedUploads(context.Context, string, string, string) error {
	return nil
}

func (s *captureUploadStore) ListUploads(context.Context, string, string) ([]filestore.UploadRecord, error) {
	return nil, nil
}

func (s *captureUploadStore) GetUpload(context.Context, string) (*filestore.UploadRecord, error) {
	return nil, fmt.Errorf("not found")
}

func (s *captureUploadStore) DeleteUpload(context.Context, string) error {
	return nil
}

func TestKnowledgeAppRestrictsDocumentsToCurrentUser(t *testing.T) {
	store := &knowledgeDocumentStore{records: map[string]filestore.UploadRecord{
		"alice-file": {FileID: "alice-file", FileName: "alice.md", SourceKind: uploadSourceKind, SourceKey: uploadSourcePrefixForContext(context.WithValue(context.Background(), consts.CtxKeyUserID, "alice")), IndexStatus: "ready"},
		"bob-file":   {FileID: "bob-file", FileName: "bob.md", SourceKind: uploadSourceKind, SourceKey: uploadSourcePrefixForContext(context.WithValue(context.Background(), consts.CtxKeyUserID, "bob")), IndexStatus: "ready"},
	}}
	k := &KnowledgeApp{uploadStore: store, deleteSource: func(context.Context, string) error { return nil }}
	aliceCtx := context.WithValue(context.Background(), consts.CtxKeyUserID, "alice")
	items, err := k.ListDocuments(aliceCtx)
	if err != nil {
		t.Fatalf("list documents: %v", err)
	}
	if len(items) != 1 || items[0].FileID != "alice-file" {
		t.Fatalf("unexpected documents: %#v", items)
	}
	if err := k.DeleteDocument(aliceCtx, "bob-file"); err == nil || !strings.Contains(err.Error(), "资料不存在") {
		t.Fatalf("expected ownership error, got %v", err)
	}
}

func TestKnowledgeAppKeepsDocumentWhenIndexDeletionFails(t *testing.T) {
	store := &knowledgeDocumentStore{records: map[string]filestore.UploadRecord{
		"file": {FileID: "file", FileName: "runbook.md", SourceKind: uploadSourceKind, SourceKey: uploadSourcePrefix, IndexStatus: "ready"},
	}}
	k := &KnowledgeApp{uploadStore: store, deleteSource: func(context.Context, string) error { return fmt.Errorf("milvus unavailable") }}
	if err := k.DeleteDocument(context.Background(), "file"); err == nil {
		t.Fatal("expected delete failure")
	}
	if store.deleted != "" {
		t.Fatalf("file should remain when index deletion fails, deleted=%s", store.deleted)
	}
}

func TestKnowledgeAppRejectsDeletingIndexingDocument(t *testing.T) {
	store := &knowledgeDocumentStore{records: map[string]filestore.UploadRecord{
		"file": {FileID: "file", FileName: "runbook.md", SourceKind: uploadSourceKind, SourceKey: uploadSourcePrefix, IndexStatus: "indexing"},
	}}
	k := &KnowledgeApp{uploadStore: store, deleteSource: func(context.Context, string) error { return nil }}
	if err := k.DeleteDocument(context.Background(), "file"); err == nil || !strings.Contains(err.Error(), "正在索引") {
		t.Fatalf("expected indexing deletion rejection, got %v", err)
	}
	if store.deleted != "" {
		t.Fatalf("indexing document should remain, deleted=%s", store.deleted)
	}
}

func TestKnowledgeAppRetriesOnlyFailedDocument(t *testing.T) {
	store := &knowledgeDocumentStore{records: map[string]filestore.UploadRecord{
		"failed": {FileID: "failed", FileName: "runbook.md", FilePath: "runbook.md", SourceKind: uploadSourceKind, SourceKey: uploadSourcePrefix, IndexStatus: "failed"},
		"ready":  {FileID: "ready", FileName: "ready.md", FilePath: "ready.md", SourceKind: uploadSourceKind, SourceKey: uploadSourcePrefix, IndexStatus: "ready"},
	}}
	block := make(chan struct{})
	k := &KnowledgeApp{uploadStore: store, indexSource: func(context.Context, string) error {
		<-block
		return fmt.Errorf("stop")
	}}
	defer close(block)
	item, err := k.RetryDocumentIndex(context.Background(), "failed")
	if err != nil {
		t.Fatalf("retry failed document: %v", err)
	}
	if item.Status != "indexing" || store.records["failed"].IndexStatus != "indexing" {
		t.Fatalf("expected indexing state, item=%#v record=%#v", item, store.records["failed"])
	}
	if _, err := k.RetryDocumentIndex(context.Background(), "ready"); err == nil {
		t.Fatal("ready document should not be reindexed")
	}
}

type knowledgeDocumentStore struct {
	records map[string]filestore.UploadRecord
	deleted string
}

func (s *knowledgeDocumentStore) SaveUpload(context.Context, filestore.UploadSaveInput) (*filestore.UploadSaveResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *knowledgeDocumentStore) UploadStatus(context.Context, string) (string, error) {
	return "", nil
}

func (s *knowledgeDocumentStore) MarkUploadStatus(_ context.Context, fileID, status string) error {
	record := s.records[fileID]
	record.IndexStatus = status
	s.records[fileID] = record
	return nil
}

func (s *knowledgeDocumentStore) CleanupReplacedUploads(context.Context, string, string, string) error {
	return nil
}

func (s *knowledgeDocumentStore) ListUploads(_ context.Context, sourceKind, sourcePrefix string) ([]filestore.UploadRecord, error) {
	items := make([]filestore.UploadRecord, 0)
	for _, record := range s.records {
		if record.SourceKind == sourceKind && strings.HasPrefix(record.SourceKey, sourcePrefix) {
			items = append(items, record)
		}
	}
	return items, nil
}

func (s *knowledgeDocumentStore) GetUpload(_ context.Context, fileID string) (*filestore.UploadRecord, error) {
	record, ok := s.records[fileID]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return &record, nil
}

func (s *knowledgeDocumentStore) DeleteUpload(_ context.Context, fileID string) error {
	delete(s.records, fileID)
	s.deleted = fileID
	return nil
}
