package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	aiservice "SuperBizAgent/internal/ai/service"
	"context"
	"fmt"
)

func init() {
	aiservice.RegisterChatTaskExecutor(func(ctx context.Context, sessionID, query string) (aiservice.ChatTaskExecutionResult, error) {
		ctrl := &ControllerV1{}
		res, err := ctrl.Chat(ctx, &v1.ChatReq{
			Id:       sessionID,
			Question: query,
		})
		if err != nil {
			return aiservice.ChatTaskExecutionResult{}, err
		}
		if res == nil {
			return aiservice.ChatTaskExecutionResult{}, fmt.Errorf("chat response is empty")
		}
		return aiservice.ChatTaskExecutionResult{
			Answer:            res.Answer,
			Detail:            append([]string{}, res.Detail...),
			TraceID:           res.TraceID,
			Mode:              res.Mode,
			Degraded:          res.Degraded,
			DegradationReason: res.DegradationReason,
		}, nil
	})
}
