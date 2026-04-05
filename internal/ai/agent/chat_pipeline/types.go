package chat_pipeline

import "github.com/cloudwego/eino/schema"

type UserMessage struct {
	ID        string            `json:"id"`
	Query     string            `json:"query"`
	Documents string            `json:"documents"`
	History   []*schema.Message `json:"history"`
}
