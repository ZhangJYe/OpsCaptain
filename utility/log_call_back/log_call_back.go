package log_call_back

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
	"github.com/gogf/gf/v2/frame/g"
)

func LogCallback(handler *callbacks.HandlerBuilder) callbacks.Handler {
	if handler == nil {
		handler = &callbacks.HandlerBuilder{}
	}
	handler.OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
		g.Log().Debugf(ctx, "[view start]:[%s:%s:%s]", info.Component, info.Type, info.Name)
		return ctx
	})
	handler.OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
		if b, err := json.MarshalIndent(output, "", "  "); err == nil {
			g.Log().Debugf(ctx, "%s", string(b))
		}
		g.Log().Debugf(ctx, "[view end]:[%s:%s:%s]", info.Component, info.Type, info.Name)
		return ctx
	})
	return handler.Build()
}

func LogCallbackHandler() compose.Option {
	return compose.WithCallbacks(LogCallback(nil))
}
