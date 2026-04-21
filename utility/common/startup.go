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
		configPath string
		display    string
	}{
		{configPath: "ds_quick_chat_model.api_key", display: "GLM API key"},
		{configPath: "doubao_embedding_model.api_key", display: "SiliconFlow API key"},
	} {
		value, err := g.Cfg().Get(ctx, check.configPath)
		if err != nil {
			return fmt.Errorf("failed to read %s from config: %w", check.configPath, err)
		}
		resolved, ok := ResolveOptionalEnv(value.String())
		if !ok || LooksLikePlaceholderSecret(resolved) {
			return fmt.Errorf("%s is not configured or still uses a placeholder", check.display)
		}
	}
	return nil
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
