package runtime

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
)

type contextKey string

const (
	runtimeContextKey contextKey = "aiops_runtime"
	taskContextKey    contextKey = "aiops_task"
)

func withRuntime(ctx context.Context, rt *Runtime) context.Context {
	return context.WithValue(ctx, runtimeContextKey, rt)
}

func FromContext(ctx context.Context) (*Runtime, bool) {
	rt, ok := ctx.Value(runtimeContextKey).(*Runtime)
	return rt, ok
}

func withTask(ctx context.Context, task *protocol.TaskEnvelope) context.Context {
	return context.WithValue(ctx, taskContextKey, task)
}

func TaskFromContext(ctx context.Context) (*protocol.TaskEnvelope, bool) {
	task, ok := ctx.Value(taskContextKey).(*protocol.TaskEnvelope)
	return task, ok
}
