package chat

import (
	"context"

	// 引入我们要实现的接口定义
	v1 "StartFromZero/api/chat/v1"
	logicChat "StartFromZero/internal/logic/chat"
)

type Controller struct {
	service logicChat.Service
}

// New 创建一个新的 Controller 实例
func New(service logicChat.Service) *Controller {
	return &Controller{service: service}
}

// Chat 处理对话请求
// 注意：方法名 Chat 必须大写，且签名要符合 GoFrame 规范

func (c *Controller) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	answer, err := c.service.Ask(ctx, req.Question)
	if err != nil {
		return nil, err
	}
	return &v1.ChatRes{Answer: answer}, nil
}
