package notifier

import (
	"SuperBizAgent/internal/ai/protocol"
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

// FeishuNotifier 通过飞书 Webhook 机器人推送变更事件通知。
// 实现 changeevent.ChangeEventHandler 接口，注册到 ChangeEventBus 后自动工作。
type FeishuNotifier struct {
	webhookURL       string
	minRiskLevel     string
	services         []string // 空 = 所有服务
	baseURL          string   // OpsCaption 前端地址
	client           *http.Client
	eventTypeNames   map[string]string
	riskLevelColors  map[string]string
	riskLevelEmoji   map[string]string
}

// FeishuNotifierConfig 是飞书通知器的配置。
type FeishuNotifierConfig struct {
	WebhookURL   string
	MinRiskLevel string   // 最低推送风险等级："low" | "medium" | "high" | "critical"
	Services     []string // 只推送这些服务的变更，空 = 全部
	TimeoutMs    int      // HTTP 超时毫秒，默认 5000
	BaseURL      string   // OpsCaption 前端地址，用于生成操作链接
}

// NewFeishuNotifier 创建飞书通知器。
func NewFeishuNotifier(cfg FeishuNotifierConfig) *FeishuNotifier {
	timeoutMs := cfg.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}
	return &FeishuNotifier{
		webhookURL:   strings.TrimSpace(cfg.WebhookURL),
		minRiskLevel: normalizeRiskLevel(cfg.MinRiskLevel),
		services:     cfg.Services,
		baseURL:      strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		client: &http.Client{
			Timeout: time.Duration(timeoutMs) * time.Millisecond,
		},
		eventTypeNames: map[string]string{
			protocol.ChangeTypeDeploy:         "部署",
			protocol.ChangeTypeRollback:       "回滚",
			protocol.ChangeTypeGitPush:        "代码推送",
			protocol.ChangeTypeMerge:          "代码合并",
			protocol.ChangeTypeRelease:        "发版",
			protocol.ChangeTypePipeline:       "流水线",
			protocol.ChangeTypeScale:          "扩缩容",
			protocol.ChangeTypeConfigUpdate:   "配置变更",
			protocol.ChangeTypeRestart:        "重启",
			protocol.ChangeTypeResourceUpdate: "资源更新",
			protocol.ChangeTypeDNSSwitch:      "DNS 切换",
			protocol.ChangeTypeFailover:       "故障转移",
			protocol.ChangeTypeMaintenance:    "维护",
		},
		riskLevelColors: map[string]string{
			protocol.ChangeRiskLow:      "green",
			protocol.ChangeRiskMedium:   "orange",
			protocol.ChangeRiskHigh:     "red",
			protocol.ChangeRiskCritical: "red",
		},
		riskLevelEmoji: map[string]string{
			protocol.ChangeRiskLow:      "🟢",
			protocol.ChangeRiskMedium:   "🟡",
			protocol.ChangeRiskHigh:     "🔴",
			protocol.ChangeRiskCritical: "🚨",
		},
	}
}

// Name 实现 ChangeEventHandler 接口。
func (n *FeishuNotifier) Name() string {
	return "feishu_notifier"
}

// TestChangeEvent 是测试通知用的简化变更事件。
type TestChangeEvent struct {
	Service   string
	Env       string
	EventType string
	RiskLevel string
	Source    string
	Operator  string
	Summary   string
	StartedAt time.Time
}

// SendTestEvent 发送测试通知（绕过 shouldNotify 过滤）。
func (n *FeishuNotifier) SendTestEvent(ctx context.Context, event *TestChangeEvent) error {
	if n == nil || n.webhookURL == "" {
		return fmt.Errorf("feishu webhook URL is not configured")
	}
	pe := &protocol.ChangeEvent{
		Service:   event.Service,
		Env:       event.Env,
		EventType: event.EventType,
		RiskLevel: event.RiskLevel,
		Source:    event.Source,
		Operator:  event.Operator,
		Summary:   event.Summary,
		StartedAt: event.StartedAt,
	}
	card := n.buildCard(pe)
	payload, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshal feishu card: %w", err)
	}
	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("send feishu webhook: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu webhook returned %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("feishu API error %d: %s", result.Code, result.Msg)
	}
	return nil
}

// Handle 将变更事件推送到飞书群。
func (n *FeishuNotifier) Handle(ctx context.Context, event *protocol.ChangeEvent) error {
	if n == nil || n.webhookURL == "" {
		return fmt.Errorf("feishu webhook URL is not configured")
	}
	if !n.shouldNotify(event) {
		return nil
	}

	card := n.buildCard(event)
	payload, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshal feishu card: %w", err)
	}

	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("send feishu webhook: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu webhook returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("feishu API error %d: %s", result.Code, result.Msg)
	}

	g.Log().Infof(ctx, "[feishu_notifier] sent: event_id=%s service=%s type=%s risk=%s",
		event.EventID, event.Service, event.EventType, event.RiskLevel)
	return nil
}

// shouldNotify 判断事件是否满足推送条件。
func (n *FeishuNotifier) shouldNotify(event *protocol.ChangeEvent) bool {
	if !n.meetsMinRisk(event.RiskLevel) {
		return false
	}
	if len(n.services) > 0 {
		matched := false
		for _, svc := range n.services {
			if strings.EqualFold(event.Service, svc) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// meetsMinRisk 检查事件风险等级是否达到最低推送阈值。
func (n *FeishuNotifier) meetsMinRisk(riskLevel string) bool {
	if n.minRiskLevel == "" {
		return true
	}
	return riskLevelOrder(riskLevel) >= riskLevelOrder(n.minRiskLevel)
}

func riskLevelOrder(level string) int {
	switch strings.ToLower(level) {
	case protocol.ChangeRiskLow:
		return 0
	case protocol.ChangeRiskMedium:
		return 1
	case protocol.ChangeRiskHigh:
		return 2
	case protocol.ChangeRiskCritical:
		return 3
	default:
		return 0
	}
}

func normalizeRiskLevel(level string) string {
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case protocol.ChangeRiskLow, protocol.ChangeRiskMedium, protocol.ChangeRiskHigh, protocol.ChangeRiskCritical:
		return level
	default:
		return ""
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
	Tag      string         `json:"tag"`
	Text     *feishuText    `json:"text,omitempty"`
	Fields   []feishuField  `json:"fields,omitempty"`
	Actions  []feishuAction `json:"actions,omitempty"`
}

type feishuField struct {
	IsShort bool       `json:"is_short"`
	Text    feishuText `json:"text"`
}

type feishuAction struct {
	Tag  string     `json:"tag"`
	Text feishuText `json:"text"`
	URL  string     `json:"url,omitempty"`
	Type string     `json:"type,omitempty"`
}

// buildCard 构建飞书交互式卡片消息。
func (n *FeishuNotifier) buildCard(event *protocol.ChangeEvent) *feishuCard {
	emoji := n.riskLevelEmoji[event.RiskLevel]
	color := n.riskLevelColors[event.RiskLevel]
	eventTypeName := n.eventTypeNames[event.EventType]
	if eventTypeName == "" {
		eventTypeName = event.EventType
	}

	title := fmt.Sprintf("%s 变更通知 — %s", emoji, eventTypeName)

	fields := []feishuField{
		{IsShort: true, Text: feishuText{Tag: "lark_md", Content: fmt.Sprintf("**服务**\n%s", event.Service)}},
		{IsShort: true, Text: feishuText{Tag: "lark_md", Content: fmt.Sprintf("**环境**\n%s", event.Env)}},
		{IsShort: true, Text: feishuText{Tag: "lark_md", Content: fmt.Sprintf("**风险等级**\n%s %s", emoji, strings.ToUpper(event.RiskLevel))}},
		{IsShort: true, Text: feishuText{Tag: "lark_md", Content: fmt.Sprintf("**来源**\n%s", event.Source)}},
	}
	if event.Operator != "" {
		fields = append(fields, feishuField{IsShort: true, Text: feishuText{Tag: "lark_md", Content: fmt.Sprintf("**操作人**\n%s", event.Operator)}})
	}
	if event.Cluster != "" {
		fields = append(fields, feishuField{IsShort: true, Text: feishuText{Tag: "lark_md", Content: fmt.Sprintf("**集群**\n%s", event.Cluster)}})
	}
	if event.Namespace != "" {
		fields = append(fields, feishuField{IsShort: true, Text: feishuText{Tag: "lark_md", Content: fmt.Sprintf("**命名空间**\n%s", event.Namespace)}})
	}

	elements := []feishuElement{
		{Tag: "div", Fields: fields},
		{Tag: "hr"},
		{Tag: "div", Text: &feishuText{Tag: "lark_md", Content: fmt.Sprintf("**摘要**\n%s", event.Summary)}},
	}

	// Before/After 状态对比
	if len(event.Before) > 0 || len(event.After) > 0 {
		compareText := buildCompareText(event.Before, event.After)
		if compareText != "" {
			elements = append(elements, feishuElement{
				Tag:  "div",
				Text: &feishuText{Tag: "lark_md", Content: compareText},
			})
		}
	}

	// Diff 详情
	if event.Diff != "" {
		diff := event.Diff
		if len(diff) > 800 {
			diff = diff[:800] + "\n... (已截断)"
		}
		elements = append(elements, feishuElement{
			Tag:  "div",
			Text: &feishuText{Tag: "lark_md", Content: fmt.Sprintf("**变更详情**\n```diff\n%s\n```", diff)},
		})
	}

	// 元数据（如果有额外信息）
	if metaText := buildMetadataText(event.Metadata); metaText != "" {
		elements = append(elements, feishuElement{
			Tag:  "div",
			Text: &feishuText{Tag: "lark_md", Content: metaText},
		})
	}

	// 时间和事件 ID
	elements = append(elements, feishuElement{
		Tag: "div",
		Text: &feishuText{Tag: "lark_md", Content: fmt.Sprintf("⏰ %s  ·  `%s`", event.StartedAt.Format("2006-01-02 15:04:05"), event.EventID[:8])},
	})

	// 操作按钮
	if n.baseURL != "" {
		elements = append(elements, feishuElement{Tag: "hr"})
		elements = append(elements, feishuElement{
			Tag: "action",
			Actions: []feishuAction{
				{
					Tag:  "button",
					Text: feishuText{Tag: "plain_text", Content: "📋 查看详情"},
					URL:  fmt.Sprintf("%s/?event_id=%s", n.baseURL, event.EventID),
					Type: "primary",
				},
				{
					Tag:  "button",
					Text: feishuText{Tag: "plain_text", Content: "🔍 查询关联告警"},
					URL:  fmt.Sprintf("%s/?q=关联告警+%s+%s", n.baseURL, event.Service, event.Env),
				},
			},
		})
	}

	return &feishuCard{
		MsgType: "interactive",
		Card: feishuInner{
			Header: feishuHeader{
				Title:    feishuText{Tag: "plain_text", Content: title},
				Template: color,
			},
			Elements: elements,
		},
	}
}

// buildCompareText 构建 Before/After 对比文本。
func buildCompareText(before, after map[string]any) string {
	if len(before) == 0 && len(after) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("**状态变更**\n")

	// 收集所有 key
	keys := make(map[string]bool)
	for k := range before {
		keys[k] = true
	}
	for k := range after {
		keys[k] = true
	}

	count := 0
	for k := range keys {
		if count >= 6 {
			sb.WriteString("... 等更多字段\n")
			break
		}
		bVal := fmt.Sprintf("%v", before[k])
		aVal := fmt.Sprintf("%v", after[k])
		if bVal == aVal {
			continue
		}
		if bVal == "" {
			sb.WriteString(fmt.Sprintf("• `%s`: — → `%s`\n", k, truncateStr(aVal, 60)))
		} else if aVal == "" {
			sb.WriteString(fmt.Sprintf("• `%s`: `%s` → —\n", k, truncateStr(bVal, 60)))
		} else {
			sb.WriteString(fmt.Sprintf("• `%s`: `%s` → `%s`\n", k, truncateStr(bVal, 60), truncateStr(aVal, 60)))
		}
		count++
	}
	return sb.String()
}

// buildMetadataText 构建元数据文本。
func buildMetadataText(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	// 只显示关键元数据
	interesting := []string{"image", "version", "replicas", "replicas_before", "replicas_after", "commit", "branch", "pipeline_url"}
	var sb strings.Builder
	showed := 0
	for _, key := range interesting {
		if val, ok := metadata[key]; ok {
			if showed == 0 {
				sb.WriteString("**附加信息**\n")
			}
			if showed >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("• `%s`: %s\n", key, truncateStr(fmt.Sprintf("%v", val), 80)))
			showed++
		}
	}
	return sb.String()
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
