package notifier

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFeishuNotifier_Handle_Success(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	n := NewFeishuNotifier(FeishuNotifierConfig{
		WebhookURL: srv.URL,
	})

	event := &protocol.ChangeEvent{
		EventID:   "test-001",
		Service:   "payment-service",
		Env:       "prod",
		EventType: protocol.ChangeTypeDeploy,
		RiskLevel: protocol.ChangeRiskHigh,
		Source:    "cicd",
		Operator:  "zhangsan",
		Summary:   "部署 v2.3.1 到生产环境",
		StartedAt: time.Now(),
	}

	err := n.Handle(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var card feishuCard
	if err := json.Unmarshal(received, &card); err != nil {
		t.Fatalf("failed to unmarshal card: %v", err)
	}
	if card.MsgType != "interactive" {
		t.Errorf("expected msg_type=interactive, got %s", card.MsgType)
	}
	if card.Card.Header.Template != "red" {
		t.Errorf("expected red header for high risk, got %s", card.Card.Header.Template)
	}
}

func TestFeishuNotifier_ShouldNotify_RiskFilter(t *testing.T) {
	n := NewFeishuNotifier(FeishuNotifierConfig{
		WebhookURL:   "http://localhost",
		MinRiskLevel: "high",
	})

	lowEvent := &protocol.ChangeEvent{RiskLevel: protocol.ChangeRiskLow, Service: "svc"}
	highEvent := &protocol.ChangeEvent{RiskLevel: protocol.ChangeRiskHigh, Service: "svc"}

	if n.shouldNotify(lowEvent) {
		t.Error("low risk should not pass high filter")
	}
	if !n.shouldNotify(highEvent) {
		t.Error("high risk should pass high filter")
	}
}

func TestFeishuNotifier_ShouldNotify_ServiceFilter(t *testing.T) {
	n := NewFeishuNotifier(FeishuNotifierConfig{
		WebhookURL: "http://localhost",
		Services:   []string{"payment-service", "order-service"},
	})

	matchEvent := &protocol.ChangeEvent{RiskLevel: protocol.ChangeRiskLow, Service: "payment-service"}
	noMatchEvent := &protocol.ChangeEvent{RiskLevel: protocol.ChangeRiskLow, Service: "user-service"}

	if !n.shouldNotify(matchEvent) {
		t.Error("matching service should pass filter")
	}
	if n.shouldNotify(noMatchEvent) {
		t.Error("non-matching service should not pass filter")
	}
}

func TestFeishuNotifier_Handle_WebhookError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":90001,"msg":"invalid webhook url"}`))
	}))
	defer srv.Close()

	n := NewFeishuNotifier(FeishuNotifierConfig{
		WebhookURL: srv.URL,
	})

	event := &protocol.ChangeEvent{
		EventID:   "test-err",
		Service:   "svc",
		Env:       "prod",
		EventType: protocol.ChangeTypeDeploy,
		RiskLevel: protocol.ChangeRiskHigh,
		StartedAt: time.Now(),
	}

	err := n.Handle(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFeishuNotifier_Handle_NoWebhookURL(t *testing.T) {
	n := NewFeishuNotifier(FeishuNotifierConfig{})
	err := n.Handle(context.Background(), &protocol.ChangeEvent{})
	if err == nil {
		t.Fatal("expected error for empty webhook URL")
	}
}

func TestRiskLevelOrder(t *testing.T) {
	if riskLevelOrder(protocol.ChangeRiskLow) >= riskLevelOrder(protocol.ChangeRiskHigh) {
		t.Error("low should be less than high")
	}
	if riskLevelOrder(protocol.ChangeRiskCritical) <= riskLevelOrder(protocol.ChangeRiskHigh) {
		t.Error("critical should be greater than high")
	}
}
