package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

func saveChatTaskRecord(ctx context.Context, cfg rabbitMQChatTaskConfig, record *ChatTaskRecord) error {
	if record == nil {
		return fmt.Errorf("chat task record is nil")
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	ttl := int(cfg.TaskTTL.Seconds())
	if ttl <= 0 {
		ttl = int(defaultChatTaskTTL.Seconds())
	}
	_, err = g.Redis().Do(ctx, "SETEX", chatTaskRecordKey(cfg.RedisKeyPrefix, record.ID), ttl, string(payload))
	return err
}

func loadChatTaskRecord(ctx context.Context, cfg rabbitMQChatTaskConfig, taskID string) (*ChatTaskRecord, error) {
	key := chatTaskRecordKey(cfg.RedisKeyPrefix, taskID)
	val, err := g.Redis().Do(ctx, "GET", key)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(val.String())
	if raw == "" {
		return nil, fmt.Errorf("chat task %s not found", strings.TrimSpace(taskID))
	}
	var record ChatTaskRecord
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func chatTaskRecordKey(prefix, taskID string) string {
	p := strings.TrimSpace(prefix)
	if p == "" {
		p = defaultChatTaskKeyPrefix
	}
	return fmt.Sprintf("%s:task:%s", p, strings.TrimSpace(taskID))
}
