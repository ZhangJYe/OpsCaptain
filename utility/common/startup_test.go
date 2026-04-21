package common

import "testing"

func TestBuildRedisConfigMapResolvesEnvPlaceholders(t *testing.T) {
	t.Setenv("REDIS_ADDRESS", "redis:6379")
	t.Setenv("REDIS_PASSWORD", "")

	configMap, ok := buildRedisConfigMap(redisRawConfig{
		Address: "${REDIS_ADDRESS}",
		DB:      2,
		Pass:    "${REDIS_PASSWORD}",
	})
	if !ok {
		t.Fatal("expected redis config to be built")
	}
	if got := configMap["address"]; got != "redis:6379" {
		t.Fatalf("expected resolved address redis:6379, got %#v", got)
	}
	if got := configMap["db"]; got != 2 {
		t.Fatalf("expected db 2, got %#v", got)
	}
	if _, ok := configMap["pass"]; ok {
		t.Fatal("expected empty redis password to be omitted")
	}
}

func TestBuildRedisConfigMapSkipsUnresolvedAddress(t *testing.T) {
	t.Setenv("REDIS_ADDRESS", "")

	if _, ok := buildRedisConfigMap(redisRawConfig{Address: "${REDIS_ADDRESS}"}); ok {
		t.Fatal("expected unresolved redis address to be skipped")
	}
}
