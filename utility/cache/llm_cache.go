package cache

import (
	"SuperBizAgent/utility/common"
	"SuperBizAgent/utility/metrics"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultLLMResponseTTL = 5 * time.Minute
	llmResponseCacheType  = "llm_response"
	llmResponseCacheVsn   = "v2"
)

type ChatResponseEntry struct {
	Answer   string   `json:"answer"`
	Detail   []string `json:"detail,omitempty"`
	Mode     string   `json:"mode,omitempty"`
	CachedAt int64    `json:"cached_at"`
}

func LoadChatResponse(ctx context.Context, sessionID, query string, scope ...string) (ChatResponseEntry, bool, error) {
	if !responseCacheEnabled(ctx) {
		return ChatResponseEntry{}, false, nil
	}

	key := responseCacheKey(sessionID, query, scope...)
	value, err := g.Redis().Do(ctx, "GET", key)
	if err != nil {
		return ChatResponseEntry{}, false, err
	}
	raw := strings.TrimSpace(value.String())
	if raw == "" {
		metrics.IncCacheMiss(llmResponseCacheType)
		g.Log().Infof(ctx, "[cache] miss type=%s key=%s", llmResponseCacheType, key)
		return ChatResponseEntry{}, false, nil
	}

	var entry ChatResponseEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		return ChatResponseEntry{}, false, err
	}
	metrics.IncCacheHit(llmResponseCacheType)
	g.Log().Infof(ctx, "[cache] hit type=%s key=%s", llmResponseCacheType, key)
	return entry, true, nil
}

func StoreChatResponse(ctx context.Context, sessionID, query string, entry ChatResponseEntry, scope ...string) error {
	if !responseCacheEnabled(ctx) {
		return nil
	}
	if strings.TrimSpace(entry.Answer) == "" {
		return nil
	}

	entry.CachedAt = time.Now().Unix()
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	key := responseCacheKey(sessionID, query, scope...)
	ttl := int(responseCacheTTL(ctx).Seconds())
	_, err = g.Redis().Do(ctx, "SETEX", key, ttl, string(payload))
	return err
}

func responseCacheEnabled(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "cache.llm_response.enabled")
	if err != nil || !v.Bool() {
		return false
	}
	addr, err := g.Cfg().Get(ctx, "redis.default.address")
	if err != nil {
		return false
	}
	_, ok := common.ResolveOptionalEnv(addr.String())
	return ok
}

func responseCacheTTL(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "cache.llm_response.ttl_seconds")
	if err != nil || v.Int64() <= 0 {
		return defaultLLMResponseTTL
	}
	return time.Duration(v.Int64()) * time.Second
}

func responseCacheKey(sessionID, query string, scope ...string) string {
	parts := []string{strings.TrimSpace(query)}
	normalizedScope := make([]string, 0, len(scope))
	seen := make(map[string]bool, len(scope))
	for _, item := range scope {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := strings.ToLower(item)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalizedScope = append(normalizedScope, item)
	}
	sort.Strings(normalizedScope)
	parts = append(parts, normalizedScope...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return fmt.Sprintf("opscaptionai:cache:chat:%s:%s:%s", llmResponseCacheVsn, strings.TrimSpace(sessionID), hex.EncodeToString(sum[:8]))
}
