package common

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
)

type redisRawConfig struct {
	Address string
	DB      int
	User    string
	Pass    string
}

func ConfigureRedis(ctx context.Context) error {
	configMap, ok := buildRedisConfigMap(loadRedisRawConfig(ctx))
	if !ok {
		return nil
	}
	return gredis.SetConfigByMap(configMap)
}

func ValidateStartupSecrets(ctx context.Context) error {
	if _, err := LoadChatModelConfig(ctx, ChatModelFast); err != nil {
		return fmt.Errorf("chat model configuration is invalid: %w", err)
	}
	if _, err := LoadEmbeddingModelConfig(ctx); err != nil {
		return fmt.Errorf("embedding model configuration is invalid: %w", err)
	}
	return nil
}

func RequireStartupModelSecrets(ctx context.Context) bool {
	v, err := g.Cfg().Get(ctx, "startup.require_model_secrets")
	if err != nil {
		return false
	}
	return v.Bool()
}

func loadRedisRawConfig(ctx context.Context) redisRawConfig {
	var raw redisRawConfig
	if v, err := g.Cfg().Get(ctx, "redis.default.address"); err == nil {
		raw.Address = v.String()
	}
	if v, err := g.Cfg().Get(ctx, "redis.default.db"); err == nil {
		raw.DB = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "redis.default.user"); err == nil {
		raw.User = v.String()
	}
	if v, err := g.Cfg().Get(ctx, "redis.default.pass"); err == nil {
		raw.Pass = v.String()
	}
	return raw
}

func buildRedisConfigMap(raw redisRawConfig) (map[string]any, bool) {
	address, ok := ResolveOptionalEnv(raw.Address)
	if !ok {
		return nil, false
	}
	configMap := map[string]any{
		"address": address,
		"db":      raw.DB,
	}
	if user, ok := ResolveOptionalEnv(raw.User); ok {
		configMap["user"] = user
	}
	if pass, ok := ResolveOptionalEnv(raw.Pass); ok {
		configMap["pass"] = pass
	}
	return configMap, true
}
