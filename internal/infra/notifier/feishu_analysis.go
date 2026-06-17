package notifier

import (
	"SuperBizAgent/internal/ai/changeevent"
	"SuperBizAgent/internal/ai/protocol"
	aiservice "SuperBizAgent/internal/ai/service"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// AIOpsResultGetter 获取 AIOps 分析结果的接口。
type AIOpsResultGetter func(ctx context.Context, traceID string) (*aiservice.ExecutionResponse, error)

// FeishuAnalysisNotifier 在 AIOps 分析完成后，向飞书群发送分析结论跟进卡片。
// 实现 ChangeEventHandler 接口，注册到 ChangeEventBus 后自动工作。
type FeishuAnalysisNotifier struct {
	tracker      *changeevent.AnalysisTracker
	getter       AIOpsResultGetter
	webhookURL   string
	baseURL      string
	pollInterval time.Duration
	maxWait      time.Duration
	client       *http.Client
}

// FeishuAnalysisNotifierConfig 配置。
type FeishuAnalysisNotifierConfig struct {
	Tracker      *changeevent.AnalysisTracker
	Getter       AIOpsResultGetter
	WebhookURL   string
	BaseURL      string
	PollInterval time.Duration // 轮询间隔，默认 10s
	MaxWait      time.Duration // 最长等待时间，默认 3min
	TimeoutMs    int           // HTTP 超时毫秒
}

// NewFeishuAnalysisNotifier 创建分析结论跟进通知器。
func NewFeishuAnalysisNotifier(cfg FeishuAnalysisNotifierConfig) *FeishuAnalysisNotifier {
	pollInterval := cfg.PollInterval
	if pollInterval <= 0 {
		pollInterval = 10 * time.Second
	}
	maxWait := cfg.MaxWait
	if maxWait <= 0 {
		maxWait = 3 * time.Minute
	}
	timeoutMs := cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return &FeishuAnalysisNotifier{
		tracker:      cfg.Tracker,
		getter:       cfg.Getter,
		webhookURL:   strings.TrimSpace(cfg.WebhookURL),
		baseURL:      strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		pollInterval: pollInterval,
		maxWait:      maxWait,
		client:       &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond},
	}
}

// Name 实现 ChangeEventHandler 接口。
func (n *FeishuAnalysisNotifier) Name() string {
	return "feishu_analysis_notifier"
}

// Handle 收到变更事件后，启动后台 goroutine 轮询分析结果并发送跟进卡片。
func (n *FeishuAnalysisNotifier) Handle(ctx context.Context, event *protocol.ChangeEvent) error {
	if n == nil || n.tracker == nil || n.getter == nil || n.webhookURL == "" {
		return nil
	}

	// 等一会儿让 ProactiveAnalyzer 有机会存入 trace_id
	time.AfterFunc(3*time.Second, func() {
		n.pollAndSend(event)
	})
	return nil
}

// pollAndSend 轮询分析结果，完成后发送跟进卡片。
func (n *FeishuAnalysisNotifier) pollAndSend(event *protocol.ChangeEvent) {
	ctx := context.Background()
	record, ok := n.tracker.Load(event.EventID)
	if !ok {
		g.Log().Debugf(ctx, "[feishu_analysis] no analysis record for event %s, skipping", event.EventID)
		return
	}

	g.Log().Infof(ctx, "[feishu_analysis] polling analysis result: trace_id=%s service=%s", record.TraceID, event.Service)

	deadline := time.Now().Add(n.maxWait)
	for time.Now().Before(deadline) {
		result, err := n.getter(ctx, record.TraceID)
		if err != nil {
			time.Sleep(n.pollInterval)
			continue
		}
		if result == nil {
			time.Sleep(n.pollInterval)
			continue
		}

		// 检查是否完成
		status := strings.ToLower(string(result.Status))
		if status == "succeeded" || status == "failed" || status == "degraded" {
			n.sendFollowUp(event, record, result, status == "failed")
			n.tracker.Delete(event.EventID)
			return
		}
		time.Sleep(n.pollInterval)
	}

	// 超时
	g.Log().Warningf(ctx, "[feishu_analysis] timeout waiting for analysis: trace_id=%s", record.TraceID)
	n.sendTimeoutCard(event, record)
	n.tracker.Delete(event.EventID)
}

// sendFollowUp 发送分析结论跟进卡片。
func (n *FeishuAnalysisNotifier) sendFollowUp(event *protocol.ChangeEvent, record *changeevent.AnalysisRecord, result *aiservice.ExecutionResponse, isError bool) {
	card := n.buildAnalysisCard(event, record, result, isError)
	payload, _ := json.Marshal(card)

	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		g.Log().Warningf(context.Background(), "[feishu_analysis] send failed: %v", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		g.Log().Warningf(context.Background(), "[feishu_analysis] webhook returned %d: %s", resp.StatusCode, string(body))
		return
	}
	var resultResp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &resultResp); err == nil && resultResp.Code != 0 {
		g.Log().Warningf(context.Background(), "[feishu_analysis] feishu API error %d", resultResp.Code)
		return
	}
	g.Log().Infof(context.Background(), "[feishu_analysis] sent follow-up: service=%s trace_id=%s", event.Service, record.TraceID)
}

// sendTimeoutCard 发送分析超时卡片。
func (n *FeishuAnalysisNotifier) sendTimeoutCard(event *protocol.ChangeEvent, record *changeevent.AnalysisRecord) {
	card := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title":    map[string]any{"tag": "plain_text", "content": fmt.Sprintf("⏰ 分析超时 · %s", event.Service)},
				"template": "orange",
			},
			"elements": []any{
				map[string]any{
					"tag": "div",
					"text": map[string]any{
						"tag":     "lark_md",
						"content": fmt.Sprintf("AIOps 分析在 %s 内未完成，可能正在处理复杂查询。\n\n**Trace ID** `%s`", n.maxWait.Round(time.Second), record.TraceID),
					},
				},
				map[string]any{
					"tag": "action",
					"actions": []any{
						map[string]any{
							"tag":  "button",
							"text": map[string]any{"tag": "plain_text", "content": "查看分析状态"},
							"url":  fmt.Sprintf("%s/?trace_id=%s", n.baseURL, record.TraceID),
							"type": "default",
						},
					},
				},
			},
		},
	}

	payload, _ := json.Marshal(card)
	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

// buildAnalysisCard 构建分析结论跟进卡片。
func (n *FeishuAnalysisNotifier) buildAnalysisCard(event *protocol.ChangeEvent, record *changeevent.AnalysisRecord, result *aiservice.ExecutionResponse, isError bool) map[string]any {
	color := "green"
	title := "✅ 分析结论"
	if isError {
		color = "red"
		title = "❌ 分析异常"
	} else if result.Degraded() {
		color = "orange"
		title = "⚠️ 分析降级"
	}

	elements := []any{}

	// 关联信息
	elements = append(elements, map[string]any{
		"tag": "div",
		"text": map[string]any{
			"tag":     "lark_md",
			"content": fmt.Sprintf("关联变更：**%s** %s · %s", event.EventType, event.Service, event.Env),
		},
	})

	// 分析结论
	content := result.Content
	if content == "" && len(result.Detail) > 0 {
		content = result.Detail[0]
	}
	if content != "" {
		// 截断过长的结论
		if len(content) > 1500 {
			content = content[:1500] + "\n\n... (已截断)"
		}
		elements = append(elements, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":     "lark_md",
				"content": content,
			},
		})
	}

	// 置信度和证据
	if result.Confidence > 0 {
		confidenceStr := fmt.Sprintf("%.0f%%", result.Confidence*100)
		elements = append(elements, map[string]any{
			"tag": "div",
			"fields": []any{
				map[string]any{
					"is_short": true,
					"text":     map[string]any{"tag": "lark_md", "content": fmt.Sprintf("**置信度**\n%s", confidenceStr)},
				},
				map[string]any{
					"is_short": true,
					"text":     map[string]any{"tag": "lark_md", "content": fmt.Sprintf("**引擎**\n%s", result.Engine)},
				},
			},
		})
	}

	// 建议操作
	if len(result.NextActions) > 0 {
		elements = append(elements, map[string]any{"tag": "hr"})
		var actions []string
		for i, action := range result.NextActions {
			if i >= 3 {
				break
			}
			actions = append(actions, fmt.Sprintf("%d. %s", i+1, action))
		}
		elements = append(elements, map[string]any{
			"tag": "div",
			"text": map[string]any{
				"tag":     "lark_md",
				"content": fmt.Sprintf("**建议操作**\n%s", strings.Join(actions, "\n")),
			},
		})
	}

	// 操作按钮
	if n.baseURL != "" {
		elements = append(elements, map[string]any{
			"tag": "action",
			"actions": []any{
				map[string]any{
					"tag":  "button",
					"text": map[string]any{"tag": "plain_text", "content": "查看详情"},
					"url":  fmt.Sprintf("%s/?trace_id=%s", n.baseURL, record.TraceID),
					"type": "primary",
				},
			},
		})
	}

	// 底部备注
	elements = append(elements, map[string]any{
		"tag": "note",
		"elements": []any{
			map[string]any{"tag": "plain_text", "content": record.StartedAt.Format("2006-01-02 15:04:05")},
		},
	})

	return map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title":    map[string]any{"tag": "plain_text", "content": title},
				"template": color,
			},
			"elements": elements,
		},
	}
}
