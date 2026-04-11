package middleware

import (
	"SuperBizAgent/utility/auth"
	"net/http"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
)

func TestSSEContentTypeDetection(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
		isSSE       bool
	}{
		{"SSE stream", "text/event-stream", true},
		{"SSE with charset", "text/event-stream; charset=utf-8", true},
		{"JSON response", "application/json", false},
		{"HTML response", "text/html", false},
		{"Empty content type", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := strings.Contains(tc.contentType, "text/event-stream")
			if result != tc.isSSE {
				t.Errorf("expected isSSE=%v for content type '%s', got %v", tc.isSSE, tc.contentType, result)
			}
		})
	}
}

func TestResolveAllowedOrigin(t *testing.T) {
	if origin, ok := matchAllowedOrigin("https://example.com", []string{"https://example.com", "http://localhost:3000"}); !ok || origin != "https://example.com" {
		t.Fatalf("expected exact origin to be allowed, got origin=%q ok=%v", origin, ok)
	}
	if origin, ok := matchAllowedOrigin("https://example.com", []string{"*"}); !ok || origin != "*" {
		t.Fatalf("expected wildcard origin to be allowed, got origin=%q ok=%v", origin, ok)
	}
	if _, ok := matchAllowedOrigin("https://evil.example", []string{"https://example.com"}); ok {
		t.Fatal("expected unknown origin to be rejected")
	}
}

func TestNormalizeCORSOriginsDropsUnresolvedEnvPlaceholder(t *testing.T) {
	got := normalizeCORSOrigins([]string{"${CORS_ALLOWED_ORIGIN}", "https://example.com"})
	if len(got) != 1 || got[0] != "https://example.com" {
		t.Fatalf("expected unresolved placeholder to be dropped, got %#v", got)
	}
}

func TestIsSameOriginRequest(t *testing.T) {
	r := &ghttp.Request{}
	r.Request = &http.Request{Host: "example.com"}
	r.Header = http.Header{}
	r.Header.Set("X-Forwarded-Proto", "https")
	if !isSameOriginRequest(r, "https://example.com") {
		t.Fatal("expected same origin request to be allowed")
	}
}

func TestAuthorizePathAccess(t *testing.T) {
	if !authorizePathAccess("/api/chat", auth.RoleViewer) {
		t.Fatal("viewer should access chat")
	}
	if authorizePathAccess("/api/ai_ops", auth.RoleViewer) {
		t.Fatal("viewer should not access ai_ops")
	}
	if !authorizePathAccess("/api/approval_requests/approve", auth.RoleAdmin) {
		t.Fatal("admin should access approval actions")
	}
	if authorizePathAccess("/api/approval_requests/approve", auth.RoleOperator) {
		t.Fatal("operator should not access approval actions")
	}
}

func TestAllowAnonymousPath(t *testing.T) {
	if !allowAnonymousPath("/api/chat") {
		t.Fatal("chat should allow anonymous access")
	}
	if allowAnonymousPath("/api/ai_ops") {
		t.Fatal("ai_ops should require authentication")
	}
}
