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
	for _, check := range []struct {
		configPaths []string
		display     string
	}{
		{configPaths: []string{"chat_model_fast.api_key", "glm_chat_model_fast.api_key"}, display: "chat model API key"},
		{configPaths: []string{"embedding_model.api_key", "doubao_embedding_model.api_key"}, display: "embedding model API key"},
	} {
		value, err := firstConfiguredValue(ctx, check.configPaths...)
		if err != nil {
			return fmt.Errorf("failed to read %s from config: %w", check.display, err)
		}
		resolved, ok := ResolveOptionalEnv(value)
		if !ok || LooksLikePlaceholderSecret(resolved) {
			return fmt.Errorf("%s is not configured or still uses a placeholder", check.display)
		}
	}
	return nil
}

func firstConfiguredValue(ctx context.Context, paths ...string) (string, error) {
	var lastErr error
	for _, path := range paths {
		value, err := g.Cfg().Get(ctx, path)
		if err != nil {
			lastErr = err
			continue
		}
		if value.String() != "" {
			return value.String(), nil
		}
	}
	return "", lastErr
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
