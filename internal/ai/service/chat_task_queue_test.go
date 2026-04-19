package service

import "testing"

func TestDecodeChatTaskEventValidation(t *testing.T) {
	event, err := decodeChatTaskEvent([]byte(`{"task_id":"t1","session_id":"s1","query":"hello","requested_at":1,"attempt":0}`))
	if err != nil {
		t.Fatalf("decode should succeed: %v", err)
	}
	if event.TaskID != "t1" {
		t.Fatalf("unexpected task id: %s", event.TaskID)
	}

	if _, err := decodeChatTaskEvent([]byte(`{"task_id":"","session_id":"s1","query":"hello"}`)); err == nil {
		t.Fatal("expected validation error for empty task_id")
	}
}

func TestValidateRabbitMQChatTaskConfig(t *testing.T) {
	if err := validateRabbitMQChatTaskConfig(rabbitMQChatTaskConfig{Enabled: false}); err != nil {
		t.Fatalf("disabled config should pass validation, got %v", err)
	}

	oldRedisConfigured := chatTaskRedisConfigured
	defer func() {
		chatTaskRedisConfigured = oldRedisConfigured
	}()
	chatTaskRedisConfigured = func() bool { return true }

	if err := validateRabbitMQChatTaskConfig(rabbitMQChatTaskConfig{
		Enabled:        true,
		URL:            "amqp://guest:guest@127.0.0.1:5672/",
		Prefetch:       4,
		MaxRetries:     3,
		RetryDelay:     2,
		PublishTimeout: 2,
		ReconnectDelay: 2,
		TaskTTL:        10,
	}); err != nil {
		t.Fatalf("valid config should pass, got %v", err)
	}
}
