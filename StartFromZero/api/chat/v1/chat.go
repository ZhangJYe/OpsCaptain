package v1

import (
	"github.com/gogf/gf/v2/frame/g"
)

// ChatReq 定义请求结构
type ChatReq struct {
	g.Meta   `path:"/chat" method:"post" summary:"AI对话接口" tags:"Chat"`
	Question string `json:"question" v:"required#问题不能为空" dc:"用户的问题"`
}

// ChatRes 定义响应结构
type ChatRes struct {
	Answer string `json:"answer" dc:"AI的回答"`
}
