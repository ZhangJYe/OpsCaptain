package v1

import "github.com/gogf/gf/v2/frame/g"

// === 通知配置 API ===

// NotificationConfigReq 获取通知配置。
type NotificationConfigReq struct {
	g.Meta `path:"/notifications/config" method:"get" tags:"Notifications" summary:"获取通知配置"`
}

type NotificationConfigRes struct {
	Feishu *FeishuNotificationConfig `json:"feishu"`
}

// FeishuNotificationConfig 飞书通知配置。
type FeishuNotificationConfig struct {
	Enabled      bool     `json:"enabled"`
	WebhookURL   string   `json:"webhook_url"`
	MinRiskLevel string   `json:"min_risk_level"`
	Services     []string `json:"services"`
	TimeoutMs    int      `json:"timeout_ms"`
}

// NotificationTestReq 测试通知连接。
type NotificationTestReq struct {
	g.Meta     `path:"/notifications/test" method:"post" tags:"Notifications" summary:"测试通知连接"`
	Channel    string `json:"channel" v:"required"` // feishu
	WebhookURL string `json:"webhook_url"`          // 可选，覆盖配置中的 URL
}

type NotificationTestRes struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
