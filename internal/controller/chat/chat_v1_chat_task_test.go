package chat

import (
	"context"
	"strings"
	"testing"
	"time"

	v1 "SuperBizAgent/api/chat/v1"
	aiservice "SuperBizAgent/internal/ai/service"
)

func TestChatSubmitSuccess(t *testing.T) {
	oldSubmit := submitChatTask
	defer func() {
		submitChatTask = oldSubmit
	}()

	submitChatTask = func(context.Context, string, string) (*aiservice.ChatTaskRecord, error) {
		return &aiservice.ChatTaskRecord{
			ID:        "task-1",
			Status:    aiservice.ChatTaskStatusQueued,
			CreatedAt: time.Now().Unix(),
		}, nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.ChatSubmit(context.Background(), &v1.ChatSubmitReq{
		Id:       "session_1234567890abcdef",
		Question: "hello",
	})
	if err != nil {
		t.Fatalf("chat submit returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected response")
	}
	if res.TaskID != "task-1" {
		t.Fatalf("unexpected task id: %s", res.TaskID)
	}
	if res.Status != string(aiservice.ChatTaskStatusQueued) {
		t.Fatalf("unexpected status: %s", res.Status)
	}
}

func TestChatTaskSuccess(t *testing.T) {
	oldGet := getChatTask
	defer func() {
		getChatTask = oldGet
	}()

	getChatTask = func(context.Context, string) (*aiservice.ChatTaskRecord, error) {
		return &aiservice.ChatTaskRecord{
			ID:        "task-2",
			SessionID: "session_1234567890abcdef",
			Query:     "hello",
			Status:    aiservice.ChatTaskStatusSucceeded,
			Answer:    "world",
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}, nil
	}

	ctrl := &ControllerV1{}
	res, err := ctrl.ChatTask(context.Background(), &v1.ChatTaskReq{
		TaskID:  "task-2",
		Session: "session_1234567890abcdef",
	})
	if err != nil {
		t.Fatalf("chat task returned error: %v", err)
	}
	if res == nil {
		t.Fatal("expected response")
	}
	if res.Answer != "world" {
		t.Fatalf("unexpected answer: %s", res.Answer)
	}
	if res.Status != string(aiservice.ChatTaskStatusSucceeded) {
		t.Fatalf("unexpected status: %s", res.Status)
	}
}

func TestChatTaskSessionMismatch(t *testing.T) {
	oldGet := getChatTask
	defer func() {
		getChatTask = oldGet
	}()

	getChatTask = func(context.Context, string) (*aiservice.ChatTaskRecord, error) {
		return &aiservice.ChatTaskRecord{
			ID:        "task-3",
			SessionID: "session_a",
			Query:     "hello",
			Status:    aiservice.ChatTaskStatusQueued,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
		}, nil
	}

	ctrl := &ControllerV1{}
	_, err := ctrl.ChatTask(context.Background(), &v1.ChatTaskReq{
		TaskID:  "task-3",
		Session: "session_b",
	})
	if err == nil {
		t.Fatal("expected session mismatch error")
	}
	if !strings.Contains(err.Error(), "does not belong to session") {
		t.Fatalf("unexpected error: %v", err)
	}
}
