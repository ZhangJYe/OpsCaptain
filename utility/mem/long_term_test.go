package mem

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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

func TestLongTermMemory_StoreWithOptionsMetadata(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()
	expiresAt := time.Now().Add(time.Hour).UnixMilli()

	ltm.StoreWithOptions(ctx, "sess-meta", MemoryTypeFact, "服务名是payment-service", "review", MemoryStoreOptions{
		Scope:       MemoryScopeProject,
		Confidence:  0.92,
		SafetyLabel: "internal",
		Provenance:  "human_review",
		ExpiresAt:   expiresAt,
	})

	results := ltm.RetrieveScoped(ctx, "payment-service", 1, MemoryRetrievePolicy{
		ScopeRefs: []MemoryScopeRef{{Scope: MemoryScopeProject, ScopeID: "sess-meta"}},
	})
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	got := results[0]
	if got.Scope != MemoryScopeProject {
		t.Fatalf("expected project scope, got %q", got.Scope)
	}
	if got.Confidence != 0.92 {
		t.Fatalf("expected confidence 0.92, got %f", got.Confidence)
	}
	if got.Provenance != "human_review" {
		t.Fatalf("expected provenance, got %q", got.Provenance)
	}
	if got.ExpiresAt != expiresAt {
		t.Fatalf("expected expires_at %d, got %d", expiresAt, got.ExpiresAt)
	}
}

func TestLongTermMemory_RetrieveScopedIncludesMultipleScopes(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.Store(ctx, "sess-scope", MemoryTypeFact, "session memory payment-service", "test")
	ltm.StoreWithOptions(ctx, "user-1", MemoryTypeFact, "user memory payment-service", "test", MemoryStoreOptions{
		Scope:   MemoryScopeUser,
		ScopeID: "user-1",
	})
	ltm.StoreWithOptions(ctx, "project-1", MemoryTypeFact, "project memory payment-service", "test", MemoryStoreOptions{
		Scope:   MemoryScopeProject,
		ScopeID: "project-1",
	})
	ltm.StoreWithOptions(ctx, "global", MemoryTypeFact, "global memory payment-service", "test", MemoryStoreOptions{
		Scope: MemoryScopeGlobal,
	})

	results := ltm.RetrieveScoped(ctx, "payment-service", 10, MemoryRetrievePolicy{
		ScopeRefs: []MemoryScopeRef{
			{Scope: MemoryScopeSession, ScopeID: "sess-scope"},
			{Scope: MemoryScopeUser, ScopeID: "user-1"},
			{Scope: MemoryScopeProject, ScopeID: "project-1"},
			{Scope: MemoryScopeGlobal, ScopeID: "global"},
		},
	})
	if len(results) != 4 {
		t.Fatalf("expected 4 scoped memories, got %d", len(results))
	}
}

func TestLongTermMemory_FileStorePersistsEntries(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memory.json")
	store := NewFileLongTermMemoryStore(path)
	ltm := NewLongTermMemoryWithStore(ctx, store)

	ltm.Store(ctx, "sess-file", MemoryTypeFact, "file-backed payment-service memory", "test")

	reloaded := NewLongTermMemoryWithStore(ctx, store)
	results := reloaded.Retrieve(ctx, "sess-file", "payment-service", 10)
	if len(results) != 1 {
		t.Fatalf("expected persisted memory after reload, got %d", len(results))
	}
}

func TestLongTermMemory_DeleteDisablePromote(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	id := ltm.Store(ctx, "sess-manage", MemoryTypeFact, "manageable payment-service memory", "test")
	if !ltm.Promote(ctx, id, MemoryScopeProject, "project-manage", 0.95) {
		t.Fatal("expected promote to succeed")
	}
	listed := ltm.List([]MemoryScopeRef{{Scope: MemoryScopeProject, ScopeID: "project-manage"}}, false)
	if len(listed) != 1 || listed[0].Confidence != 0.95 {
		t.Fatalf("expected promoted memory, got %+v", listed)
	}
	if !ltm.Disable(ctx, id) {
		t.Fatal("expected disable to succeed")
	}
	if got := ltm.RetrieveScoped(ctx, "payment-service", 10, MemoryRetrievePolicy{
		ScopeRefs: []MemoryScopeRef{{Scope: MemoryScopeProject, ScopeID: "project-manage"}},
	}); len(got) != 0 {
		t.Fatalf("expected disabled memory to be filtered, got %d", len(got))
	}
	if !ltm.Delete(ctx, id) {
		t.Fatal("expected delete to succeed")
	}
	if ltm.Count() != 0 {
		t.Fatalf("expected memory to be deleted, count=%d", ltm.Count())
	}
}

func TestLongTermMemory_ConflictGroupSupersedesOldMemory(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	oldID := ltm.StoreWithOptions(ctx, "sess-conflict", MemoryTypeFact, "服务端口是8080", "test", MemoryStoreOptions{
		ConflictGroup: "service-port",
		Confidence:    0.90,
	})
	ltm.StoreWithOptions(ctx, "sess-conflict", MemoryTypeFact, "服务端口是9090", "test", MemoryStoreOptions{
		ConflictGroup: "service-port",
		Confidence:    0.90,
	})

	results := ltm.Retrieve(ctx, "sess-conflict", "服务端口", 10)
	if len(results) != 1 || !strings.Contains(results[0].Content, "9090") {
		t.Fatalf("expected only new conflict memory, got %+v", results)
	}
	all := ltm.List([]MemoryScopeRef{{Scope: MemoryScopeSession, ScopeID: "sess-conflict"}}, true)
	foundSuperseded := false
	for _, entry := range all {
		if entry.ID == oldID && entry.SafetyLabel == "superseded" {
			foundSuperseded = true
		}
	}
	if !foundSuperseded {
		t.Fatalf("expected old memory to be superseded, got %+v", all)
	}
}

func TestLongTermMemory_RetrieveSkipsExpiredByDefault(t *testing.T) {
	resetLTM()
	ltm := GetLongTermMemory()
	ctx := context.Background()

	ltm.StoreWithOptions(ctx, "sess-expired", MemoryTypeFact, "过期事实", "test", MemoryStoreOptions{
		Confidence: 0.90,
		ExpiresAt:  time.Now().Add(-time.Minute).UnixMilli(),
	})

	if got := ltm.Retrieve(ctx, "sess-expired", "过期", 10); len(got) != 0 {
		t.Fatalf("expected expired memory to be skipped, got %d", len(got))
	}
	withExpired := ltm.RetrieveWithPolicy(ctx, "sess-expired", "过期", 10, MemoryRetrievePolicy{IncludeExpired: true})
	if len(withExpired) != 1 {
		t.Fatalf("expected expired memory with explicit policy, got %d", len(withExpired))
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
