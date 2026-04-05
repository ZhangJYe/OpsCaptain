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
