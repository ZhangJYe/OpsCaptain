package consts

type contextKey string

const (
	CtxKeyClientID  contextKey = "client_id"
	CtxKeySessionID contextKey = "session_id"
	CtxKeyRequestID contextKey = "request_id"
	CtxKeyUserID    contextKey = "user_id"
	CtxKeyUserRole  contextKey = "user_role"
)
