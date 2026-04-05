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
	Answer  string   `json:"answer"`
	TraceID string   `json:"trace_id,omitempty"`
	Detail  []string `json:"detail,omitempty"`
	Mode    string   `json:"mode,omitempty"`
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
	TraceID string   `json:"trace_id"`
	Result  string   `json:"result"`
	Detail  []string `json:"detail"`
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
