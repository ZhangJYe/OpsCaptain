package protocol

import "time"

// ChangeEvent 表示一个变更事件，是 AIOps 的时序事实源。
// 变更事件的本质是：某个服务在某个时间窗口发生过什么人为或系统变更，
// 这些变更可能解释后续指标、日志、告警异常。
type ChangeEvent struct {
	// === 标识 ===
	EventID   string `json:"event_id"`   // UUID，系统生成
	DedupeKey string `json:"dedupe_key"` // 幂等键，防 webhook 重复投递

	// === 来源 ===
	Source    string `json:"source"`     // "cicd", "k8s", "config", "manual"
	EventType string `json:"event_type"` // "deploy", "rollback", "config_update", "scale", "restart"

	// === 关联标识（用于结构化查询）===
	Service   string `json:"service"` // 影响的服务名（主查询键）
	Env       string `json:"env"`     // "prod", "staging", "dev"
	Namespace string `json:"namespace"`
	Cluster   string `json:"cluster"`

	// === 变更内容 ===
	Summary    string         `json:"summary"`               // 一句话摘要
	Before     map[string]any `json:"before,omitempty"`      // 变更前状态
	After      map[string]any `json:"after,omitempty"`       // 变更后状态
	Diff       string         `json:"diff,omitempty"`        // 差异文本
	RawPayload map[string]any `json:"raw_payload,omitempty"` // 原始 webhook payload

	// === 时间 ===
	StartedAt  time.Time `json:"started_at"`  // 变更开始时间（用于时间关联）
	FinishedAt time.Time `json:"finished_at"` // 变更完成时间
	CreatedAt  time.Time `json:"created_at"`  // 事件接入时间

	// === 风险评估 ===
	RiskLevel string `json:"risk_level"` // "low", "medium", "high", "critical"
	Operator  string `json:"operator"`   // 触发者

	// === 关联键（用于和告警、日志绑定）===
	CorrelationKeys []string `json:"correlation_keys,omitempty"`

	// === 元数据 ===
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ChangeEventFilter 是结构化查询条件。
type ChangeEventFilter struct {
	Services  []string   // 按服务名过滤（支持多服务）
	Env       string     // 按环境过滤
	Cluster   string     // 按集群过滤
	EventType string     // 按变更类型过滤
	RiskLevel string     // 最低风险等级
	Since     *time.Time // 起始时间
	Until     *time.Time // 结束时间
	Limit     int        // 返回条数上限
}

// 变更事件来源常量。
const (
	ChangeSourceCI_CD   = "cicd"
	ChangeSourceGitHub  = "github"
	ChangeSourceGitLab  = "gitlab"
	ChangeSourceArgoCD  = "argocd"
	ChangeSourceK8s     = "k8s"
	ChangeSourceConfig  = "config"
	ChangeSourceManual  = "manual"
	ChangeSourceWebhook = "webhook"
)

// 变更事件类型常量。
const (
	ChangeTypeDeploy         = "deploy"
	ChangeTypeRollback       = "rollback"
	ChangeTypeGitPush        = "git_push"
	ChangeTypeMerge          = "merge"
	ChangeTypeRelease        = "release"
	ChangeTypePipeline       = "pipeline"
	ChangeTypeScale          = "scale"
	ChangeTypeConfigUpdate   = "config_update"
	ChangeTypeRestart        = "restart"
	ChangeTypeResourceUpdate = "resource_update"
	ChangeTypeDNSSwitch      = "dns_switch"
	ChangeTypeFailover       = "failover"
	ChangeTypeMaintenance    = "maintenance"
)

// 变更事件风险等级常量。
const (
	ChangeRiskLow      = "low"
	ChangeRiskMedium   = "medium"
	ChangeRiskHigh     = "high"
	ChangeRiskCritical = "critical"
)

// ChangeEventSource 常量列表，用于验证。
var ValidChangeSources = []string{
	ChangeSourceCI_CD, ChangeSourceGitHub, ChangeSourceGitLab, ChangeSourceArgoCD,
	ChangeSourceK8s, ChangeSourceConfig, ChangeSourceManual, ChangeSourceWebhook,
}

// ValidChangeEventTypes 常量列表，用于验证。
var ValidChangeEventTypes = []string{
	ChangeTypeDeploy, ChangeTypeRollback, ChangeTypeGitPush, ChangeTypeMerge,
	ChangeTypeRelease, ChangeTypePipeline, ChangeTypeScale, ChangeTypeConfigUpdate,
	ChangeTypeRestart, ChangeTypeResourceUpdate, ChangeTypeDNSSwitch,
	ChangeTypeFailover, ChangeTypeMaintenance,
}
