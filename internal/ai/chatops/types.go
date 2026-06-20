package chatops

import (
	"net/http"
	"time"
)

// Message 是 ChatOps 推送的消息。
type Message struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	Level   string `json:"level"` // info, warning, error
}

// FeishuSender 通过飞书 Webhook 发送消息到群聊。
type FeishuSender struct {
	webhookURL string
	client     *http.Client
}

// NewFeishuSender 创建飞书消息发送器。
func NewFeishuSender(webhookURL string, timeoutMs int) *FeishuSender {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return &FeishuSender{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		},
	}
}


