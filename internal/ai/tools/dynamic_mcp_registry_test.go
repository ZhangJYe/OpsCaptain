package tools

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"SuperBizAgent/internal/ai/skills"
)

func TestDynamicMCPRegistry_Whitelist_PrivateIPAllowed(t *testing.T) {
	r, err := NewDynamicMCPRegistry([]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	// 127.0.0.1 is in 10.0.0.0/8? No, but let's add loopback.
	// Actually 127.0.0.1 is not in those CIDRs. Let's test with a proper one.
	if err := r.checkWhitelist("http://10.1.2.3:8080/sse"); err != nil {
		t.Errorf("expected 10.1.2.3 to pass whitelist, got: %v", err)
	}
	if err := r.checkWhitelist("http://192.168.1.100:9090/sse"); err != nil {
		t.Errorf("expected 192.168.1.100 to pass whitelist, got: %v", err)
	}
}

func TestDynamicMCPRegistry_Whitelist_PublicIPRejected(t *testing.T) {
	r, err := NewDynamicMCPRegistry([]string{"10.0.0.0/8"}, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	// 8.8.8.8 is a public IP that should be rejected.
	err = r.checkWhitelist("http://8.8.8.8:8080/sse")
	if err == nil {
		t.Error("expected public IP 8.8.8.8 to be rejected by whitelist")
	}
}

func TestDynamicMCPRegistry_Whitelist_NoWhitelistAllowsAll(t *testing.T) {
	r, err := NewDynamicMCPRegistry(nil, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	if err := r.checkWhitelist("http://8.8.8.8:8080/sse"); err != nil {
		t.Errorf("expected no-whitelist to allow all, got: %v", err)
	}
	if err := r.checkWhitelist("http://example.com:8080/sse"); err != nil {
		t.Errorf("expected no-whitelist to allow hostname, got: %v", err)
	}
}

func TestDynamicMCPRegistry_Whitelist_InvalidCIDR(t *testing.T) {
	_, err := NewDynamicMCPRegistry([]string{"not-a-cidr"}, 5000)
	if err == nil {
		t.Error("expected error for invalid CIDR")
	}
}

func TestDynamicMCPRegistry_Whitelist_LoopbackWithCIDR(t *testing.T) {
	r, err := NewDynamicMCPRegistry([]string{"127.0.0.0/8"}, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	if err := r.checkWhitelist("http://127.0.0.1:3000/sse"); err != nil {
		t.Errorf("expected loopback to pass, got: %v", err)
	}
}

func TestDynamicMCPRegistry_Get_NonExistent(t *testing.T) {
	r, err := NewDynamicMCPRegistry(nil, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	_, ok := r.Get("non-existent-id")
	if ok {
		t.Error("expected Get for non-existent tool to return false")
	}
}

func TestDynamicMCPRegistry_Invoke_NonExistent_Degraded(t *testing.T) {
	r, err := NewDynamicMCPRegistry(nil, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	result, err := r.Invoke(context.Background(), "missing-tool", `{"key":"value"}`)
	if err != nil {
		t.Fatalf("Invoke should not return error for degraded mode, got: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got: %v (raw: %s)", err, result)
	}
	if degraded, _ := parsed["degraded"].(bool); !degraded {
		t.Errorf("expected degraded=true, got %v", parsed["degraded"])
	}
}

func TestDynamicMCPRegistry_Unregister_RemovesTool(t *testing.T) {
	r, err := NewDynamicMCPRegistry(nil, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	// Manually insert a dummy connection to test Unregister.
	r.mu.Lock()
	r.connections["test-id"] = &mcpConn{
		config: skills.UserMCPTool{ID: "test-id", Name: "test"},
	}
	r.mu.Unlock()

	_, ok := r.Get("test-id")
	if !ok {
		t.Fatal("expected tool to be present before Unregister")
	}

	r.Unregister("test-id")

	_, ok = r.Get("test-id")
	if ok {
		t.Error("expected tool to be removed after Unregister")
	}
}

func TestDynamicMCPRegistry_ListConfigs(t *testing.T) {
	r, err := NewDynamicMCPRegistry(nil, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	configs := r.ListConfigs()
	if len(configs) != 0 {
		t.Errorf("expected empty list, got %d", len(configs))
	}

	r.mu.Lock()
	r.connections["a"] = &mcpConn{config: skills.UserMCPTool{ID: "a", Name: "tool-a"}}
	r.connections["b"] = &mcpConn{config: skills.UserMCPTool{ID: "b", Name: "tool-b"}}
	r.mu.Unlock()

	configs = r.ListConfigs()
	if len(configs) != 2 {
		t.Errorf("expected 2 configs, got %d", len(configs))
	}
}

func TestDynamicMCPRegistry_Whitelist_IPv6(t *testing.T) {
	r, err := NewDynamicMCPRegistry([]string{"::1/128"}, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	if err := r.checkWhitelist("http://[::1]:8080/sse"); err != nil {
		t.Errorf("expected IPv6 loopback to pass, got: %v", err)
	}
}

func TestDynamicMCPRegistry_Whitelist_DirectIP(t *testing.T) {
	r, err := NewDynamicMCPRegistry([]string{"10.0.0.0/8"}, 5000)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	// Test with a raw IP URL (no hostname resolution needed).
	ip := net.ParseIP("10.5.5.5")
	if ip == nil {
		t.Fatal("failed to parse IP")
	}
	if err := r.checkWhitelist("http://10.5.5.5:8080/sse"); err != nil {
		t.Errorf("expected 10.5.5.5 to pass, got: %v", err)
	}
}

func TestDynamicMCPRegistry_DefaultTimeout(t *testing.T) {
	r, err := NewDynamicMCPRegistry(nil, 0)
	if err != nil {
		t.Fatalf("NewDynamicMCPRegistry: %v", err)
	}
	if r.timeoutMs != defaultDynamicTimeoutMs {
		t.Errorf("expected default timeout %d, got %d", defaultDynamicTimeoutMs, r.timeoutMs)
	}
}

func TestDegradedJSON(t *testing.T) {
	out := degradedJSON("test reason")
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["degraded"] != true {
		t.Errorf("expected degraded=true, got %v", parsed["degraded"])
	}
	if parsed["reason"] != "test reason" {
		t.Errorf("expected reason='test reason', got %v", parsed["reason"])
	}
}
