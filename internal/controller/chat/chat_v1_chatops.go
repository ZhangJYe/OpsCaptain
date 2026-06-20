package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/chatops"
	"context"
)

// ChatOpsSend 发送消息到飞书群。
func (c *ControllerV1) ChatOpsSend(ctx context.Context, req *v1.ChatOpsSendReq) (res *v1.ChatOpsSendRes, err error) {
	if c.feishuSender == nil {
		return &v1.ChatOpsSendRes{
			Success: false,
			Error:   "feishu sender is not configured",
		}, nil
	}

	msg := &chatops.Message{
		Title:   req.Title,
		Content: req.Content,
		Level:   req.Level,
	}

	if err := c.feishuSender.Send(ctx, msg); err != nil {
		return &v1.ChatOpsSendRes{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &v1.ChatOpsSendRes{
		Success: true,
		Message: "消息已发送到飞书群",
	}, nil
}
