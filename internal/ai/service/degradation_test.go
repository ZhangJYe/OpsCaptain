package service

import (
	"context"
	"testing"
)

func TestGetDegradationDecisionUsesConfigKillSwitch(t *testing.T) {
	oldBool := degradationConfigBool
	oldString := degradationConfigString
	oldRedisGet := degradationRedisGet
	defer func() {
		degradationConfigBool = oldBool
		degradationConfigString = oldString
		degradationRedisGet = oldRedisGet
	}()

	degradationConfigBool = func(context.Context, string) bool { return true }
	degradationConfigString = func(_ context.Context, key string) string {
		switch key {
		case "degradation.message":
			return "degraded by config"
		default:
			return ""
		}
	}
	degradationRedisGet = func(context.Context, string) (string, error) {
		t.Fatal("redis should not be consulted when config kill switch is enabled")
		return "", nil
	}

	decision := GetDegradationDecision(context.Background(), "chat")
	if !decision.Enabled {
		t.Fatal("expected config kill switch to enable degradation")
	}
	if decision.Source != "config" {
		t.Fatalf("unexpected source: %q", decision.Source)
	}
	if decision.Message != "degraded by config" {
		t.Fatalf("unexpected message: %q", decision.Message)
	}
}

func TestGetDegradationDecisionUsesRedisKillSwitch(t *testing.T) {
	oldBool := degradationConfigBool
	oldString := degradationConfigString
	oldRedisGet := degradationRedisGet
	defer func() {
		degradationConfigBool = oldBool
		degradationConfigString = oldString
		degradationRedisGet = oldRedisGet
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(_ context.Context, key string) string {
		switch key {
		case "redis.default.address":
			return "127.0.0.1:6379"
		case "degradation.redis_key":
			return "custom:key"
		case "degradation.message":
			return "redis degraded"
		default:
			return ""
		}
	}
	degradationRedisGet = func(context.Context, string) (string, error) {
		return "true", nil
	}

	decision := GetDegradationDecision(context.Background(), "ai_ops")
	if !decision.Enabled {
		t.Fatal("expected redis kill switch to enable degradation")
	}
	if decision.Source != "redis" {
		t.Fatalf("unexpected source: %q", decision.Source)
	}
	if decision.Message != "redis degraded" {
		t.Fatalf("unexpected message: %q", decision.Message)
	}
}
