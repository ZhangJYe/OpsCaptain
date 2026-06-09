package memory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func generateMemoryID(scope MemoryScope, scopeID, content string) string {
	h := sha256.Sum256([]byte(string(scope) + ":" + scopeID + ":" + content))
	return fmt.Sprintf("%x", h[:8])
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

func computeRelevance(entry *MemoryEntry) float64 {
	hoursSinceUse := time.Since(entry.LastUsed).Hours()
	decay := 1.0 / (1.0 + hoursSinceUse/24.0)
	frequency := 1.0 + float64(entry.AccessCnt-1)*0.3
	if frequency > 3.0 {
		frequency = 3.0
	}
	return decay * frequency
}

func normalizeMemoryStoreOptions(sessionID string, memType MemoryType, source string, opts MemoryStoreOptions) MemoryStoreOptions {
	if opts.Scope == "" {
		opts.Scope = MemoryScopeSession
	}
	if strings.TrimSpace(opts.ScopeID) == "" {
		if opts.Scope == MemoryScopeGlobal {
			opts.ScopeID = "global"
		} else {
			opts.ScopeID = sessionID
		}
	}
	if opts.Confidence <= 0 {
		opts.Confidence = defaultMemoryConfidence(memType)
	}
	if opts.Confidence > 1 {
		opts.Confidence = 1
	}
	if opts.SafetyLabel == "" {
		opts.SafetyLabel = "internal"
	}
	if opts.Provenance == "" {
		opts.Provenance = source
	}
	opts.ConflictGroup = strings.TrimSpace(opts.ConflictGroup)
	return opts
}

func defaultMemoryConfidence(memType MemoryType) float64 {
	switch memType {
	case MemoryTypePreference:
		return 0.85
	case MemoryTypeProcedure:
		return 0.75
	case MemoryTypeFact:
		return 0.70
	case MemoryTypeEpisode:
		return 0.50
	default:
		return 0.50
	}
}

func cloneMemoryEntry(entry *MemoryEntry, relevance float64) MemoryEntry {
	scope := entry.Scope
	if scope == "" {
		scope = MemoryScopeSession
	}
	confidence := entry.Confidence
	if confidence <= 0 {
		confidence = defaultMemoryConfidence(entry.Type)
	}
	safetyLabel := entry.SafetyLabel
	if safetyLabel == "" {
		safetyLabel = "internal"
	}
	provenance := entry.Provenance
	if provenance == "" {
		provenance = entry.Source
	}
	scopeID := strings.TrimSpace(entry.ScopeID)
	if scopeID == "" {
		if scope == MemoryScopeGlobal {
			scopeID = "global"
		} else {
			scopeID = entry.SessionID
		}
	}
	return MemoryEntry{
		ID:            entry.ID,
		SessionID:     entry.SessionID,
		Type:          entry.Type,
		Content:       entry.Content,
		Source:        entry.Source,
		Scope:         scope,
		ScopeID:       scopeID,
		Confidence:    confidence,
		SafetyLabel:   safetyLabel,
		Provenance:    provenance,
		ConflictGroup: entry.ConflictGroup,
		ExpiresAt:     entry.ExpiresAt,
		Relevance:     relevance,
		AccessCnt:     entry.AccessCnt,
		CreatedAt:     entry.CreatedAt,
		UpdatedAt:     entry.UpdatedAt,
		LastUsed:      entry.LastUsed,
	}
}

func memoryExpired(entry *MemoryEntry, now time.Time) bool {
	return entry != nil && entry.ExpiresAt > 0 && entry.ExpiresAt <= now.UnixMilli()
}

// tokenOverlap 计算两个 token 集合的 Jaccard 相似度
func tokenOverlap(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	smaller, larger := a, b
	if len(a) > len(b) {
		smaller, larger = b, a
	}
	intersection := 0
	for t := range smaller {
		if _, ok := larger[t]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func memoryInScope(entry *MemoryEntry, refs []MemoryScopeRef) bool {
	if entry == nil {
		return false
	}
	entryScope := entry.Scope
	if entryScope == "" {
		entryScope = MemoryScopeSession
	}
	entryScopeID := strings.TrimSpace(entry.ScopeID)
	if entryScopeID == "" {
		if entryScope == MemoryScopeGlobal {
			entryScopeID = "global"
		} else {
			entryScopeID = entry.SessionID
		}
	}
	for _, ref := range refs {
		scope := ref.Scope
		if scope == "" {
			scope = MemoryScopeSession
		}
		scopeID := strings.TrimSpace(ref.ScopeID)
		if scopeID == "" {
			if scope == MemoryScopeGlobal {
				scopeID = "global"
			} else {
				continue
			}
		}
		if entryScope == scope && entryScopeID == scopeID {
			return true
		}
	}
	return false
}

func (ltm *LongTermMemory) addLoadedEntryLocked(entry *MemoryEntry) {
	if entry == nil {
		return
	}
	cloned := cloneMemoryEntry(entry, entry.Relevance)
	if cloned.ID == "" {
		cloned.ID = generateMemoryID(cloned.Scope, cloned.ScopeID, cloned.Content)
	}
	if cloned.SessionID == "" {
		cloned.SessionID = cloned.ScopeID
	}
	ltm.entries[cloned.ID] = &cloned
	if cloned.SessionID != "" && !containsString(ltm.index[cloned.SessionID], cloned.ID) {
		ltm.index[cloned.SessionID] = append(ltm.index[cloned.SessionID], cloned.ID)
	}
}

func (ltm *LongTermMemory) retireConflictingMemoriesLocked(id string, memType MemoryType, opts MemoryStoreOptions, now time.Time) []*MemoryEntry {
	if opts.ConflictGroup == "" {
		return nil
	}
	var retired []*MemoryEntry
	for existingID, entry := range ltm.entries {
		if existingID == id || entry == nil {
			continue
		}
		if entry.Type != memType || entry.ConflictGroup != opts.ConflictGroup {
			continue
		}
		if entry.Scope != opts.Scope || entry.ScopeID != opts.ScopeID {
			continue
		}
		entry.SafetyLabel = "superseded"
		entry.ExpiresAt = now.UnixMilli()
		entry.Confidence = entry.Confidence * 0.5
		entry.UpdatedAt = now
		cloned := cloneMemoryEntry(entry, entry.Relevance)
		retired = append(retired, &cloned)
	}
	return retired
}

func (ltm *LongTermMemory) persistChangesLocked(ctx context.Context, changes pendingChanges) {
	if ltm.store == nil {
		return
	}
	if len(changes.upserts) > 0 {
		if err := ltm.store.SaveEntries(ctx, changes.upserts); err != nil {
			g.Log().Warningf(ctx, "[ltm] save entries failed: %v", err)
		}
	}
	if len(changes.deletes) > 0 {
		if err := ltm.store.DeleteEntries(ctx, changes.deletes); err != nil {
			g.Log().Warningf(ctx, "[ltm] delete entries failed: %v", err)
		}
	}
}

func (ltm *LongTermMemory) snapshotLocked() []*MemoryEntry {
	result := make([]*MemoryEntry, 0, len(ltm.entries))
	for _, entry := range ltm.entries {
		cloned := cloneMemoryEntry(entry, entry.Relevance)
		result = append(result, &cloned)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func (ltm *LongTermMemory) evictIfNeededLocked(ctx context.Context, sessionID string) []string {
	maxEntries := loadLongTermMaxEntries()
	maxPerSession := loadLongTermMaxEntriesPerSession()

	var evicted []string
	for maxPerSession > 0 && len(ltm.index[sessionID]) >= maxPerSession {
		if id := ltm.evictOneLocked(ctx, ltm.index[sessionID]); id != "" {
			evicted = append(evicted, id)
		}
	}
	for maxEntries > 0 && len(ltm.entries) >= maxEntries {
		ids := make([]string, 0, len(ltm.entries))
		for id := range ltm.entries {
			ids = append(ids, id)
		}
		if id := ltm.evictOneLocked(ctx, ids); id != "" {
			evicted = append(evicted, id)
		}
	}
	return evicted
}

func (ltm *LongTermMemory) evictOneLocked(ctx context.Context, candidateIDs []string) string {
	if len(candidateIDs) == 0 {
		return ""
	}
	sort.Slice(candidateIDs, func(i, j int) bool {
		left := ltm.entries[candidateIDs[i]]
		right := ltm.entries[candidateIDs[j]]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		leftScore := computeRelevance(left)
		rightScore := computeRelevance(right)
		if leftScore == rightScore {
			return left.LastUsed.Before(right.LastUsed)
		}
		return leftScore < rightScore
	})
	evictedID := candidateIDs[0]
	ltm.removeEntryLocked(evictedID)
	g.Log().Debugf(ctx, "[ltm] Evicted memory %s to enforce capacity", evictedID)
	return evictedID
}

func (ltm *LongTermMemory) removeEntryLocked(id string) {
	entry, ok := ltm.entries[id]
	if !ok {
		return
	}
	delete(ltm.entries, id)
	ids := ltm.index[entry.SessionID]
	for i, sid := range ids {
		if sid == id {
			ltm.index[entry.SessionID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(ltm.index[entry.SessionID]) == 0 {
		delete(ltm.index, entry.SessionID)
	}
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

type fileLongTermMemoryStore struct {
	path string
}

func NewFileLongTermMemoryStore(path string) LongTermMemoryStore {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &fileLongTermMemoryStore{path: path}
}

func (s *fileLongTermMemoryStore) LoadAll(ctx context.Context) ([]*MemoryEntry, error) {
	body, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, nil
	}
	var entries []*MemoryEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, err
	}
	return entries, ctx.Err()
}

func (s *fileLongTermMemoryStore) SaveEntries(ctx context.Context, entries []*MemoryEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := s.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("load existing memory store before save: %w", err)
	}
	byID := make(map[string]*MemoryEntry, len(existing)+len(entries))
	for _, e := range existing {
		byID[e.ID] = e
	}
	for _, e := range entries {
		byID[e.ID] = e
	}
	merged := make([]*MemoryEntry, 0, len(byID))
	for _, e := range byID {
		merged = append(merged, e)
	}
	return s.writeAll(merged)
}

func (s *fileLongTermMemoryStore) DeleteEntries(ctx context.Context, ids []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := s.LoadAll(ctx)
	if err != nil {
		return fmt.Errorf("load existing memory store before delete: %w", err)
	}
	deleteSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		deleteSet[id] = struct{}{}
	}
	remaining := make([]*MemoryEntry, 0, len(existing))
	for _, e := range existing {
		if _, deleted := deleteSet[e.ID]; !deleted {
			remaining = append(remaining, e)
		}
	}
	return s.writeAll(remaining)
}

func (s *fileLongTermMemoryStore) writeAll(entries []*MemoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func loadLongTermMemoryStore() LongTermMemoryStore {
	backend, _ := g.Cfg().Get(context.Background(), "memory.store_backend")
	switch strings.TrimSpace(backend.String()) {
	case "redis":
		return NewRedisMemoryStore(g.Redis())
	case "file":
		path, _ := g.Cfg().Get(context.Background(), "memory.long_term_store_path")
		if p := strings.TrimSpace(path.String()); p != "" {
			return NewFileLongTermMemoryStore(p)
		}
		g.Log().Warning(context.Background(), "[ltm] memory.store_backend=file requires memory.long_term_store_path")
		return nil
	default:
		// 向后兼容：旧配置 long_term_store_path 有值则走 file
		path, _ := g.Cfg().Get(context.Background(), "memory.long_term_store_path")
		if p := strings.TrimSpace(path.String()); p != "" {
			return NewFileLongTermMemoryStore(p)
		}
		return nil
	}
}
