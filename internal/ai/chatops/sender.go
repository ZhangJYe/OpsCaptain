package chatops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
)

// levelColor 根据消息等级返回飞书卡片主题色。
func levelColor(level string) string {
	switch strings.ToLower(level) {
	case "warning":
		return "orange"
	case "error":
		return "red"
	default:
		return "blue"
	}
}

// feishuCard 是飞书交互式卡片的消息格式。
type feishuCard struct {
	MsgType string      `json:"msg_type"`
	Card    feishuInner `json:"card"`
}

type feishuInner struct {
	Header   feishuHeader    `json:"header"`
	Elements []feishuElement `json:"elements"`
}

type feishuHeader struct {
	Title    feishuText `json:"title"`
	Template string     `json:"template"`
}

type feishuText struct {
	Tag     string `json:"tag"`
	Content string `json:"content"`
}

type feishuElement struct {
	Tag  string     `json:"tag"`
	Text *feishuText `json:"text,omitempty"`
}

// feishuResponse 是飞书 Webhook 响应格式。
type feishuResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// Send 发送消息到飞书群 Webhook。
func (s *FeishuSender) Send(ctx context.Context, msg *Message) error {
	if s == nil || s.webhookURL == "" {
		return fmt.Errorf("feishu chatops webhook URL is not configured")
	}
	if msg == nil {
		return fmt.Errorf("message is nil")
	}
	if strings.TrimSpace(msg.Content) == "" {
		return fmt.Errorf("message content is empty")
	}

	title := strings.TrimSpace(msg.Title)
	if title == "" {
		title = "OpsCaption 通知"
	}

	elements := []feishuElement{
		{
			Tag:  "div",
			Text: &feishuText{Tag: "lark_md", Content: msg.Content},
		},
	}

	card := feishuCard{
		MsgType: "interactive",
		Card: feishuInner{
			Header: feishuHeader{
				Title:    feishuText{Tag: "plain_text", Content: title},
				Template: levelColor(msg.Level),
			},
			Elements: elements,
		},
	}

	payload, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshal feishu card: %w", err)
	}

	resp, err := s.client.Post(s.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("send feishu webhook: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu webhook returned %d: %s", resp.StatusCode, string(body))
	}

	var result feishuResponse
	if err := json.Unmarshal(body, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("feishu API error %d: %s", result.Code, result.Msg)
	}

	g.Log().Infof(ctx, "[chatops] sent feishu message: title=%s level=%s", title, msg.Level)
	return nil
}
