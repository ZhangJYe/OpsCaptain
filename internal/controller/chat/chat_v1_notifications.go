package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/infra/notifier"
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// NotificationConfig 获取通知配置。
func (c *ControllerV1) NotificationConfig(ctx context.Context, req *v1.NotificationConfigReq) (res *v1.NotificationConfigRes, err error) {
	enabled := g.Cfg().MustGet(ctx, "change_events.notifier.feishu.enabled", false).Bool()
	webhookURL := g.Cfg().MustGet(ctx, "change_events.notifier.feishu.webhook_url", "").String()
	minRiskLevel := g.Cfg().MustGet(ctx, "change_events.notifier.feishu.min_risk_level", "medium").String()
	timeoutMs := g.Cfg().MustGet(ctx, "change_events.notifier.feishu.timeout_ms", 5000).Int()

	var services []string
	if v := g.Cfg().MustGet(ctx, "change_events.notifier.feishu.services", nil); !v.IsNil() && !v.IsEmpty() {
		services = v.Strings()
	}

	return &v1.NotificationConfigRes{
		Feishu: &v1.FeishuNotificationConfig{
			Enabled:      enabled,
			WebhookURL:   webhookURL,
			MinRiskLevel: minRiskLevel,
			Services:     services,
			TimeoutMs:    timeoutMs,
		},
	}, nil
}

// NotificationTest 测试通知连接。
func (c *ControllerV1) NotificationTest(ctx context.Context, req *v1.NotificationTestReq) (res *v1.NotificationTestRes, err error) {
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	if channel != "feishu" {
		return &v1.NotificationTestRes{
			Success: false,
			Message: "unsupported channel: " + req.Channel,
		}, nil
	}

	webhookURL := strings.TrimSpace(req.WebhookURL)
	if webhookURL == "" {
		webhookURL = g.Cfg().MustGet(ctx, "change_events.notifier.feishu.webhook_url", "").String()
	}
	if webhookURL == "" {
		return &v1.NotificationTestRes{
			Success: false,
			Message: "webhook URL is not configured",
		}, nil
	}

	n := notifier.NewFeishuNotifier(notifier.FeishuNotifierConfig{
		WebhookURL: webhookURL,
		TimeoutMs:  5000,
	})

	testEvent := &notifier.TestChangeEvent{
		Service:   "OpsCaption-Test",
		Env:       "test",
		EventType: "deploy",
		RiskLevel: "medium",
		Source:    "manual",
		Operator:  "system",
		Summary:   "这是一条测试通知，用于验证飞书 Webhook 连接是否正常。",
		StartedAt: time.Now(),
	}

	if err := n.SendTestEvent(ctx, testEvent); err != nil {
		return &v1.NotificationTestRes{
			Success: false,
			Message: "发送失败: " + err.Error(),
		}, nil
	}

	return &v1.NotificationTestRes{
		Success: true,
		Message: "测试消息已发送到飞书群，请检查",
	}, nil
}
