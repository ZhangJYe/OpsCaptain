package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// WebhookAdapter 将不同平台的 webhook payload 解析为标准 ChangeEvent。
type WebhookAdapter interface {
	// Name 返回适配器名称（如 "github", "gitlab", "jenkins", "argocd"）。
	Name() string
	// Parse 将原始 webhook payload 解析为标准 ChangeEvent。
	Parse(ctx context.Context, headers http.Header, body []byte) (*protocol.ChangeEvent, error)
}

// WebhookAdapterConfig controls whether a provider webhook adapter is enabled
// and which shared secret is used to validate provider-signed requests.
type WebhookAdapterConfig struct {
	Enabled bool
	Secret  string
}

// AdapterRegistryConfig is assembled by main from config.yaml and environment
// variables. Secrets are intentionally passed in memory only and are never
// written to logs or response bodies.
type AdapterRegistryConfig struct {
	JSONEnabled bool
	GitHub      WebhookAdapterConfig
	GitLab      WebhookAdapterConfig
	ArgoCD      WebhookAdapterConfig
}

// JSONAdapter 是通用 JSON webhook 适配器。
// 接受标准 JSON 格式的变更事件，直接映射到 ChangeEvent 结构。
type JSONAdapter struct{}

// NewJSONAdapter 创建通用 JSON 适配器。
func NewJSONAdapter() *JSONAdapter {
	return &JSONAdapter{}
}

func (a *JSONAdapter) Name() string {
	return "json"
}

func (a *JSONAdapter) Parse(ctx context.Context, headers http.Header, body []byte) (*protocol.ChangeEvent, error) {
	var event protocol.ChangeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("invalid JSON payload: %w", err)
	}
	return &event, nil
}

// AdapterRegistry 管理所有注册的 webhook 适配器。
type AdapterRegistry struct {
	adapters map[string]WebhookAdapter
}

// NewAdapterRegistry 创建适配器注册表。
func NewAdapterRegistry() *AdapterRegistry {
	r := &AdapterRegistry{
		adapters: make(map[string]WebhookAdapter),
	}
	r.Register(NewJSONAdapter())
	r.Register(NewGitHubAdapter())
	r.Register(NewGitLabAdapter())
	r.Register(NewArgoCDAdapter())
	return r
}

// NewAdapterRegistryWithConfig creates the production registry. Provider
// adapters are registered only when explicitly enabled and backed by a secret.
func NewAdapterRegistryWithConfig(cfg AdapterRegistryConfig) *AdapterRegistry {
	r := &AdapterRegistry{
		adapters: make(map[string]WebhookAdapter),
	}
	if cfg.JSONEnabled {
		r.Register(NewJSONAdapter())
	}
	if cfg.GitHub.Enabled && strings.TrimSpace(cfg.GitHub.Secret) != "" {
		r.Register(NewGitHubAdapter(cfg.GitHub.Secret))
	}
	if cfg.GitLab.Enabled && strings.TrimSpace(cfg.GitLab.Secret) != "" {
		r.Register(NewGitLabAdapter(cfg.GitLab.Secret))
	}
	if cfg.ArgoCD.Enabled && strings.TrimSpace(cfg.ArgoCD.Secret) != "" {
		r.Register(NewArgoCDAdapter(cfg.ArgoCD.Secret))
	}
	return r
}

// Register 注册一个 webhook 适配器。
func (r *AdapterRegistry) Register(adapter WebhookAdapter) {
	if adapter == nil {
		return
	}
	r.adapters[adapter.Name()] = adapter
}

// Get 获取指定名称的适配器。
func (r *AdapterRegistry) Get(name string) (WebhookAdapter, bool) {
	a, ok := r.adapters[name]
	return a, ok
}

func verifyHMACSHA256Signature(secret, bodySignature string, body []byte) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	bodySignature = strings.TrimSpace(bodySignature)
	if bodySignature == "" {
		return fmt.Errorf("missing webhook signature")
	}
	signatureHex := strings.TrimPrefix(bodySignature, "sha256=")
	expectedMAC := hmac.New(sha256.New, []byte(secret))
	expectedMAC.Write(body)
	expected := expectedMAC.Sum(nil)
	actual, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("invalid webhook signature format")
	}
	if !hmac.Equal(actual, expected) {
		return fmt.Errorf("invalid webhook signature")
	}
	return nil
}

func verifySharedToken(secret, actual string) error {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return fmt.Errorf("missing webhook token")
	}
	if !hmac.Equal([]byte(actual), []byte(secret)) {
		return fmt.Errorf("invalid webhook token")
	}
	return nil
}

func inferEnv(text string) string {
	return normalizeEnv(text)
}

func normalizeEnv(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.Contains(lower, "production"), strings.Contains(lower, "prod"):
		return "prod"
	case strings.Contains(lower, "staging"), strings.Contains(lower, "stage"), strings.Contains(lower, "stg"):
		return "staging"
	case strings.Contains(lower, "development"), strings.Contains(lower, "dev"):
		return "dev"
	default:
		return "unknown"
	}
}
