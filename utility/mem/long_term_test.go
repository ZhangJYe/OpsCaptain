package mem

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func resetLTM() {
	globalLTMOnce = sync.Once{}
	globalLTM = nil
}

func TestLongTermMemory_StoreAndRetrieve(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess1", MemoryTypeFact, "服务名是order-service", "conversation")
	ltm.Store(ctx, "sess1", MemoryTypePreference, "用户偏好中文回复", "user_input")

	results := ltm.Retrieve(ctx, "sess1", "服务名", 10)
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}

	foundFact := false
	for _, r := range results {
		if r.Type == MemoryTypeFact {
			foundFact = true
		}
	}
	if !foundFact {
		t.Fatal("expected to find fact memory")
	}
}

func TestLongTermMemory_Reinforcement(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	id1 := ltm.Store(ctx, "sess1", MemoryTypeFact, "test fact", "test")
	id2 := ltm.Store(ctx, "sess1", MemoryTypeFact, "test fact", "test")

	if id1 != id2 {
		t.Fatal("same content should produce same ID")
	}
	if ltm.Count() != 1 {
		t.Fatalf("expected 1 memory entry, got %d", ltm.Count())
	}

	results := ltm.Retrieve(ctx, "sess1", "test", 1)
	if len(results) != 1 {
		t.Fatal("expected 1 result")
	}
	if results[0].AccessCnt < 2 {
		t.Fatalf("expected access count >= 2, got %d", results[0].AccessCnt)
	}
}

func TestLongTermMemory_SessionIsolation(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess-a", MemoryTypeFact, "data for session A", "test")
	ltm.Store(ctx, "sess-b", MemoryTypeFact, "data for session B", "test")

	resultsA := ltm.Retrieve(ctx, "sess-a", "data", 10)
	resultsB := ltm.Retrieve(ctx, "sess-b", "data", 10)

	if len(resultsA) != 1 || len(resultsB) != 1 {
		t.Fatalf("expected 1 result per session, got A=%d B=%d", len(resultsA), len(resultsB))
	}
	if resultsA[0].SessionID != "sess-a" {
		t.Fatal("session isolation failed for A")
	}
	if resultsB[0].SessionID != "sess-b" {
		t.Fatal("session isolation failed for B")
	}
}

func TestLongTermMemory_Forget(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess1", MemoryTypeFact, "fact1", "test")
	ltm.Store(ctx, "sess1", MemoryTypeFact, "fact2", "test")

	if ltm.Count() != 2 {
		t.Fatalf("expected 2 entries, got %d", ltm.Count())
	}

	removed := ltm.Forget(ctx, 999.0)
	if removed != 2 {
		t.Fatalf("expected to remove 2 entries with high threshold, removed %d", removed)
	}
	if ltm.Count() != 0 {
		t.Fatalf("expected 0 entries after forget, got %d", ltm.Count())
	}
}

func TestLongTermMemory_ForgetLowThreshold(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess1", MemoryTypeFact, "recent fact", "test")

	removed := ltm.Forget(ctx, 0.01)
	if removed != 0 {
		t.Fatalf("recent memory should not be forgotten with low threshold, removed %d", removed)
	}
}

func TestLongTermMemory_CountBySession(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess1", MemoryTypeFact, "f1", "test")
	ltm.Store(ctx, "sess1", MemoryTypeFact, "f2", "test")
	ltm.Store(ctx, "sess2", MemoryTypeFact, "f3", "test")

	if ltm.CountBySession("sess1") != 2 {
		t.Fatalf("expected 2 for sess1, got %d", ltm.CountBySession("sess1"))
	}
	if ltm.CountBySession("sess2") != 1 {
		t.Fatalf("expected 1 for sess2, got %d", ltm.CountBySession("sess2"))
	}
	if ltm.CountBySession("nonexistent") != 0 {
		t.Fatal("expected 0 for nonexistent session")
	}
}

func TestLongTermMemory_GetAllBySession(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess1", MemoryTypeFact, "fact1", "test")
	ltm.Store(ctx, "sess1", MemoryTypePreference, "pref1", "test")

	all := ltm.GetAllBySession("sess1")
	if len(all) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(all))
	}

	types := map[MemoryType]bool{}
	for _, m := range all {
		types[m.Type] = true
	}
	if !types[MemoryTypeFact] || !types[MemoryTypePreference] {
		t.Fatal("expected both fact and preference types")
	}
}

func TestLongTermMemory_ConcurrentAccess(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sess := fmt.Sprintf("concurrent-%d", idx%5)
			ltm.Store(ctx, sess, MemoryTypeFact, fmt.Sprintf("fact %d", idx), "test")
			ltm.Retrieve(ctx, sess, "fact", 3)
		}(i)
	}
	wg.Wait()

	total := ltm.Count()
	if total == 0 {
		t.Fatal("expected non-zero memories after concurrent access")
	}
}

func TestLongTermMemory_RetrieveRelevanceScoring(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess1", MemoryTypeFact, "Prometheus告警阈值是85%", "test")
	ltm.Store(ctx, "sess1", MemoryTypeFact, "数据库连接池大小是100", "test")
	ltm.Store(ctx, "sess1", MemoryTypeFact, "Prometheus监控的服务端口是8080", "test")

	results := ltm.Retrieve(ctx, "sess1", "Prometheus告警", 2)
	if len(results) == 0 {
		t.Fatal("expected results for query")
	}
}

func TestLongTermMemory_EmptyRetrieve(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	results := ltm.Retrieve(ctx, "nonexistent", "anything", 10)
	if results != nil && len(results) != 0 {
		t.Fatal("expected nil or empty results")
	}
}

func TestComputeRelevance(t *testing.T) {
	entry := &MemoryEntry{
		AccessCnt: 1,
		LastUsed:  time.Now(),
	}
	r1 := computeRelevance(entry)
	if r1 <= 0 {
		t.Fatalf("expected positive relevance, got %f", r1)
	}

	entry.AccessCnt = 5
	r2 := computeRelevance(entry)
	if r2 <= r1 {
		t.Fatalf("higher access count should yield higher relevance: %f <= %f", r2, r1)
	}

	entry.LastUsed = time.Now().Add(-48 * time.Hour)
	r3 := computeRelevance(entry)
	if r3 >= r2 {
		t.Fatalf("older memory should have lower relevance: %f >= %f", r3, r2)
	}
}

func TestLongTermMemory_RespectsPerSessionCapacity(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	for i := 0; i < defaultLongTermMaxEntriesPerSession+5; i++ {
		ltm.Store(ctx, "sess-cap", MemoryTypeFact, fmt.Sprintf("fact-%d", i), "test")
	}

	if got := ltm.CountBySession("sess-cap"); got > defaultLongTermMaxEntriesPerSession {
		t.Fatalf("expected session count <= %d, got %d", defaultLongTermMaxEntriesPerSession, got)
	}
}
