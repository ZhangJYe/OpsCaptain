package common

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/encoding/gyaml"
)

func TestLoadCanonicalModelConfigs(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	t.Setenv("ARK_API_KEY", "test-ark-key")
	ctx := context.Background()

	chat, err := LoadChatModelConfig(ctx, ChatModelFast)
	if err != nil {
		t.Fatalf("load chat model config: %v", err)
	}
	if chat.Provider != "deepseek" || chat.APIKey != "test-deepseek-key" {
		t.Fatalf("unexpected chat model config: provider=%q api_key_configured=%t", chat.Provider, chat.APIKey != "")
	}

	embedding, err := LoadEmbeddingModelConfig(ctx)
	if err != nil {
		t.Fatalf("load embedding model config: %v", err)
	}
	if embedding.Provider != "doubao" || embedding.APIKey != "test-ark-key" || embedding.Dimension != 2048 {
		t.Fatalf("unexpected embedding model config: provider=%q api_key_configured=%t dimension=%d", embedding.Provider, embedding.APIKey != "", embedding.Dimension)
	}
}

func TestLoadChatModelConfigRejectsLegacyAlias(t *testing.T) {
	if _, err := LoadChatModelConfig(context.Background(), "glm_chat_model"); err == nil {
		t.Fatal("expected legacy chat model alias to be rejected")
	}
}

type modelConfigDocument struct {
	ChatModel     modelConfigYAML `yaml:"chat_model"`
	ChatModelFast modelConfigYAML `yaml:"chat_model_fast"`
	Embedding     modelConfigYAML `yaml:"embedding_model"`
}

type modelConfigYAML struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	APIKey    string `yaml:"api_key"`
	BaseURL   string `yaml:"base_url"`
	Dimension int    `yaml:"dimension,omitempty"`
}

func TestDevelopmentAndProductionModelConfigsStayAligned(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	development := readModelConfigDocument(t, filepath.Join(repoRoot, "manifest", "config", "config.yaml"))
	production := readModelConfigDocument(t, filepath.Join(repoRoot, "deploy", "config.prod.yaml"))

	if !reflect.DeepEqual(development, production) {
		t.Fatalf("model configuration drifted between manifest/config/config.yaml and deploy/config.prod.yaml")
	}
	if development.ChatModel.Provider != "deepseek" || development.ChatModelFast.Provider != "deepseek" {
		t.Fatal("chat models must keep the existing deepseek provider")
	}
	if development.Embedding.Provider != "doubao" {
		t.Fatal("embedding model must keep the existing doubao provider")
	}
}

func readModelConfigDocument(t *testing.T, path string) modelConfigDocument {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var document modelConfigDocument
	if err := gyaml.DecodeTo(data, &document); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return document
}
