package mem

import (
	"context"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestInMemorySessionStore_RoundTrip(t *testing.T) {
	t.Parallel()
	store := NewInMemorySessionStore()
	ctx := context.Background()

	msgs, summary, err := store.LoadMessages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 0 || summary != "" {
		t.Fatal("expected empty for non-existent session")
	}

	input := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("hi", nil),
	}
	if err := store.SaveMessages(ctx, "s1", input, "greeting"); err != nil {
		t.Fatal(err)
	}

	msgs, summary, err = store.LoadMessages(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if summary != "greeting" {
		t.Fatalf("expected summary 'greeting', got '%s'", summary)
	}

	exists, _ := store.Exists(ctx, "s1")
	if !exists {
		t.Fatal("expected session to exist")
	}

	if err := store.Delete(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	exists, _ = store.Exists(ctx, "s1")
	if exists {
		t.Fatal("expected session to be deleted")
	}
}

func TestInMemoryLongTermStore_RoundTrip(t *testing.T) {
	t.Parallel()
	store := NewInMemoryLongTermStore()
	ctx := context.Background()

	entry := &MemoryEntry{
		ID:        "m1",
		SessionID: "s1",
		Type:      MemoryTypeFact,
		Content:   "user prefers dark mode",
		Relevance: 0.9,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	if err := store.StoreEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}

	entries, err := store.LoadEntries(ctx, "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Content != "user prefers dark mode" {
		t.Fatalf("unexpected content: %s", entries[0].Content)
	}

	cnt, _ := store.CountBySession(ctx, "s1")
	if cnt != 1 {
		t.Fatalf("expected count 1, got %d", cnt)
	}

	total, _ := store.CountAll(ctx)
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}

	entry.Content = "user prefers light mode"
	if err := store.UpdateEntry(ctx, entry); err != nil {
		t.Fatal(err)
	}
	entries, _ = store.LoadEntries(ctx, "s1")
	if entries[0].Content != "user prefers light mode" {
		t.Fatalf("expected updated content")
	}

	if err := store.DeleteEntry(ctx, "m1"); err != nil {
		t.Fatal(err)
	}
	cnt, _ = store.CountBySession(ctx, "s1")
	if cnt != 0 {
		t.Fatalf("expected count 0 after delete, got %d", cnt)
	}
}

func TestInMemoryLongTermStore_NoDuplicateID(t *testing.T) {
	t.Parallel()
	store := NewInMemoryLongTermStore()
	ctx := context.Background()

	entry := &MemoryEntry{
		ID:        "dup",
		SessionID: "s1",
		Type:      MemoryTypeFact,
		Content:   "v1",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	_ = store.StoreEntry(ctx, entry)
	_ = store.StoreEntry(ctx, entry)

	cnt, _ := store.CountBySession(ctx, "s1")
	if cnt != 1 {
		t.Fatalf("expected 1 (no dup), got %d", cnt)
	}
}

func TestSerializeDeserializeMessages(t *testing.T) {
	t.Parallel()
	msgs := []*schema.Message{
		schema.UserMessage("hello"),
		schema.AssistantMessage("world", nil),
	}
	serialized := serializeMessages(msgs)
	if len(serialized) != 2 {
		t.Fatal("expected 2 serialized")
	}
	deserialized := deserializeMessages(serialized)
	if len(deserialized) != 2 {
		t.Fatal("expected 2 deserialized")
	}
	if string(deserialized[0].Role) != "user" || deserialized[0].Content != "hello" {
		t.Fatal("unexpected deserialized user msg")
	}
	if string(deserialized[1].Role) != "assistant" || deserialized[1].Content != "world" {
		t.Fatal("unexpected deserialized assistant msg")
	}
}

func TestSerializeDeserializeEntry(t *testing.T) {
	t.Parallel()
	now := time.Now()
	entry := &MemoryEntry{
		ID:        "e1",
		SessionID: "s1",
		Type:      MemoryTypeProcedure,
		Content:   "step 1: do this",
		Source:    "user",
		Relevance: 0.8,
		AccessCnt: 3,
		CreatedAt: now,
		UpdatedAt: now,
		LastUsed:  now,
		Decay:     0.05,
	}
	s := serializeEntry(entry)
	back := deserializeEntry(s)
	if back.ID != entry.ID || back.Type != entry.Type || back.Content != entry.Content {
		t.Fatal("roundtrip failed")
	}
	if back.Relevance != entry.Relevance || back.AccessCnt != entry.AccessCnt {
		t.Fatal("numeric fields mismatch")
	}
}