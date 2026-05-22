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
	oldKnowledge := knowledgeReadyCheck
	defer func() {
		redisReadyCheck = oldRedis
		milvusReadyCheck = oldMilvus
		rabbitMQReadyCheck = oldRabbitMQ
		knowledgeReadyCheck = oldKnowledge
	}()

	redisReadyCheck = func(context.Context) error { return nil }
	milvusReadyCheck = func(context.Context) error { return errCheckSkipped }
	rabbitMQReadyCheck = func(context.Context) error { return errCheckSkipped }
	schemaOK := true
	docCount := int64(3)
	knowledgeReadyCheck = func(context.Context) CheckStatus {
		return CheckStatus{Ready: true, Collection: "opscaption_knowledge_v2", SchemaOK: &schemaOK, DocCount: &docCount}
	}

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
	if report.Checks["knowledge"].Collection != "opscaption_knowledge_v2" {
		t.Fatalf("expected knowledge collection details: %#v", report.Checks["knowledge"])
	}
	if report.Checks["knowledge"].SchemaOK == nil || !*report.Checks["knowledge"].SchemaOK {
		t.Fatalf("expected knowledge schema_ok=true: %#v", report.Checks["knowledge"])
	}
	if report.Checks["knowledge"].DocCount == nil || *report.Checks["knowledge"].DocCount != 3 {
		t.Fatalf("expected knowledge doc_count=3: %#v", report.Checks["knowledge"])
	}
}

func TestBuildReadinessReportFailedDependency(t *testing.T) {
	oldRedis := redisReadyCheck
	oldMilvus := milvusReadyCheck
	oldRabbitMQ := rabbitMQReadyCheck
	oldKnowledge := knowledgeReadyCheck
	defer func() {
		redisReadyCheck = oldRedis
		milvusReadyCheck = oldMilvus
		rabbitMQReadyCheck = oldRabbitMQ
		knowledgeReadyCheck = oldKnowledge
	}()

	redisReadyCheck = func(context.Context) error { return errors.New("redis down") }
	milvusReadyCheck = func(context.Context) error { return nil }
	rabbitMQReadyCheck = func(context.Context) error { return nil }
	knowledgeReadyCheck = func(context.Context) CheckStatus { return CheckStatus{Ready: true} }

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
	oldKnowledge := knowledgeReadyCheck
	defer func() {
		redisReadyCheck = oldRedis
		milvusReadyCheck = oldMilvus
		rabbitMQReadyCheck = oldRabbitMQ
		knowledgeReadyCheck = oldKnowledge
	}()

	redisReadyCheck = func(context.Context) error { return nil }
	milvusReadyCheck = func(context.Context) error { return nil }
	rabbitMQReadyCheck = func(context.Context) error { return nil }
	knowledgeReadyCheck = func(context.Context) CheckStatus { return CheckStatus{Ready: true} }

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
