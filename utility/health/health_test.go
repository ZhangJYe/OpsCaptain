package health

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestBuildReadinessReportHealthy(t *testing.T) {
	oldRedis := redisReadyCheck
	oldMilvus := milvusReadyCheck
	oldRabbitMQ := rabbitMQReadyCheck
	defer func() {
		redisReadyCheck = oldRedis
		milvusReadyCheck = oldMilvus
		rabbitMQReadyCheck = oldRabbitMQ
	}()

	redisReadyCheck = func(context.Context) error { return nil }
	milvusReadyCheck = func(context.Context) error { return errCheckSkipped }
	rabbitMQReadyCheck = func(context.Context) error { return errCheckSkipped }

	report, status := BuildReadinessReport(context.Background(), false)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if !report.Ready {
		t.Fatalf("expected report to be ready: %#v", report)
	}
	if !report.Checks["redis"].Ready {
		t.Fatalf("expected redis to be ready: %#v", report.Checks["redis"])
	}
	if !report.Checks["milvus"].Skipped {
		t.Fatalf("expected milvus check to be skipped: %#v", report.Checks["milvus"])
	}
	if !report.Checks["rabbitmq"].Skipped {
		t.Fatalf("expected rabbitmq check to be skipped: %#v", report.Checks["rabbitmq"])
	}
}

func TestBuildReadinessReportFailedDependency(t *testing.T) {
	oldRedis := redisReadyCheck
	oldMilvus := milvusReadyCheck
	oldRabbitMQ := rabbitMQReadyCheck
	defer func() {
		redisReadyCheck = oldRedis
		milvusReadyCheck = oldMilvus
		rabbitMQReadyCheck = oldRabbitMQ
	}()

	redisReadyCheck = func(context.Context) error { return errors.New("redis down") }
	milvusReadyCheck = func(context.Context) error { return nil }
	rabbitMQReadyCheck = func(context.Context) error { return nil }

	report, status := BuildReadinessReport(context.Background(), false)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", status)
	}
	if report.Ready {
		t.Fatalf("expected report to be not ready: %#v", report)
	}
	if report.Checks["redis"].Error == "" {
		t.Fatalf("expected redis error details: %#v", report.Checks["redis"])
	}
}

func TestBuildReadinessReportShutdown(t *testing.T) {
	oldRedis := redisReadyCheck
	oldMilvus := milvusReadyCheck
	oldRabbitMQ := rabbitMQReadyCheck
	defer func() {
		redisReadyCheck = oldRedis
		milvusReadyCheck = oldMilvus
		rabbitMQReadyCheck = oldRabbitMQ
	}()

	redisReadyCheck = func(context.Context) error { return nil }
	milvusReadyCheck = func(context.Context) error { return nil }
	rabbitMQReadyCheck = func(context.Context) error { return nil }

	report, status := BuildReadinessReport(context.Background(), true)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", status)
	}
	if report.Ready {
		t.Fatalf("expected report to be not ready during shutdown: %#v", report)
	}
	if report.Checks["server"].Ready {
		t.Fatalf("expected server readiness to be false: %#v", report.Checks["server"])
	}
}
