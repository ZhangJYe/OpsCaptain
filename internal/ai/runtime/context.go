package runtime

import "context"

type contextKey string

const runtimeContextKey contextKey = "multi_agent_runtime"

func withRuntime(ctx context.Context, rt *Runtime) context.Context {
	return context.WithValue(ctx, runtimeContextKey, rt)
}

func FromContext(ctx context.Context) (*Runtime, bool) {
	rt, ok := ctx.Value(runtimeContextKey).(*Runtime)
	return rt, ok
}
