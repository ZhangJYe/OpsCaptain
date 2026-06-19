package cmdb

import (
	"os"
	"path/filepath"
	"testing"
)

const testYAML = `services:
  - name: paymentservice
    display_name: "支付服务"
    owner: "张三"
    team: "支付团队"
    cluster: "prod-cluster-01"
    env: "production"
    dependencies:
      - userservice
      - cartservice
    tags:
      - critical
      - payment
  - name: userservice
    display_name: "用户服务"
    owner: "李四"
    team: "用户团队"
    cluster: "prod-cluster-01"
    env: "production"
    tags:
      - core
  - name: cartservice
    display_name: "购物车服务"
    owner: "王五"
    team: "交易团队"
    cluster: "prod-cluster-02"
    env: "production"
    dependencies:
      - userservice
`

func writeTestYAML(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "services.yaml")
	if err := os.WriteFile(path, []byte(testYAML), 0644); err != nil {
		t.Fatalf("write test yaml: %v", err)
	}
	return path
}

func TestYAMLLoader_GetService(t *testing.T) {
	path := writeTestYAML(t)
	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	svc, ok := loader.GetService("paymentservice")
	if !ok {
		t.Fatal("expected to find paymentservice")
	}
	if svc.DisplayName != "支付服务" {
		t.Errorf("DisplayName = %q, want %q", svc.DisplayName, "支付服务")
	}
	if svc.Owner != "张三" {
		t.Errorf("Owner = %q, want %q", svc.Owner, "张三")
	}
	if svc.Team != "支付团队" {
		t.Errorf("Team = %q, want %q", svc.Team, "支付团队")
	}
	if svc.Cluster != "prod-cluster-01" {
		t.Errorf("Cluster = %q, want %q", svc.Cluster, "prod-cluster-01")
	}
	if svc.Env != "production" {
		t.Errorf("Env = %q, want %q", svc.Env, "production")
	}
	if len(svc.Dependencies) != 2 {
		t.Errorf("Dependencies len = %d, want 2", len(svc.Dependencies))
	}
	if len(svc.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(svc.Tags))
	}

	_, ok = loader.GetService("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent service")
	}
}

func TestYAMLLoader_GetDependents(t *testing.T) {
	path := writeTestYAML(t)
	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	deps := loader.GetDependents("userservice")
	if len(deps) != 2 {
		t.Fatalf("GetDependents(userservice) len = %d, want 2", len(deps))
	}
	depMap := make(map[string]bool)
	for _, d := range deps {
		depMap[d] = true
	}
	if !depMap["paymentservice"] {
		t.Error("expected paymentservice to depend on userservice")
	}
	if !depMap["cartservice"] {
		t.Error("expected cartservice to depend on userservice")
	}

	deps = loader.GetDependents("cartservice")
	if len(deps) != 1 {
		t.Fatalf("GetDependents(cartservice) len = %d, want 1", len(deps))
	}
	if deps[0] != "paymentservice" {
		t.Errorf("GetDependents(cartservice)[0] = %q, want paymentservice", deps[0])
	}

	deps = loader.GetDependents("paymentservice")
	if len(deps) != 0 {
		t.Errorf("GetDependents(paymentservice) len = %d, want 0", len(deps))
	}
}

func TestYAMLLoader_SearchServices(t *testing.T) {
	path := writeTestYAML(t)
	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	results := loader.SearchServices("支付", 10)
	if len(results) != 1 {
		t.Fatalf("SearchServices(支付) len = %d, want 1", len(results))
	}
	if results[0].Name != "paymentservice" {
		t.Errorf("SearchServices(支付)[0].Name = %q, want paymentservice", results[0].Name)
	}

	results = loader.SearchServices("service", 10)
	if len(results) != 3 {
		t.Errorf("SearchServices(service) len = %d, want 3", len(results))
	}

	results = loader.SearchServices("critical", 10)
	if len(results) != 1 {
		t.Fatalf("SearchServices(critical) len = %d, want 1", len(results))
	}
	if results[0].Name != "paymentservice" {
		t.Errorf("SearchServices(critical)[0].Name = %q, want paymentservice", results[0].Name)
	}

	results = loader.SearchServices("不存在", 10)
	if len(results) != 0 {
		t.Errorf("SearchServices(不存在) len = %d, want 0", len(results))
	}

	results = loader.SearchServices("service", 2)
	if len(results) != 2 {
		t.Errorf("SearchServices(service, limit=2) len = %d, want 2", len(results))
	}
}

func TestYAMLLoader_ListByCluster(t *testing.T) {
	path := writeTestYAML(t)
	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	results := loader.ListServicesByCluster("prod-cluster-01")
	if len(results) != 2 {
		t.Fatalf("ListByCluster(prod-cluster-01) len = %d, want 2", len(results))
	}

	results = loader.ListServicesByCluster("prod-cluster-02")
	if len(results) != 1 {
		t.Fatalf("ListByCluster(prod-cluster-02) len = %d, want 1", len(results))
	}
	if results[0].Name != "cartservice" {
		t.Errorf("ListByCluster(prod-cluster-02)[0].Name = %q, want cartservice", results[0].Name)
	}

	results = loader.ListServicesByCluster("nonexistent")
	if len(results) != 0 {
		t.Errorf("ListByCluster(nonexistent) len = %d, want 0", len(results))
	}
}

func TestYAMLLoader_ListByTeam(t *testing.T) {
	path := writeTestYAML(t)
	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	results := loader.ListServicesByTeam("支付团队")
	if len(results) != 1 {
		t.Fatalf("ListByTeam(支付团队) len = %d, want 1", len(results))
	}
	if results[0].Name != "paymentservice" {
		t.Errorf("ListByTeam(支付团队)[0].Name = %q, want paymentservice", results[0].Name)
	}

	results = loader.ListServicesByTeam("nonexistent")
	if len(results) != 0 {
		t.Errorf("ListByTeam(nonexistent) len = %d, want 0", len(results))
	}
}

func TestYAMLLoader_FileNotFound(t *testing.T) {
	_, err := NewYAMLLoader("/nonexistent/path/services.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestYAMLLoader_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yaml")
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		t.Fatalf("write empty yaml: %v", err)
	}

	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	all := loader.ListAll()
	if len(all) != 0 {
		t.Errorf("ListAll on empty file len = %d, want 0", len(all))
	}
}

func TestYAMLLoader_ListAll(t *testing.T) {
	path := writeTestYAML(t)
	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	all := loader.ListAll()
	if len(all) != 3 {
		t.Errorf("ListAll len = %d, want 3", len(all))
	}
}

func TestYAMLLoader_Reload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "services.yaml")
	if err := os.WriteFile(path, []byte(testYAML), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	loader, err := NewYAMLLoader(path)
	if err != nil {
		t.Fatalf("NewYAMLLoader: %v", err)
	}

	all := loader.ListAll()
	if len(all) != 3 {
		t.Fatalf("initial ListAll len = %d, want 3", len(all))
	}

	emptyYAML := "services: []\n"
	if err := os.WriteFile(path, []byte(emptyYAML), 0644); err != nil {
		t.Fatalf("overwrite yaml: %v", err)
	}

	if err := loader.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	all = loader.ListAll()
	if len(all) != 0 {
		t.Errorf("after reload ListAll len = %d, want 0", len(all))
	}
}
