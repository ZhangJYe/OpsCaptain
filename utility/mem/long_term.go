package mem

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
	UpdatePolicy  string      `json:"update_policy,omitempty"`
	ConflictGroup string      `json:"conflict_group,omitempty"`
	ExpiresAt     int64       `json:"expires_at,omitempty"`
	AllowTriage   bool        `json:"allow_triage"`
	Relevance     float64     `json:"relevance"`
	AccessCnt     int         `json:"access_count"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
	LastUsed      time.Time   `json:"last_used"`
	Decay         float64     `json:"decay"`
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
	UpdatePolicy  string
	ConflictGroup string
	ExpiresAt     int64
	AllowTriage   bool
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
	Load(ctx context.Context) ([]*MemoryEntry, error)
	Save(ctx context.Context, entries []*MemoryEntry) error
}

const (
	defaultLongTermMaxEntries           = 1000
	defaultLongTermMaxEntriesPerSession = 100
)

var (
	globalLTM     *LongTermMemory
	globalLTMOnce sync.Once
)

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
	entries, err := store.Load(ctx)
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
		existing.UpdatePolicy = opts.UpdatePolicy
		existing.ConflictGroup = opts.ConflictGroup
		if opts.ExpiresAt > 0 {
			existing.ExpiresAt = opts.ExpiresAt
		}
		existing.AllowTriage = existing.AllowTriage || opts.AllowTriage
		existing.Relevance = computeRelevance(existing)
		ltm.retireConflictingMemoriesLocked(id, memType, opts, now)
		ltm.persistLocked(ctx)
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
		UpdatePolicy:  opts.UpdatePolicy,
		ConflictGroup: opts.ConflictGroup,
		ExpiresAt:     opts.ExpiresAt,
		AllowTriage:   opts.AllowTriage,
		Relevance:     1.0,
		AccessCnt:     1,
		CreatedAt:     now,
		UpdatedAt:     now,
		LastUsed:      now,
		Decay:         1.0,
	}

	ltm.evictIfNeededLocked(ctx, sessionID)
	ltm.retireConflictingMemoriesLocked(id, memType, opts, now)
	ltm.entries[id] = entry
	ltm.index[sessionID] = append(ltm.index[sessionID], id)
	ltm.persistLocked(ctx)

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
	ltm.mu.RLock()
	if len(ltm.entries) == 0 {
		ltm.mu.RUnlock()
		return nil
	}

	queryLower := strings.ToLower(query)
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
		contentLower := strings.ToLower(entry.Content)
		words := strings.Fields(queryLower)
		matchCount := 0
		for _, w := range words {
			if len(w) > 1 && strings.Contains(contentLower, w) {
				matchCount++
			}
		}
		if len(words) > 0 {
			score += float64(matchCount) / float64(len(words)) * 2.0
		}
		candidates = append(candidates, scored{
			id:    id,
			entry: cloneMemoryEntry(entry, relevance),
			score: score,
		})
	}
	ltm.mu.RUnlock()

	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[i].score {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	if limit > len(candidates) {
		limit = len(candidates)
	}

	result := make([]*MemoryEntry, 0, limit)
	selectedIDs := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		e := candidates[i].entry
		e.AccessCnt++
		e.LastUsed = time.Now()
		result = append(result, &e)
		selectedIDs = append(selectedIDs, candidates[i].id)
	}

	if len(selectedIDs) > 0 && !policy.ReadOnly {
		ltm.mu.Lock()
		now := time.Now()
		for _, id := range selectedIDs {
			if entry, ok := ltm.entries[id]; ok {
				entry.AccessCnt++
				entry.LastUsed = now
				entry.Relevance = computeRelevance(entry)
			}
		}
		ltm.persistLocked(ctx)
		ltm.mu.Unlock()
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
		ltm.persistLocked(ctx)
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
	ltm.persistLocked(ctx)
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
	entry.UpdatePolicy = "disabled"
	entry.ExpiresAt = now.UnixMilli()
	entry.UpdatedAt = now
	ltm.persistLocked(ctx)
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
	ltm.persistLocked(ctx)
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
	if opts.UpdatePolicy == "" {
		opts.UpdatePolicy = "reinforce"
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
	updatePolicy := entry.UpdatePolicy
	if updatePolicy == "" {
		updatePolicy = "reinforce"
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
		UpdatePolicy:  updatePolicy,
		ConflictGroup: entry.ConflictGroup,
		ExpiresAt:     entry.ExpiresAt,
		AllowTriage:   entry.AllowTriage,
		Relevance:     relevance,
		AccessCnt:     entry.AccessCnt,
		CreatedAt:     entry.CreatedAt,
		UpdatedAt:     entry.UpdatedAt,
		LastUsed:      entry.LastUsed,
		Decay:         entry.Decay,
	}
}

func memoryExpired(entry *MemoryEntry, now time.Time) bool {
	return entry != nil && entry.ExpiresAt > 0 && entry.ExpiresAt <= now.UnixMilli()
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

func (ltm *LongTermMemory) retireConflictingMemoriesLocked(id string, memType MemoryType, opts MemoryStoreOptions, now time.Time) {
	if opts.ConflictGroup == "" {
		return
	}
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
		entry.UpdatePolicy = "superseded"
		entry.ExpiresAt = now.UnixMilli()
		entry.Confidence = entry.Confidence * 0.5
		entry.UpdatedAt = now
	}
}

func (ltm *LongTermMemory) persistLocked(ctx context.Context) {
	if ltm.store == nil {
		return
	}
	if err := ltm.store.Save(ctx, ltm.snapshotLocked()); err != nil {
		g.Log().Warningf(ctx, "[ltm] save memory store failed: %v", err)
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

func (ltm *LongTermMemory) evictIfNeededLocked(ctx context.Context, sessionID string) {
	maxEntries := loadLongTermMaxEntries()
	maxPerSession := loadLongTermMaxEntriesPerSession()

	for maxPerSession > 0 && len(ltm.index[sessionID]) >= maxPerSession {
		ltm.evictOneLocked(ctx, ltm.index[sessionID])
	}
	for maxEntries > 0 && len(ltm.entries) >= maxEntries {
		ids := make([]string, 0, len(ltm.entries))
		for id := range ltm.entries {
			ids = append(ids, id)
		}
		ltm.evictOneLocked(ctx, ids)
	}
}

func (ltm *LongTermMemory) evictOneLocked(ctx context.Context, candidateIDs []string) {
	if len(candidateIDs) == 0 {
		return
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
	ltm.removeEntryLocked(candidateIDs[0])
	g.Log().Debugf(ctx, "[ltm] Evicted memory %s to enforce capacity", candidateIDs[0])
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

func (s *fileLongTermMemoryStore) Load(ctx context.Context) ([]*MemoryEntry, error) {
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

func (s *fileLongTermMemoryStore) Save(ctx context.Context, entries []*MemoryEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
	v, err := g.Cfg().Get(context.Background(), "memory.long_term_store_path")
	if err != nil || strings.TrimSpace(v.String()) == "" {
		return nil
	}
	return NewFileLongTermMemoryStore(v.String())
}
