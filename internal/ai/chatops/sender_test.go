package chatops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSend_Success(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	sender := NewFeishuSender(srv.URL, 5000)
	msg := &Message{
		Title:   "测试通知",
		Content: "这是一条测试消息",
		Level:   "info",
	}

	err := sender.Send(context.Background(), msg)
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
	if card.Card.Header.Template != "blue" {
		t.Errorf("expected blue header for info level, got %s", card.Card.Header.Template)
	}
	if card.Card.Header.Title.Content != "测试通知" {
		t.Errorf("expected title '测试通知', got %s", card.Card.Header.Title.Content)
	}
}

func TestSend_InvalidURL(t *testing.T) {
	sender := NewFeishuSender("http://localhost:1", 1000)
	msg := &Message{
		Title:   "test",
		Content: "test",
		Level:   "error",
	}

	err := sender.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestSend_EmptyContent(t *testing.T) {
	sender := NewFeishuSender("http://localhost", 5000)

	err := sender.Send(context.Background(), &Message{Content: ""})
	if err == nil {
		t.Fatal("expected error for empty content")
	}

	err = sender.Send(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil message")
	}
}

func TestSend_NoWebhookURL(t *testing.T) {
	sender := NewFeishuSender("", 5000)
	msg := &Message{
		Content: "test",
	}

	err := sender.Send(context.Background(), msg)
	if err == nil {
		t.Fatal("expected error for empty webhook URL")
	}
}

func TestSend_LevelColor(t *testing.T) {
	tests := []struct {
		level    string
		expected string
	}{
		{"info", "blue"},
		{"warning", "orange"},
		{"error", "red"},
		{"", "blue"},
		{"unknown", "blue"},
	}

	for _, tt := range tests {
		if got := levelColor(tt.level); got != tt.expected {
			t.Errorf("levelColor(%q) = %q, want %q", tt.level, got, tt.expected)
		}
	}
}

func TestSend_WarningLevel(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	sender := NewFeishuSender(srv.URL, 5000)
	msg := &Message{
		Title:   "警告",
		Content: "CPU 使用率过高",
		Level:   "warning",
	}

	err := sender.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var card feishuCard
	if err := json.Unmarshal(received, &card); err != nil {
		t.Fatalf("failed to unmarshal card: %v", err)
	}
	if card.Card.Header.Template != "orange" {
		t.Errorf("expected orange header for warning level, got %s", card.Card.Header.Template)
	}
}

func TestSend_DefaultTitle(t *testing.T) {
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = body
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer srv.Close()

	sender := NewFeishuSender(srv.URL, 5000)
	msg := &Message{
		Content: "no title message",
		Level:   "info",
	}

	err := sender.Send(context.Background(), msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var card feishuCard
	if err := json.Unmarshal(received, &card); err != nil {
		t.Fatalf("failed to unmarshal card: %v", err)
	}
	if card.Card.Header.Title.Content != "OpsCaption 通知" {
		t.Errorf("expected default title 'OpsCaption 通知', got %s", card.Card.Header.Title.Content)
	}
}
