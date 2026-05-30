package app

import (
	aiservice "SuperBizAgent/internal/ai/service"
	"context"
	"fmt"
)

// RegisterChatTaskExecutor registers a chat task executor callback with the
// async task pipeline. The callback adapts ChatApp.HandleChat to the
// function signature expected by aiservice.RegisterChatTaskExecutor.
//
// This must be called before aiservice.StartChatTaskPipeline to ensure
// the executor is ready when the consumer starts.
func RegisterChatTaskExecutor(chatApp *ChatApp) {
	aiservice.RegisterChatTaskExecutor(func(ctx context.Context, sessionID, query string) (aiservice.ChatTaskExecutionResult, error) {
		result, err := chatApp.HandleChat(ctx, &ChatInput{
			SessionID: sessionID,
			Question:  query,
		})
		if err != nil {
			return aiservice.ChatTaskExecutionResult{}, err
		}
		if result == nil {
			return aiservice.ChatTaskExecutionResult{}, fmt.Errorf("chat task executor: HandleChat returned nil result with nil error")
		}
		return aiservice.ChatTaskExecutionResult{
			Answer:            result.Answer,
			Detail:            append([]string{}, result.Detail...),
			TraceID:           result.TraceID,
			Mode:              result.Mode,
			Degraded:          result.Degraded,
			DegradationReason: result.DegradationReason,
		}, nil
	})
}
