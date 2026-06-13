package memory

import (
	"context"
	"sort"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

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
