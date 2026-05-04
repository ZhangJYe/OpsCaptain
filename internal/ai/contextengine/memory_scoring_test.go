package contextengine

import (
	"testing"
	"time"

	"SuperBizAgent/utility/mem"

	"github.com/stretchr/testify/assert"
)

func TestMemoryCompositeScore_ConfidenceDominates(t *testing.T) {
	now := time.Now()
	profile := ContextProfile{}

	highConf := &mem.MemoryEntry{
		Confidence: 0.9,
		LastUsed:   now,
		Scope:      mem.MemoryScopeSession,
	}
	lowConf := &mem.MemoryEntry{
		Confidence: 0.2,
		LastUsed:   now,
		Scope:      mem.MemoryScopeSession,
	}

	high := memoryCompositeScore(highConf, profile, now)
	low := memoryCompositeScore(lowConf, profile, now)
	assert.Greater(t, high, low)
}

func TestMemoryCompositeScore_FreshnessMatters(t *testing.T) {
	now := time.Now()
	profile := ContextProfile{}

	recent := &mem.MemoryEntry{
		Confidence: 0.7,
		LastUsed:   now,
		Scope:      mem.MemoryScopeSession,
	}
	old := &mem.MemoryEntry{
		Confidence: 0.7,
		LastUsed:   now.Add(-72 * time.Hour),
		Scope:      mem.MemoryScopeSession,
	}

	recentScore := memoryCompositeScore(recent, profile, now)
	oldScore := memoryCompositeScore(old, profile, now)
	assert.Greater(t, recentScore, oldScore)
}

func TestMemoryCompositeScore_ScopePriority(t *testing.T) {
	now := time.Now()
	profile := ContextProfile{}

	session := &mem.MemoryEntry{
		Confidence: 0.7,
		LastUsed:   now,
		Scope:      mem.MemoryScopeSession,
	}
	global := &mem.MemoryEntry{
		Confidence: 0.7,
		LastUsed:   now,
		Scope:      mem.MemoryScopeGlobal,
	}

	sessionScore := memoryCompositeScore(session, profile, now)
	globalScore := memoryCompositeScore(global, profile, now)
	assert.Greater(t, sessionScore, globalScore)
}

func TestRankMemoryEntries_SortsCorrectly(t *testing.T) {
	now := time.Now()
	profile := ContextProfile{}

	entries := []*mem.MemoryEntry{
		{ID: "low", Confidence: 0.3, LastUsed: now.Add(-48 * time.Hour), Scope: mem.MemoryScopeGlobal},
		{ID: "high", Confidence: 0.9, LastUsed: now, Scope: mem.MemoryScopeSession},
		{ID: "mid", Confidence: 0.6, LastUsed: now.Add(-12 * time.Hour), Scope: mem.MemoryScopeUser},
	}

	ranked := rankMemoryEntries(entries, profile, now)
	assert.Equal(t, "high", ranked[0].ID)
	assert.Equal(t, "mid", ranked[1].ID)
	assert.Equal(t, "low", ranked[2].ID)
}

func TestRankMemoryEntries_SingleEntry(t *testing.T) {
	entries := []*mem.MemoryEntry{
		{ID: "only", Confidence: 0.5},
	}
	ranked := rankMemoryEntries(entries, ContextProfile{}, time.Now())
	assert.Len(t, ranked, 1)
	assert.Equal(t, "only", ranked[0].ID)
}

func TestRankMemoryEntries_Empty(t *testing.T) {
	ranked := rankMemoryEntries(nil, ContextProfile{}, time.Now())
	assert.Nil(t, ranked)
}
