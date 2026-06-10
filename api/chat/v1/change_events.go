package v1

import "github.com/gogf/gf/v2/frame/g"

// === 变更事件 API ===

// ChangeEventCreateReq 接收变更事件。
type ChangeEventCreateReq struct {
	g.Meta     `path:"/change_events" method:"post" tags:"ChangeEvents" summary:"接收变更事件"`
	Source     string         `json:"source"`                  // cicd, k8s, config, manual
	EventType  string         `json:"event_type" v:"required"` // deploy, rollback, config_update, scale, restart
	Service    string         `json:"service" v:"required"`
	Env        string         `json:"env"`
	Namespace  string         `json:"namespace"`
	Cluster    string         `json:"cluster"`
	Summary    string         `json:"summary" v:"required"`
	Operator   string         `json:"operator"`
	Before     map[string]any `json:"before"`
	After      map[string]any `json:"after"`
	Diff       string         `json:"diff"`
	StartedAt  string         `json:"started_at"` // ISO8601, 默认 now
	RiskLevel  string         `json:"risk_level"` // 自动评估 if empty
	DedupeKey  string         `json:"dedupe_key"` // 可选
	RawPayload map[string]any `json:"raw_payload"`
	Metadata   map[string]any `json:"metadata"`
}

type ChangeEventCreateRes struct {
	EventID  string `json:"event_id"`
	Accepted bool   `json:"accepted"` // false if deduplicated
}

// ChangeEventListReq 结构化查询变更事件。
type ChangeEventListReq struct {
	g.Meta    `path:"/change_events" method:"get" tags:"ChangeEvents" summary:"查询变更事件"`
	Service   string `json:"service"`    // 按服务过滤
	Env       string `json:"env"`        // 按环境过滤
	EventType string `json:"event_type"` // 按类型过滤
	RiskLevel string `json:"risk_level"` // 最低风险等级
	Since     string `json:"since"`      // ISO8601
	Until     string `json:"until"`      // ISO8601
	Limit     int    `json:"limit" v:"max:200"` // 默认 50，上限 200
}

type ChangeEventListRes struct {
	Items []ChangeEventItem `json:"items"`
	Total int               `json:"total"`
}

// ChangeEventItem 是 API 响应中的变更事件项。
type ChangeEventItem struct {
	EventID         string         `json:"event_id"`
	Source          string         `json:"source"`
	EventType       string         `json:"event_type"`
	Service         string         `json:"service"`
	Env             string         `json:"env"`
	Namespace       string         `json:"namespace"`
	Cluster         string         `json:"cluster"`
	Summary         string         `json:"summary"`
	Before          map[string]any `json:"before,omitempty"`
	After           map[string]any `json:"after,omitempty"`
	Diff            string         `json:"diff,omitempty"`
	RiskLevel       string         `json:"risk_level"`
	Operator        string         `json:"operator"`
	StartedAt       string         `json:"started_at"`
	FinishedAt      string         `json:"finished_at"`
	CorrelationKeys []string       `json:"correlation_keys,omitempty"`
}

// ChangeEventGetReq 获取单个变更事件详情。
type ChangeEventGetReq struct {
	g.Meta  `path:"/change_events/{event_id}" method:"get" tags:"ChangeEvents" summary:"获取变更事件详情"`
	EventID string `json:"event_id" v:"required"`
}

type ChangeEventGetRes struct {
	Item ChangeEventItem `json:"item"`
}

// ChangeEventStreamReq SSE 推送变更事件。
type ChangeEventStreamReq struct {
	g.Meta  `path:"/change_events/stream" method:"get" tags:"ChangeEvents" summary:"SSE 订阅变更事件"`
	Service string `json:"service"` // 订阅特定服务的变更
	Env     string `json:"env"`     // 订阅特定环境的变更
}

type ChangeEventStreamRes struct{}

// === Webhook API ===

// ChangeEventWebhookReq webhook 接入变更事件。
type ChangeEventWebhookReq struct {
	g.Meta `path:"/webhooks/change_events/{source}" method:"post" tags:"ChangeEvents" summary:"Webhook 接入变更事件"`
	Source string `json:"source" v:"required"` // 路由到对应 adapter
	// Body: 原始 webhook payload
}

type ChangeEventWebhookRes struct {
	EventID  string `json:"event_id"`
	Accepted bool   `json:"accepted"`
}
