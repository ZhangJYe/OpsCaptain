package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "FOO=bar\nexport HELLO=\"world\"\n# comment\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	os.Unsetenv("FOO")
	os.Unsetenv("HELLO")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("load env file: %v", err)
	}
	if got := os.Getenv("FOO"); got != "bar" {
		t.Fatalf("expected FOO=bar, got %q", got)
	}
	if got := os.Getenv("HELLO"); got != "world" {
		t.Fatalf("expected HELLO=world, got %q", got)
	}
}

func TestLoadPreferredEnvFileUsesSingleLocalEnv(t *testing.T) {
	dir := t.TempDir()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	t.Setenv("APP_ENV", "")
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("GO_ENV", "")
	oldSource, sourceExisted := os.LookupEnv("ENV_SOURCE")
	t.Cleanup(func() {
		if sourceExisted {
			_ = os.Setenv("ENV_SOURCE", oldSource)
		} else {
			_ = os.Unsetenv("ENV_SOURCE")
		}
	})
	if err := os.Unsetenv("ENV_SOURCE"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.local"), []byte("ENV_SOURCE=local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ENV_SOURCE=env\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := LoadPreferredEnvFile(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("ENV_SOURCE"); got != "env" {
		t.Fatalf("expected .env to be the only local source, got %q", got)
	}
}

func TestLooksLikePlaceholderSecretRejectsUnderscorePlaceholder(t *testing.T) {
	for _, value := range []string{"YOUR_API_KEY", "your_api_key", "your api key"} {
		if !LooksLikePlaceholderSecret(value) {
			t.Fatalf("expected %q to be recognized as a placeholder", value)
		}
	}
}
