package common

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/frame/g"
)

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
