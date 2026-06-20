package collaboration

import (
	"testing"
	"time"
)

func TestCreate(t *testing.T) {
	store := NewShareStore()
	link, err := store.Create("sess-1", "user-1", 24)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if link.ID == "" {
		t.Fatal("expected ID to be set")
	}
	if link.SessionID != "sess-1" {
		t.Fatalf("expected session_id=sess-1, got %s", link.SessionID)
	}
	if link.CreatedBy != "user-1" {
		t.Fatalf("expected created_by=user-1, got %s", link.CreatedBy)
	}
	if link.ExpiresAt <= link.CreatedAt {
		t.Fatal("expected ExpiresAt > CreatedAt")
	}
}

func TestCreateEmptySession(t *testing.T) {
	store := NewShareStore()
	_, err := store.Create("", "user-1", 24)
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

func TestCreateDefaultTTL(t *testing.T) {
	store := NewShareStore()
	link, err := store.Create("sess-1", "", 0)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	expectedExpiry := time.Now().Add(24 * time.Hour).UnixMilli()
	if link.ExpiresAt < expectedExpiry-60000 || link.ExpiresAt > expectedExpiry+60000 {
		t.Fatalf("expected expiry around %d, got %d", expectedExpiry, link.ExpiresAt)
	}
}

func TestGet(t *testing.T) {
	store := NewShareStore()
	link, _ := store.Create("sess-1", "user-1", 24)

	got, ok := store.Get(link.ID)
	if !ok {
		t.Fatal("expected to find link")
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("expected session_id=sess-1, got %s", got.SessionID)
	}
}

func TestGetNotFound(t *testing.T) {
	store := NewShareStore()
	_, ok := store.Get("nonexistent")
	if ok {
		t.Fatal("expected not found")
	}
}

func TestGetExpired(t *testing.T) {
	store := NewShareStore()
	link := &ShareLink{
		ID:        "expired_1",
		SessionID: "sess-1",
		CreatedBy: "user-1",
		CreatedAt: time.Now().Add(-2 * time.Hour).UnixMilli(),
		ExpiresAt: time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	store.mu.Lock()
	store.links[link.ID] = link
	store.mu.Unlock()

	_, ok := store.Get("expired_1")
	if ok {
		t.Fatal("expected expired link to not be found")
	}
}

func TestRevoke(t *testing.T) {
	store := NewShareStore()
	link, _ := store.Create("sess-1", "user-1", 24)

	if err := store.Revoke(link.ID); err != nil {
		t.Fatalf("Revoke failed: %v", err)
	}

	_, ok := store.Get(link.ID)
	if ok {
		t.Fatal("expected revoked link to not be found")
	}
}

func TestRevokeNotFound(t *testing.T) {
	store := NewShareStore()
	err := store.Revoke("nonexistent")
	if err == nil {
		t.Fatal("expected error for revoking nonexistent link")
	}
}
