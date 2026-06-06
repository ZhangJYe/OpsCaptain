package app

import (
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/internal/infra/filestore"
	"context"
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
