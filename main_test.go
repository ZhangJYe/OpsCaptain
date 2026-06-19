package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCMDBStorePathUsesConfiguredPathWhenPresent(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("docs", "assets"), 0755); err != nil {
		t.Fatalf("mkdir docs assets: %v", err)
	}
	configured := filepath.Join("docs", "assets", "services.yaml")
	if err := os.WriteFile(configured, []byte("services: []\n"), 0644); err != nil {
		t.Fatalf("write configured cmdb: %v", err)
	}

	if got := resolveCMDBStorePath(configured); got != configured {
		t.Fatalf("resolveCMDBStorePath() = %q, want %q", got, configured)
	}
}

func TestResolveCMDBStorePathCopiesPackagedSeedToConfiguredPath(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join("manifest", "cmdb"), 0755); err != nil {
		t.Fatalf("mkdir manifest cmdb: %v", err)
	}
	fallback := filepath.Join("manifest", "cmdb", "services.yaml")
	if err := os.WriteFile(fallback, []byte("services: []\n"), 0644); err != nil {
		t.Fatalf("write fallback cmdb: %v", err)
	}

	configured := filepath.Join("var", "runtime", "cmdb", "services.yaml")
	if got := resolveCMDBStorePath(configured); got != configured {
		t.Fatalf("resolveCMDBStorePath() = %q, want %q", got, configured)
	}
	copied, err := os.ReadFile(configured)
	if err != nil {
		t.Fatalf("read copied cmdb: %v", err)
	}
	if string(copied) != "services: []\n" {
		t.Fatalf("copied seed = %q", copied)
	}
}
