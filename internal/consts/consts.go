package consts

type contextKey string

const (
	CtxKeyClientID  contextKey = "client_id"
	CtxKeySessionID contextKey = "session_id"
	CtxKeyRequestID contextKey = "request_id"
	CtxKeyTraceID   contextKey = "trace_id"
	CtxKeyUserID    contextKey = "user_id"
	CtxKeyUserRole  contextKey = "user_role"

	CtxKeyApprovalBypass    contextKey = "approval_bypass"
	CtxKeyApprovalRequestID contextKey = "approval_request_id"
)
