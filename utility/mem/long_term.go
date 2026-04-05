package mem

import (
	"context"
	"crypto/sha256"
	"fmt"
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

type MemoryEntry struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	Type      MemoryType `json:"type"`
	Content   string     `json:"content"`
	Source    string     `json:"source"`
	Relevance float64    `json:"relevance"`
	AccessCnt int        `json:"access_count"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastUsed  time.Time  `json:"last_used"`
	Decay     float64    `json:"decay"`
}

type LongTermMemory struct {
	entries map[string]*MemoryEntry
	index   map[string][]string
	mu      sync.RWMutex
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
		globalLTM = &LongTermMemory{
			entries: make(map[string]*MemoryEntry),
			index:   make(map[string][]string),
		}
	})
	return globalLTM
}

func generateMemoryID(sessionID, content string) string {
	h := sha256.Sum256([]byte(sessionID + ":" + content))
	return fmt.Sprintf("%x", h[:8])
}

func (ltm *LongTermMemory) Store(ctx context.Context, sessionID string, memType MemoryType, content string, source string) string {
	ltm.mu.Lock()
	defer ltm.mu.Unlock()

	id := generateMemoryID(sessionID, content)

	if existing, ok := ltm.entries[id]; ok {
		existing.AccessCnt++
		existing.UpdatedAt = time.Now()
		existing.LastUsed = time.Now()
		existing.Relevance = computeRelevance(existing)
		g.Log().Debugf(ctx, "[ltm] Reinforced memory %s, access count: %d", id, existing.AccessCnt)
		return id
	}

	entry := &MemoryEntry{
		ID:        id,
		SessionID: sessionID,
		Type:      memType,
		Content:   content,
		Source:    source,
		Relevance: 1.0,
		AccessCnt: 1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		LastUsed:  time.Now(),
		Decay:     1.0,
	}

	ltm.evictIfNeededLocked(ctx, sessionID)
	ltm.entries[id] = entry
	ltm.index[sessionID] = append(ltm.index[sessionID], id)

	g.Log().Debugf(ctx, "[ltm] Stored new %s memory %s for session %s", memType, id, sessionID)
	return id
}

func (ltm *LongTermMemory) Retrieve(ctx context.Context, sessionID string, query string, limit int) []*MemoryEntry {
	ltm.mu.RLock()

	ids, ok := ltm.index[sessionID]
	if !ok || len(ids) == 0 {
		ltm.mu.RUnlock()
		return nil
	}

	queryLower := strings.ToLower(query)
	type scored struct {
		id    string
		entry MemoryEntry
		score float64
	}
	var candidates []scored
	for _, id := range ids {
		entry, ok := ltm.entries[id]
		if !ok {
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
			id: id,
			entry: MemoryEntry{
				ID:        entry.ID,
				SessionID: entry.SessionID,
				Type:      entry.Type,
				Content:   entry.Content,
				Source:    entry.Source,
				Relevance: relevance,
				AccessCnt: entry.AccessCnt,
				CreatedAt: entry.CreatedAt,
				UpdatedAt: entry.UpdatedAt,
				LastUsed:  entry.LastUsed,
				Decay:     entry.Decay,
			},
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
		result = append(result, &MemoryEntry{
			ID:        e.ID,
			SessionID: e.SessionID,
			Type:      e.Type,
			Content:   e.Content,
			Source:    e.Source,
			Relevance: e.Relevance,
			AccessCnt: e.AccessCnt + 1,
			CreatedAt: e.CreatedAt,
			UpdatedAt: e.UpdatedAt,
			LastUsed:  time.Now(),
			Decay:     e.Decay,
		})
		selectedIDs = append(selectedIDs, candidates[i].id)
	}

	if len(selectedIDs) > 0 {
		ltm.mu.Lock()
		now := time.Now()
		for _, id := range selectedIDs {
			if entry, ok := ltm.entries[id]; ok {
				entry.AccessCnt++
				entry.LastUsed = now
				entry.Relevance = computeRelevance(entry)
			}
		}
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
			result = append(result, &MemoryEntry{
				ID:        e.ID,
				SessionID: e.SessionID,
				Type:      e.Type,
				Content:   e.Content,
				Source:    e.Source,
				Relevance: computeRelevance(e),
				AccessCnt: e.AccessCnt,
				CreatedAt: e.CreatedAt,
				UpdatedAt: e.UpdatedAt,
				LastUsed:  e.LastUsed,
				Decay:     e.Decay,
			})
		}
	}
	return result
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
