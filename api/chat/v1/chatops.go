package v1

import "github.com/gogf/gf/v2/frame/g"

// === ChatOps API ===

// ChatOpsSendReq 发送消息到飞书群。
type ChatOpsSendReq struct {
	g.Meta  `path:"/chatops/send" method:"post" tags:"ChatOps" summary:"发送消息到飞书群"`
	Title   string `json:"title" v:"required"`
	Content string `json:"content" v:"required"`
	Level   string `json:"level,omitempty" d:"info"`
}

// ChatOpsSendRes 发送结果。
type ChatOpsSendRes struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}
