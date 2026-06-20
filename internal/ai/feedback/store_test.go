package feedback

import (
	"testing"
)

func TestSubmit(t *testing.T) {
	store := NewStore()

	entry := &FeedbackEntry{
		SessionID: "sess-1",
		Query:     "test query",
		Rating:    RatingHelpful,
	}
	if err := store.Submit(entry); err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if entry.ID == "" {
		t.Fatal("expected ID to be set")
	}
	if entry.CreatedAt == 0 {
		t.Fatal("expected CreatedAt to be set")
	}

	stats := store.Stats()
	if stats.Total != 1 {
		t.Fatalf("expected total=1, got %d", stats.Total)
	}
	if stats.Helpful != 1 {
		t.Fatalf("expected helpful=1, got %d", stats.Helpful)
	}
	if stats.Score != 1.0 {
		t.Fatalf("expected score=1.0, got %f", stats.Score)
	}
}

func TestSubmitInvalidRating(t *testing.T) {
	store := NewStore()

	entry := &FeedbackEntry{
		SessionID: "sess-1",
		Query:     "test query",
		Rating:    "invalid",
	}
	if err := store.Submit(entry); err == nil {
		t.Fatal("expected error for invalid rating")
	}
}

func TestSubmitEmptySession(t *testing.T) {
	store := NewStore()

	entry := &FeedbackEntry{
		Query:  "test query",
		Rating: RatingHelpful,
	}
	if err := store.Submit(entry); err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

func TestSubmitEmptyQuery(t *testing.T) {
	store := NewStore()

	entry := &FeedbackEntry{
		SessionID: "sess-1",
		Rating:    RatingHelpful,
	}
	if err := store.Submit(entry); err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestGetBySession(t *testing.T) {
	store := NewStore()

	store.Submit(&FeedbackEntry{SessionID: "sess-1", Query: "q1", Rating: RatingHelpful})
	store.Submit(&FeedbackEntry{SessionID: "sess-1", Query: "q2", Rating: RatingNotHelpful})
	store.Submit(&FeedbackEntry{SessionID: "sess-2", Query: "q3", Rating: RatingHelpful})

	entries := store.GetBySession("sess-1")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for sess-1, got %d", len(entries))
	}

	entries = store.GetBySession("sess-2")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for sess-2, got %d", len(entries))
	}

	entries = store.GetBySession("sess-3")
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for sess-3, got %d", len(entries))
	}
}

func TestStats(t *testing.T) {
	store := NewStore()

	store.Submit(&FeedbackEntry{SessionID: "s1", Query: "q1", Rating: RatingHelpful})
	store.Submit(&FeedbackEntry{SessionID: "s1", Query: "q2", Rating: RatingNotHelpful})
	store.Submit(&FeedbackEntry{SessionID: "s2", Query: "q3", Rating: RatingHelpful})

	stats := store.Stats()
	if stats.Total != 3 {
		t.Fatalf("expected total=3, got %d", stats.Total)
	}
	if stats.Helpful != 2 {
		t.Fatalf("expected helpful=2, got %d", stats.Helpful)
	}
	if stats.NotHelpful != 1 {
		t.Fatalf("expected not_helpful=1, got %d", stats.NotHelpful)
	}
	expectedScore := float64(2) / float64(3)
	if stats.Score != expectedScore {
		t.Fatalf("expected score=%f, got %f", expectedScore, stats.Score)
	}
}

func TestStatsEmpty(t *testing.T) {
	store := NewStore()
	stats := store.Stats()
	if stats.Total != 0 || stats.Score != 0 {
		t.Fatalf("expected empty stats, got %+v", stats)
	}
}
