package changeevent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net/http"
	"testing"
)

func TestGitHubAdapter_WorkflowRun(t *testing.T) {
	a := NewGitHubAdapter()
	body := []byte(`{
		"action": "completed",
		"workflow_run": {
			"id": 12345,
			"name": "Deploy to Production",
			"head_branch": "main",
			"head_sha": "abc1234567890",
			"status": "completed",
			"conclusion": "success",
			"created_at": "2026-06-10T14:00:00Z",
			"run_started_at": "2026-06-10T14:00:00Z",
			"repository": {"full_name": "org/user-service"},
			"head_commit": {"message": "feat: add new endpoint"}
		}
	}`)

	headers := http.Header{}
	headers.Set("X-GitHub-Event", "workflow_run")

	event, err := a.Parse(context.Background(), headers, body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if event.Service != "user-service" {
		t.Fatalf("expected service=user-service, got %s", event.Service)
	}
	if event.EventType != "deploy" {
		t.Fatalf("expected event_type=deploy, got %s", event.EventType)
	}
	if event.Source != "github" {
		t.Fatalf("expected source=github, got %s", event.Source)
	}
	if event.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
}

func TestGitHubAdapter_Push(t *testing.T) {
	a := NewGitHubAdapter()
	body := []byte(`{
		"ref": "refs/heads/main",
		"before": "1111111111111111111111111111111111111111",
		"after": "2222222222222222222222222222222222222222",
		"repository": {"full_name": "org/order-service"},
		"head_commit": {
			"message": "fix: resolve timeout issue",
			"author": {"name": "zhangsan"}
		}
	}`)

	headers := http.Header{}
	headers.Set("X-GitHub-Event", "push")

	event, err := a.Parse(context.Background(), headers, body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if event.Service != "order-service" {
		t.Fatalf("expected service=order-service, got %s", event.Service)
	}
	if event.Operator != "zhangsan" {
		t.Fatalf("expected operator=zhangsan, got %s", event.Operator)
	}
	if event.EventType != "git_push" {
		t.Fatalf("expected event_type=git_push, got %s", event.EventType)
	}
	if event.RiskLevel != "low" {
		t.Fatalf("expected risk_level=low, got %s", event.RiskLevel)
	}
}

func TestGitHubAdapter_PushNonMainIgnored(t *testing.T) {
	a := NewGitHubAdapter()
	body := []byte(`{
		"ref": "refs/heads/feature-branch",
		"before": "1111111111111111111111111111111111111111",
		"after": "2222222222222222222222222222222222222222",
		"repository": {"full_name": "org/order-service"},
		"head_commit": {"message": "wip"}
	}`)

	headers := http.Header{}
	headers.Set("X-GitHub-Event", "push")

	_, err := a.Parse(context.Background(), headers, body)
	if err == nil {
		t.Fatal("expected error for non-main branch push")
	}
}

func TestGitLabAdapter_Pipeline(t *testing.T) {
	a := NewGitLabAdapter()
	body := []byte(`{
		"object_kind": "pipeline",
		"object_attributes": {
			"id": 9999,
			"ref": "main",
			"status": "success",
			"created_at": "2026-06-10T14:00:00Z",
			"finished_at": "2026-06-10T14:05:00Z",
			"duration": 300
		},
		"project": {
			"path_with_namespace": "org/payment-service",
			"name": "payment-service"
		},
		"commit": {
			"message": "chore: update dependencies",
			"id": "abc12345",
			"author": {"name": "lisi"}
		}
	}`)

	headers := http.Header{}
	headers.Set("X-Gitlab-Event", "Pipeline Hook")

	event, err := a.Parse(context.Background(), headers, body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if event.Service != "payment-service" {
		t.Fatalf("expected service=payment-service, got %s", event.Service)
	}
	if event.Source != "gitlab" {
		t.Fatalf("expected source=gitlab, got %s", event.Source)
	}
	if event.Operator != "lisi" {
		t.Fatalf("expected operator=lisi, got %s", event.Operator)
	}
	if event.EventType != "pipeline" {
		t.Fatalf("expected event_type=pipeline, got %s", event.EventType)
	}
}

func TestGitHubAdapter_RequiresValidSignatureWhenSecretConfigured(t *testing.T) {
	a := NewGitHubAdapter("top-secret")
	body := []byte(`{"ref":"refs/heads/main","before":"11111111","after":"22222222","repository":{"full_name":"org/order-service"},"head_commit":{"message":"fix","author":{"name":"zhangsan"}}}`)

	headers := http.Header{}
	headers.Set("X-GitHub-Event", "push")
	headers.Set("X-Hub-Signature-256", githubSignature("wrong-secret", body))
	if _, err := a.Parse(context.Background(), headers, body); err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}

	headers.Set("X-Hub-Signature-256", githubSignature("top-secret", body))
	if _, err := a.Parse(context.Background(), headers, body); err != nil {
		t.Fatalf("expected valid signature to pass, got %v", err)
	}
}

func TestGitLabAdapter_RequiresValidTokenWhenSecretConfigured(t *testing.T) {
	a := NewGitLabAdapter("gitlab-secret")
	body := []byte(`{"object_kind":"push","ref":"refs/heads/main","before":"11111111","after":"22222222","project":{"path_with_namespace":"org/order-service","name":"order-service"},"commits":[{"message":"fix","author":{"name":"lisi"}}]}`)
	headers := http.Header{}
	headers.Set("X-Gitlab-Event", "Push Hook")
	headers.Set("X-Gitlab-Token", "bad")
	if _, err := a.Parse(context.Background(), headers, body); err == nil {
		t.Fatal("expected invalid token to be rejected")
	}

	headers.Set("X-Gitlab-Token", "gitlab-secret")
	if _, err := a.Parse(context.Background(), headers, body); err != nil {
		t.Fatalf("expected valid token to pass, got %v", err)
	}
}

func TestArgoCDAdapter_SyncSucceeded(t *testing.T) {
	a := NewArgoCDAdapter()
	body := []byte(`{
		"spec": {
			"source": {
				"repoURL": "https://github.com/org/gitops",
				"path": "apps/user-service",
				"targetRevision": "main"
			},
			"destination": {
				"server": "https://kubernetes.default.svc",
				"namespace": "production"
			}
		},
		"status": {
			"phase": "Succeeded",
			"health": {"status": "Healthy"},
			"sync": {
				"status": "Synced",
				"revision": "abc1234567890def",
				"comparedTo": {
					"source": {"targetRevision": "main"}
				}
			},
			"operationState": {
				"phase": "Succeeded",
				"message": "successfully synced",
				"finishedAt": "2026-06-10T14:30:00Z"
			},
			"history": [{
				"revision": "abc1234567890def",
				"deployedAt": "2026-06-10T14:30:00Z"
			}]
		},
		"metadata": {
			"name": "user-service",
			"namespace": "argocd"
		}
	}`)

	event, err := a.Parse(context.Background(), nil, body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if event.Service != "user-service" {
		t.Fatalf("expected service=user-service, got %s", event.Service)
	}
	if event.Env != "prod" {
		t.Fatalf("expected env=prod, got %s", event.Env)
	}
	if event.EventType != "deploy" {
		t.Fatalf("expected event_type=deploy, got %s", event.EventType)
	}
	if event.Source != "argocd" {
		t.Fatalf("expected source=argocd, got %s", event.Source)
	}
}

func TestArgoCDAdapter_SyncFailed(t *testing.T) {
	a := NewArgoCDAdapter()
	body := []byte(`{
		"spec": {
			"source": {"repoURL": "https://github.com/org/gitops", "path": "apps/order-service"},
			"destination": {"server": "https://kubernetes.default.svc", "namespace": "staging"}
		},
		"status": {
			"phase": "Failed",
			"health": {"status": "Degraded"},
			"sync": {"status": "OutOfSync"},
			"operationState": {
				"phase": "Failed",
				"message": "sync error: configmap not found"
			}
		},
		"metadata": {"name": "order-service"}
	}`)

	event, err := a.Parse(context.Background(), nil, body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if event.RiskLevel != "high" {
		t.Fatalf("expected risk_level=high for failed sync, got %s", event.RiskLevel)
	}
	if event.EventType != "deploy" {
		t.Fatalf("expected event_type=deploy, got %s", event.EventType)
	}
}

func TestAdapterRegistry_AllRegistered(t *testing.T) {
	r := NewAdapterRegistry()

	for _, name := range []string{"json", "github", "gitlab", "argocd"} {
		adapter, ok := r.Get(name)
		if !ok {
			t.Fatalf("expected adapter %q to be registered", name)
		}
		if adapter.Name() != name {
			t.Fatalf("expected adapter name=%q, got %q", name, adapter.Name())
		}
	}

	// 不存在的适配器
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("expected nonexistent adapter to not be found")
	}
}

func TestJSONAdapter_Parse(t *testing.T) {
	a := NewJSONAdapter()
	body := []byte(`{
		"source": "manual",
		"event_type": "config_update",
		"service": "gateway",
		"env": "prod",
		"summary": "Updated rate limit from 1000 to 500"
	}`)

	event, err := a.Parse(context.Background(), nil, body)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if event.Service != "gateway" {
		t.Fatalf("expected service=gateway, got %s", event.Service)
	}
	if event.EventType != "config_update" {
		t.Fatalf("expected event_type=config_update, got %s", event.EventType)
	}
}

func githubSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return fmt.Sprintf("sha256=%x", mac.Sum(nil))
}
