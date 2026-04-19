package v1

import (
	"github.com/gogf/gf/v2/frame/g"
)

type ChatReq struct {
	g.Meta   `path:"/chat" method:"post" summary:"对话"`
	Id       string `json:"Id" v:"required|max-length:128#会话ID不能为空|会话ID长度不能超过128"`
	Question string `json:"Question" v:"required|max-length:8000#问题不能为空|问题长度不能超过8000"`
}

type ChatRes struct {
	Answer            string   `json:"answer"`
	TraceID           string   `json:"trace_id,omitempty"`
	Detail            []string `json:"detail,omitempty"`
	Mode              string   `json:"mode,omitempty"`
	Cached            bool     `json:"cached,omitempty"`
	Degraded          bool     `json:"degraded,omitempty"`
	DegradationReason string   `json:"degradation_reason,omitempty"`
}

type ChatSubmitReq struct {
	g.Meta   `path:"/chat_submit" method:"post" summary:"提交异步对话任务"`
	Id       string `json:"Id" v:"required|max-length:128#会话ID不能为空|会话ID长度不能超过128"`
	Question string `json:"Question" v:"required|max-length:8000#问题不能为空|问题长度不能超过8000"`
}

type ChatSubmitRes struct {
	TaskID    string `json:"task_id"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

type ChatTaskReq struct {
	g.Meta  `path:"/chat_task" method:"get" summary:"查询异步对话任务状态"`
	TaskID  string `json:"task_id" v:"required|max-length:128#任务ID不能为空|任务ID长度不能超过128"`
	Session string `json:"session_id,omitempty" v:"max-length:128#会话ID长度不能超过128"`
}

type ChatTaskRes struct {
	TaskID            string   `json:"task_id"`
	SessionID         string   `json:"session_id"`
	Query             string   `json:"query"`
	Status            string   `json:"status"`
	Answer            string   `json:"answer,omitempty"`
	TraceID           string   `json:"trace_id,omitempty"`
	Detail            []string `json:"detail,omitempty"`
	Mode              string   `json:"mode,omitempty"`
	Degraded          bool     `json:"degraded,omitempty"`
	DegradationReason string   `json:"degradation_reason,omitempty"`
	Error             string   `json:"error,omitempty"`
	CreatedAt         int64    `json:"created_at"`
	UpdatedAt         int64    `json:"updated_at"`
	StartedAt         int64    `json:"started_at,omitempty"`
	FinishedAt        int64    `json:"finished_at,omitempty"`
}

type ChatStreamReq struct {
	g.Meta   `path:"/chat_stream" method:"post" summary:"流式对话"`
	Id       string `json:"Id" v:"required|max-length:128#会话ID不能为空|会话ID长度不能超过128"`
	Question string `json:"Question" v:"required|max-length:8000#问题不能为空|问题长度不能超过8000"`
}

type ChatStreamRes struct {
}

type FileUploadReq struct {
	g.Meta `path:"/upload" method:"post" mime:"multipart/form-data" summary:"文件上传"`
}

type FileUploadRes struct {
	FileName string `json:"fileName" dc:"保存的文件名"`
	FileID   string `json:"fileId"   dc:"文件唯一标识"`
	FileSize int64  `json:"fileSize" dc:"文件大小(字节)"`
}

type AIOpsReq struct {
	g.Meta `path:"/ai_ops" method:"post" summary:"AI运维"`
	Query  string `json:"query,omitempty" v:"max-length:8000#自定义分析指令长度不能超过8000"`
}

type AIOpsRes struct {
	TraceID           string   `json:"trace_id"`
	Result            string   `json:"result"`
	Detail            []string `json:"detail"`
	ApprovalRequired  bool     `json:"approval_required,omitempty"`
	ApprovalRequestID string   `json:"approval_request_id,omitempty"`
	ApprovalStatus    string   `json:"approval_status,omitempty"`
	ExecutionPlan     []string `json:"execution_plan,omitempty"`
	Degraded          bool     `json:"degraded,omitempty"`
	DegradationReason string   `json:"degradation_reason,omitempty"`
}

type AIOpsTraceReq struct {
	g.Meta  `path:"/ai_ops_trace" method:"get" summary:"查询AI运维Trace"`
	TraceID string `json:"trace_id" v:"required|max-length:128#TraceID不能为空|TraceID长度不能超过128"`
}

type AIOpsTraceEvent struct {
	EventID   string         `json:"event_id"`
	TaskID    string         `json:"task_id"`
	TraceID   string         `json:"trace_id"`
	Type      string         `json:"type"`
	Agent     string         `json:"agent"`
	Message   string         `json:"message,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

type AIOpsTraceRes struct {
	TraceID string            `json:"trace_id"`
	Detail  []string          `json:"detail"`
	Events  []AIOpsTraceEvent `json:"events"`
}

type TokenAuditReq struct {
	g.Meta    `path:"/token_audit" method:"get" summary:"查询会话 Token 审计"`
	SessionID string `json:"session_id" v:"required|max-length:128#SessionID不能为空|SessionID长度不能超过128"`
	Date      string `json:"date,omitempty" v:"date-format:Y-m-d#日期格式必须为YYYY-MM-DD"`
}

type TokenAuditRes struct {
	Date             string `json:"date"`
	SessionID        string `json:"session_id"`
	UserID           string `json:"user_id,omitempty"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	Calls            int    `json:"calls"`
	LastModel        string `json:"last_model,omitempty"`
	UpdatedAt        int64  `json:"updated_at,omitempty"`
}

type ApprovalRequestsReq struct {
	g.Meta `path:"/approval_requests" method:"get" summary:"查询审批请求"`
	Status string `json:"status,omitempty" v:"in:pending,approved,rejected,executed#审批状态不合法"`
}

type ApprovalRequestItem struct {
	ID            string   `json:"id"`
	Query         string   `json:"query"`
	Reason        string   `json:"reason"`
	Status        string   `json:"status"`
	SessionID     string   `json:"session_id,omitempty"`
	UserID        string   `json:"user_id,omitempty"`
	RequestedBy   string   `json:"requested_by,omitempty"`
	ReviewedBy    string   `json:"reviewed_by,omitempty"`
	ReviewReason  string   `json:"review_reason,omitempty"`
	ExecutionPlan []string `json:"execution_plan,omitempty"`
	ResultTraceID string   `json:"result_trace_id,omitempty"`
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
	ApprovedAt    int64    `json:"approved_at,omitempty"`
	RejectedAt    int64    `json:"rejected_at,omitempty"`
	ExecutionAt   int64    `json:"execution_at,omitempty"`
}

type ApprovalRequestsRes struct {
	Items []ApprovalRequestItem `json:"items"`
}

type ApprovalActionReq struct {
	g.Meta    `path:"/approval_requests/approve" method:"post" summary:"批准审批请求"`
	RequestID string `json:"request_id" v:"required|max-length:128#RequestID不能为空|RequestID长度不能超过128"`
}

type ApprovalRejectReq struct {
	g.Meta    `path:"/approval_requests/reject" method:"post" summary:"拒绝审批请求"`
	RequestID string `json:"request_id" v:"required|max-length:128#RequestID不能为空|RequestID长度不能超过128"`
	Reason    string `json:"reason,omitempty" v:"max-length:512#拒绝原因长度不能超过512"`
}
