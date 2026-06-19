package tools

import (
	"SuperBizAgent/internal/ai/cmdb"
	"context"
	"encoding/json"
	"testing"
)

type fakeCMDBRepo struct {
	services map[string]*cmdb.ServiceInfo
}

func (f *fakeCMDBRepo) GetService(name string) (*cmdb.ServiceInfo, bool) {
	s, ok := f.services[name]
	return s, ok
}

func (f *fakeCMDBRepo) SearchServices(keyword string, limit int) []cmdb.ServiceInfo {
	var result []cmdb.ServiceInfo
	for _, s := range f.services {
		if contains(s.Name, keyword) || contains(s.DisplayName, keyword) || contains(s.Description, keyword) {
			result = append(result, *s)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result
}

func (f *fakeCMDBRepo) ListServicesByCluster(cluster string) []cmdb.ServiceInfo {
	var result []cmdb.ServiceInfo
	for _, s := range f.services {
		if s.Cluster == cluster {
			result = append(result, *s)
		}
	}
	return result
}

func (f *fakeCMDBRepo) ListServicesByTeam(team string) []cmdb.ServiceInfo {
	var result []cmdb.ServiceInfo
	for _, s := range f.services {
		if s.Team == team {
			result = append(result, *s)
		}
	}
	return result
}

func (f *fakeCMDBRepo) GetDependents(name string) []string {
	s, ok := f.services[name]
	if !ok {
		return nil
	}
	return s.Dependents
}

func (f *fakeCMDBRepo) ListAll() []cmdb.ServiceInfo {
	var result []cmdb.ServiceInfo
	for _, s := range f.services {
		result = append(result, *s)
	}
	return result
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || len(s) > 0 && len(sub) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestQueryCMDBTool_GetService(t *testing.T) {
	repo := &fakeCMDBRepo{
		services: map[string]*cmdb.ServiceInfo{
			"order-service": {
				Name:        "order-service",
				DisplayName: "Order Service",
				Team:        "platform",
				Cluster:     "prod-cluster",
				Env:         "production",
				Dependencies: []string{"payment-service", "inventory-service"},
				Dependents:   []string{"gateway-service"},
			},
		},
	}

	tool := NewQueryCMDBTool(repo)
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}

	output, err := tool.InvokableRun(context.Background(), `{"action":"get_service","service_name":"order-service"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result QueryCMDBOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if !result.Success {
		t.Fatalf("expected success=true, got degraded=%v error=%q", result.Degraded, result.Error)
	}
	if result.Action != "get_service" {
		t.Fatalf("expected action=get_service, got %q", result.Action)
	}
	if result.Service == nil {
		t.Fatal("expected service to be set")
	}
	if result.Service.Name != "order-service" {
		t.Fatalf("expected service name=order-service, got %q", result.Service.Name)
	}
}

func TestQueryCMDBTool_NilRepo(t *testing.T) {
	tool := NewQueryCMDBTool(nil)
	if tool == nil {
		t.Fatal("expected non-nil tool")
	}

	output, err := tool.InvokableRun(context.Background(), `{"action":"get_service","service_name":"order-service"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result QueryCMDBOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	if result.Success {
		t.Fatal("expected success=false for nil repo")
	}
	if !result.Degraded {
		t.Fatal("expected degraded=true for nil repo")
	}
	if result.Error == "" {
		t.Fatal("expected error message for nil repo")
	}
}
