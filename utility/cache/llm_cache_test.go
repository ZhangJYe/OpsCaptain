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
