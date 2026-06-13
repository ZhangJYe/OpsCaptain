package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/rag"
	"github.com/gogf/gf/v2/frame/g"
)

type MemoryType string

const (
	MemoryTypeFact       MemoryType = "fact"
	MemoryTypePreference MemoryType = "preference"
	MemoryTypeProcedure  MemoryType = "procedure"
	MemoryTypeEpisode    MemoryType = "episode"
)

type MemoryScope string

const (
	MemoryScopeSession MemoryScope = "session"
	MemoryScopeUser    MemoryScope = "user"
	MemoryScopeProject MemoryScope = "project"
	MemoryScopeGlobal  MemoryScope = "global"
)

type MemoryEntry struct {
	ID            string      `json:"id"`
	SessionID     string      `json:"session_id"`
	Type          MemoryType  `json:"type"`
	Content       string      `json:"content"`
	Source        string      `json:"source"`
	Scope         MemoryScope `json:"scope"`
	ScopeID       string      `json:"scope_id,omitempty"`
	Confidence    float64     `json:"confidence"`
	SafetyLabel   string      `json:"safety_label"`
	Provenance    string      `json:"provenance,omitempty"`
	ConflictGroup string      `json:"conflict_group,omitempty"`
	ExpiresAt     int64       `json:"expires_at,omitempty"`
	Relevance     float64     `json:"relevance"`
	AccessCnt     int         `json:"access_count"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	LastUsed      time.Time   `json:"last_used"`
}

type LongTermMemory struct {
	entries map[string]*MemoryEntry
	index   map[string][]string
	store   LongTermMemoryStore
	mu      sync.RWMutex
}

type MemoryStoreOptions struct {
	Scope         MemoryScope
	ScopeID       string
	Confidence    float64
	SafetyLabel   string
	Provenance    string
	ConflictGroup string
	ExpiresAt     int64
}

type MemoryScopeRef struct {
	Scope   MemoryScope
	ScopeID string
}

type MemoryRetrievePolicy struct {
	IncludeExpired bool
	ReadOnly       bool
	ScopeRefs      []MemoryScopeRef
}

type LongTermMemoryStore interface {
	LoadAll(ctx context.Context) ([]*MemoryEntry, error)
	SaveEntries(ctx context.Context, entries []*MemoryEntry) error
	DeleteEntries(ctx context.Context, ids []string) error
}

type pendingChanges struct {
	upserts []*MemoryEntry
	deletes []string
}

const (
	defaultLongTermMaxEntries           = 1000
	defaultLongTermMaxEntriesPerSession = 100
)

var (
	globalLTM     *LongTermMemory
	globalLTMOnce sync.Once
)

// GetLongTermMemory returns a process-wide singleton instance.
//
// WARNING: Without a persistent store (default), this is best-effort in-memory only.
// Memory is lost on restart and not shared across instances.
// For production, enable a Redis-backed store via config.
func GetLongTermMemory() *LongTermMemory {
	globalLTMOnce.Do(func() {
		globalLTM = NewLongTermMemoryWithStore(context.Background(), loadLongTermMemoryStore())
	})
	return globalLTM
}

func NewLongTermMemoryWithStore(ctx context.Context, store LongTermMemoryStore) *LongTermMemory {
	ltm := &LongTermMemory{
		entries: make(map[string]*MemoryEntry),
		index:   make(map[string][]string),
		store:   store,
	}
	if store == nil {
		return ltm
	}
	entries, err := store.LoadAll(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "[ltm] load memory store failed: %v", err)
		return ltm
	}
	ltm.mu.Lock()
	defer ltm.mu.Unlock()
	for _, entry := range entries {
		ltm.addLoadedEntryLocked(entry)
	}
	return ltm
}

func (ltm *LongTermMemory) Store(ctx context.Context, sessionID string, memType MemoryType, content string, source string) string {
	return ltm.StoreWithOptions(ctx, sessionID, memType, content, source, MemoryStoreOptions{})
}

func (ltm *LongTermMemory) StoreWithOptions(ctx context.Context, sessionID string, memType MemoryType, content string, source string, opts MemoryStoreOptions) string {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()

	opts = normalizeMemoryStoreOptions(sessionID, memType, source, opts)
	id := generateMemoryID(opts.Scope, opts.ScopeID, content)
	now := time.Now()
	var changes pendingChanges

	if existing, ok := ltm.entries[id]; ok {
		existing.AccessCnt++
		existing.UpdatedAt = now
		existing.LastUsed = now
		existing.Scope = opts.Scope
		existing.ScopeID = opts.ScopeID
		if opts.Confidence > existing.Confidence {
			existing.Confidence = opts.Confidence
		}
		existing.SafetyLabel = opts.SafetyLabel
		existing.Provenance = opts.Provenance
		existing.ConflictGroup = opts.ConflictGroup
		if opts.ExpiresAt > 0 {
			existing.ExpiresAt = opts.ExpiresAt
		}
		existing.Relevance = computeRelevance(existing)
		retired := ltm.retireConflictingMemoriesLocked(id, memType, opts, now)
		cloned := cloneMemoryEntry(existing, existing.Relevance)
		changes.upserts = append(changes.upserts, &cloned)
		changes.upserts = append(changes.upserts, retired...)
		ltm.persistChangesLocked(ctx, changes)
		g.Log().Debugf(ctx, "[ltm] Reinforced memory %s, access count: %d", id, existing.AccessCnt)
		return id
	}

	entry := &MemoryEntry{
		ID:            id,
		SessionID:     sessionID,
		Type:          memType,
		Content:       content,
		Source:        source,
		Scope:         opts.Scope,
		ScopeID:       opts.ScopeID,
		Confidence:    opts.Confidence,
		SafetyLabel:   opts.SafetyLabel,
		Provenance:    opts.Provenance,
		ConflictGroup: opts.ConflictGroup,
		ExpiresAt:     opts.ExpiresAt,
		Relevance:     1.0,
		AccessCnt:     1,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastUsed:      now,
	}

	evicted := ltm.evictIfNeededLocked(ctx, sessionID)
	retired := ltm.retireConflictingMemoriesLocked(id, memType, opts, now)
	ltm.entries[id] = entry
	ltm.index[sessionID] = append(ltm.index[sessionID], id)

	cloned := cloneMemoryEntry(entry, entry.Relevance)
	changes.upserts = append(changes.upserts, &cloned)
	changes.upserts = append(changes.upserts, retired...)
	changes.deletes = append(changes.deletes, evicted...)
	ltm.persistChangesLocked(ctx, changes)

	g.Log().Debugf(ctx, "[ltm] Stored new %s memory %s for session %s", memType, id, sessionID)
	return id
}

func (ltm *LongTermMemory) Retrieve(ctx context.Context, sessionID string, query string, limit int) []*MemoryEntry {
	return ltm.RetrieveWithPolicy(ctx, sessionID, query, limit, MemoryRetrievePolicy{})
}

func (ltm *LongTermMemory) RetrieveWithPolicy(ctx context.Context, sessionID string, query string, limit int, policy MemoryRetrievePolicy) []*MemoryEntry {
	if len(policy.ScopeRefs) == 0 {
		policy.ScopeRefs = []MemoryScopeRef{{Scope: MemoryScopeSession, ScopeID: sessionID}}
	}
	return ltm.RetrieveScoped(ctx, query, limit, policy)
}

func (ltm *LongTermMemory) RetrieveScoped(ctx context.Context, query string, limit int, policy MemoryRetrievePolicy) []*MemoryEntry {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()

	if len(ltm.entries) == 0 {
		return nil
	}

	queryTokens := rag.BM25Tokenize(query)
	now := time.Now()
	type scored struct {
		id    string
		entry MemoryEntry
		score float64
	}
	var candidates []scored
	for id, entry := range ltm.entries {
		if len(policy.ScopeRefs) > 0 && !memoryInScope(entry, policy.ScopeRefs) {
			continue
		}
		if !policy.IncludeExpired && memoryExpired(entry, now) {
			continue
		}
		relevance := computeRelevance(entry)
		if relevance < 0.1 {
			continue
		}
		score := relevance
		if len(queryTokens) > 0 {
			contentTokens := rag.BM25Tokenize(entry.Content)
			contentSet := make(map[string]struct{}, len(contentTokens))
			for _, t := range contentTokens {
				contentSet[t] = struct{}{}
			}
			matchCount := 0
			for _, qt := range queryTokens {
				if _, ok := contentSet[qt]; ok {
					matchCount++
				}
			}
			score += float64(matchCount) / float64(len(queryTokens)) * 2.0
		}
		candidates = append(candidates, scored{
			id:    id,
			entry: cloneMemoryEntry(entry, relevance),
			score: score,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	result := make([]*MemoryEntry, 0, limit)
	selectedTokenSets := make([]map[string]struct{}, 0, limit)
	for i := 0; i < len(candidates) && len(result) < limit; i++ {
		cand := candidates[i]

		// 去重：与已选记忆的 token 重叠率 > 0.8 则跳过
		candTokens := rag.BM25Tokenize(cand.entry.Content)
		candSet := make(map[string]struct{}, len(candTokens))
		for _, t := range candTokens {
			candSet[t] = struct{}{}
		}
		duplicate := false
		for _, existing := range selectedTokenSets {
			if tokenOverlap(candSet, existing) > 0.8 {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		selectedTokenSets = append(selectedTokenSets, candSet)

		e := cand.entry
		e.AccessCnt++
		e.LastUsed = now
		result = append(result, &e)

		if !policy.ReadOnly {
			if orig, ok := ltm.entries[cand.id]; ok {
				orig.AccessCnt++
				orig.LastUsed = now
				orig.Relevance = computeRelevance(orig)
			}
		}
	}

	if !policy.ReadOnly && len(result) > 0 {
		var upserts []*MemoryEntry
		for _, e := range result {
			if orig, ok := ltm.entries[e.ID]; ok {
				cloned := cloneMemoryEntry(orig, orig.Relevance)
				upserts = append(upserts, &cloned)
			}
		}
		if len(upserts) > 0 {
			ltm.persistChangesLocked(ctx, pendingChanges{upserts: upserts})
		}
	}
	return result
}

func (ltm *LongTermMemory) Forget(ctx context.Context, threshold float64) int {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()

	var toRemove []string
	for id, entry := range ltm.entries {
		entry.Relevance = computeRelevance(entry)
		if entry.Relevance < threshold {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		entry := ltm.entries[id]
		sessionID := entry.SessionID
		delete(ltm.entries, id)
		ids := ltm.index[sessionID]
		for i, sid := range ids {
			if sid == id {
				ltm.index[sessionID] = append(ids[:i], ids[i+1:]...)
				break
			}
		}
		if len(ltm.index[sessionID]) == 0 {
			delete(ltm.index, sessionID)
		}
	}

	if len(toRemove) > 0 {
		ltm.persistChangesLocked(ctx, pendingChanges{deletes: toRemove})
		g.Log().Infof(ctx, "[ltm] Forgot %d memories below threshold %.2f", len(toRemove), threshold)
	}
	return len(toRemove)
}

func (ltm *LongTermMemory) Count() int {
	ltm.mu.RLock()
	defer ltm.mu.RUnlock()
	return len(ltm.entries)
}

func (ltm *LongTermMemory) CountBySession(sessionID string) int {
	ltm.mu.RLock()
	defer ltm.mu.RUnlock()
	return len(ltm.index[sessionID])
}

func (ltm *LongTermMemory) GetAllBySession(sessionID string) []*MemoryEntry {
	ltm.mu.RLock()
	defer ltm.mu.RUnlock()
	ids := ltm.index[sessionID]
	result := make([]*MemoryEntry, 0, len(ids))
	for _, id := range ids {
		if e, ok := ltm.entries[id]; ok {
			cloned := cloneMemoryEntry(e, computeRelevance(e))
			result = append(result, &cloned)
		}
	}
	return result
}

func (ltm *LongTermMemory) Get(id string) *MemoryEntry {
	ltm.mu.RLock()
	defer ltm.mu.RUnlock()
	entry, ok := ltm.entries[strings.TrimSpace(id)]
	if !ok {
		return nil
	}
	cloned := cloneMemoryEntry(entry, computeRelevance(entry))
	return &cloned
}

func (ltm *LongTermMemory) List(scopeRefs []MemoryScopeRef, includeExpired bool) []*MemoryEntry {
	ltm.mu.RLock()
	defer ltm.mu.RUnlock()
	now := time.Now()
	result := make([]*MemoryEntry, 0)
	for _, entry := range ltm.entries {
		if len(scopeRefs) > 0 && !memoryInScope(entry, scopeRefs) {
			continue
		}
		if !includeExpired && memoryExpired(entry, now) {
			continue
		}
		cloned := cloneMemoryEntry(entry, computeRelevance(entry))
		result = append(result, &cloned)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].UpdatedAt.After(result[j].UpdatedAt)
	})
	return result
}

func (ltm *LongTermMemory) Delete(ctx context.Context, id string) bool {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()
	if _, ok := ltm.entries[id]; !ok {
		return false
	}
	ltm.removeEntryLocked(id)
	ltm.persistChangesLocked(ctx, pendingChanges{deletes: []string{id}})
	return true
}

func (ltm *LongTermMemory) Disable(ctx context.Context, id string) bool {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()
	entry, ok := ltm.entries[id]
	if !ok {
		return false
	}
	now := time.Now()
	entry.SafetyLabel = "disabled"
	entry.ExpiresAt = now.UnixMilli()
	entry.UpdatedAt = now
	cloned := cloneMemoryEntry(entry, entry.Relevance)
	ltm.persistChangesLocked(ctx, pendingChanges{upserts: []*MemoryEntry{&cloned}})
	return true
}

func (ltm *LongTermMemory) Promote(ctx context.Context, id string, scope MemoryScope, scopeID string, confidence float64) bool {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()
	entry, ok := ltm.entries[id]
	if !ok {
		return false
	}
	now := time.Now()
	if scope != "" {
		entry.Scope = scope
	}
	if strings.TrimSpace(scopeID) != "" {
		entry.ScopeID = strings.TrimSpace(scopeID)
	}
	if confidence > 0 {
		if confidence > 1 {
			confidence = 1
		}
		entry.Confidence = confidence
	}
	if entry.ScopeID == "" {
		entry.ScopeID = entry.SessionID
	}
	entry.SafetyLabel = "internal"
	entry.UpdatedAt = now
	entry.LastUsed = now
	cloned := cloneMemoryEntry(entry, entry.Relevance)
	ltm.persistChangesLocked(ctx, pendingChanges{upserts: []*MemoryEntry{&cloned}})
	return true
}

func loadLongTermMaxEntries() int {
	v, err := g.Cfg().Get(context.Background(), "memory.long_term_max_entries")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return defaultLongTermMaxEntries
}

func loadLongTermMaxEntriesPerSession() int {
	v, err := g.Cfg().Get(context.Background(), "memory.long_term_max_entries_per_session")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return defaultLongTermMaxEntriesPerSession
}
