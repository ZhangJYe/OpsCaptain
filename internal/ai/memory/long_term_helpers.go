package memory

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

func generateMemoryID(scope MemoryScope, scopeID, content string) string {
	h := sha256.Sum256([]byte(string(scope) + ":" + scopeID + ":" + content))
	return fmt.Sprintf("%x", h[:8])
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

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
