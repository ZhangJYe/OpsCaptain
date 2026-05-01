package cache

import (
	"strings"
	"testing"
)

func TestResponseCacheKeyIncludesVersionPrefix(t *testing.T) {
	key := responseCacheKey("session-1", "hello")
	if !strings.HasPrefix(key, "opscaptionai:cache:chat:"+llmResponseCacheVsn+":") {
		t.Fatalf("expected versioned cache key, got %q", key)
	}
}

func TestResponseCacheKeySeparatesSkillScopedRequests(t *testing.T) {
	base := responseCacheKey("session-1", "hello")
	withSkills := responseCacheKey("session-1", "hello", "logs_evidence_extract")
	if base == withSkills {
		t.Fatalf("expected scoped cache key to differ when selected skills change")
	}
}

func TestResponseCacheKeyIgnoresSkillOrder(t *testing.T) {
	left := responseCacheKey("session-1", "hello", "logs_evidence_extract", "knowledge_sop_lookup")
	right := responseCacheKey("session-1", "hello", "knowledge_sop_lookup", "logs_evidence_extract")
	if left != right {
		t.Fatalf("expected cache key to stay stable across skill order, got %q vs %q", left, right)
	}
}
