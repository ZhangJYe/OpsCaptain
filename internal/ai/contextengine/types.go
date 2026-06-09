package contextengine

import (
	"SuperBizAgent/internal/ai/contextcompression"

	"github.com/cloudwego/eino/schema"
)

type ContextRequest struct {
	SessionID string
	UserID    string
	ProjectID string
	TraceID   string
	Mode      string
	Intent    string
	Query     string
	ToolItems []ContextItem
}

type ContextProfile struct {
	Name                string
	AllowHistory        bool
	AllowMemory         bool
	AllowDocs           bool
	AllowToolResults    bool
	Staged              bool
	MaxHistoryMessages  int
	MaxMemoryItems      int
	MaxToolItems        int
	MinMemoryConfidence float64
	AllowedMemoryScopes []string
	Budget              ContextBudget
}

type ContextBudget struct {
	MaxTotalTokens int
	SystemTokens   int
	HistoryTokens  int
	MemoryTokens   int
	DocumentTokens int
	ToolTokens     int
	ReservedTokens int
}

type ContextItem struct {
	ID               string
	SourceType       string
	SourceID         string
	Title            string
	Content          string
	Score            float64
	TrustLevel       string
	TokenEstimate    int
	Selected         bool
	DroppedReason    string
	Timestamp        int64
	FreshnessScore   float64
	OriginAgent      string
	SafetyLabel      string
	UpdatePolicy     string
	ConflictGroup    string
	CompressionLevel string
	Scope            string
	Confidence       float64
	Provenance       string
	ExpiresAt        int64
}

type BudgetSnapshot struct {
	HistoryTokens  int
	MemoryTokens   int
	DocumentTokens int
	ToolTokens     int
}

type StageTrace struct {
	Name          string
	SelectedCount int
	DroppedCount  int
	Notes         []string
	Retrieval     *RetrievalStageMetrics
	Recall        *RecallStageMetrics
}

// RecallStageMetrics 召回阶段的指标
type RecallStageMetrics struct {
	CacheHits int
	Embedded  int
	LatencyMs int64
	Degraded  bool
}

type RetrievalStageMetrics struct {
	CacheKey          string
	CacheHit          bool
	InitFailureCached bool
	InitLatencyMs     int64
	RetrieveLatencyMs int64
	RewriteLatencyMs  int64
	RerankLatencyMs   int64
	OriginalQuery     string
	RewrittenQuery    string
	RawResultCount    int
	ResultCount       int
	RerankEnabled     bool
}

type ContextAssemblyTrace struct {
	Profile            string
	Stages             []StageTrace
	SourcesConsidered  int
	SourcesSelected    int
	DroppedItems       []ContextItem
	BudgetBefore       BudgetSnapshot
	BudgetAfter        BudgetSnapshot
	LatencyMs          int64
	HistoryRecall      *HistoryRecallResult
	ToolRecall         *ToolRecallResult
	ToolRerank         *RerankOutcome
	Intent             *IntentResult
	CompressionReports []contextcompression.Report `json:"compression_reports,omitempty"`
}

type ContextPackage struct {
	Request         ContextRequest
	Profile         ContextProfile
	Query           string
	HistoryMessages []*schema.Message
	MemoryItems     []ContextItem
	DocumentItems   []ContextItem
	ToolItems       []ContextItem
	Trace           ContextAssemblyTrace
}
