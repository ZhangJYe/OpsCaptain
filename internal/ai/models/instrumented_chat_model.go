package models

import (
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/metrics"
	"SuperBizAgent/utility/resilience"
	traceutil "SuperBizAgent/utility/tracing"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type instrumentedChatModel struct {
	inner     einomodel.ToolCallingChatModel
	modelName string
}

var (
	tokenAuditEnforcer = func(context.Context) error { return nil }
	tokenAuditRecorder = func(context.Context, string, int, int) {}
)

func SetTokenAuditHooks(
	enforcer func(context.Context) error,
	recorder func(context.Context, string, int, int),
) {
	if enforcer != nil {
		tokenAuditEnforcer = enforcer
	}
	if recorder != nil {
		tokenAuditRecorder = recorder
	}
}

func wrapToolCallingChatModel(inner einomodel.ToolCallingChatModel, modelName string) einomodel.ToolCallingChatModel {
	if inner == nil {
		return nil
	}
	return &instrumentedChatModel{
		inner:     inner,
		modelName: modelName,
	}
}

func (m *instrumentedChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.Message, error) {
	agent := llmAgentLabel(ctx)
	started := time.Now()
	ctx, span := traceutil.StartSpan(
		ctx,
		"llm",
		"llm.generate",
		oteltrace.WithAttributes(
			attribute.String("agent.name", agent),
			attribute.String("llm.model", m.modelName),
		),
	)
	defer span.End()

	if err := tokenAuditEnforcer(ctx); err != nil {
		status := llmStatus(err)
		metrics.ObserveLLMCall(agent, m.modelName, status, time.Since(started))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	release, err := resilience.AcquireLLMSlot(ctx)
	if err != nil {
		status := llmStatus(err)
		metrics.ObserveLLMCall(agent, m.modelName, status, time.Since(started))
		emitLLMEvent(ctx, agent, "llm_queue_timeout", map[string]any{
			"model":  m.modelName,
			"status": status,
			"error":  err.Error(),
		})
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	defer release()

	output, err := resilience.Execute(ctx, llmCallOption(agent, m.modelName), func(callCtx context.Context) (*schema.Message, error) {
		return m.inner.Generate(callCtx, input, opts...)
	})

	status := llmStatus(err)
	metrics.ObserveLLMCall(agent, m.modelName, status, time.Since(started))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		emitLLMEvent(ctx, agent, "llm_call_failed", map[string]any{
			"model":  m.modelName,
			"status": status,
			"error":  err.Error(),
		})
		return nil, err
	}

	usage := usageFromMessage(output)
	recordLLMUsage(ctx, agent, m.modelName, usage)
	if usage != nil {
		tokenAuditRecorder(ctx, m.modelName, usage.PromptTokens, usage.CompletionTokens)
	}
	annotateUsage(span, usage)
	return output, nil
}

func (m *instrumentedChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...einomodel.Option) (*schema.StreamReader[*schema.Message], error) {
	agent := llmAgentLabel(ctx)
	started := time.Now()
	ctx, span := traceutil.StartSpan(
		ctx,
		"llm",
		"llm.stream",
		oteltrace.WithAttributes(
			attribute.String("agent.name", agent),
			attribute.String("llm.model", m.modelName),
		),
	)

	if err := tokenAuditEnforcer(ctx); err != nil {
		status := llmStatus(err)
		metrics.ObserveLLMCall(agent, m.modelName, status, time.Since(started))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}

	release, err := resilience.AcquireLLMSlot(ctx)
	if err != nil {
		status := llmStatus(err)
		metrics.ObserveLLMCall(agent, m.modelName, status, time.Since(started))
		emitLLMEvent(ctx, agent, "llm_queue_timeout", map[string]any{
			"model":  m.modelName,
			"status": status,
			"error":  err.Error(),
		})
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}

	// Stream 调用不经过 resilience.Execute 的超时包装，因为流式 LLM 调用需要在
	// reader 返回后长时间保持底层 HTTP 连接。resilience.Execute 会在 fn 返回后
	// 立即 cancel 超时 context，这会杀死流读取器。
	//
	// 这里只使用 circuit breaker，超时由 HTTP client 层控制。
	opt := llmCallOption(agent, m.modelName)
	cb := resilience.GetBreaker(opt.Name)
	if !cb.Allow() {
		release()
		span.RecordError(fmt.Errorf("circuit breaker open"))
		span.SetStatus(codes.Error, "circuit breaker open")
		span.End()
		return nil, fmt.Errorf("circuit breaker open for %s", opt.Name)
	}

	reader, err := m.inner.Stream(ctx, input, opts...)
	if err != nil {
		release()
		cb.RecordFailure()
		status := llmStatus(err)
		metrics.ObserveLLMCall(agent, m.modelName, status, time.Since(started))
		emitLLMEvent(ctx, agent, "llm_stream_failed", map[string]any{
			"model":  m.modelName,
			"status": status,
			"error":  err.Error(),
		})
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.End()
		return nil, err
	}
	cb.RecordSuccess()

	streamReader, streamWriter := schema.Pipe[*schema.Message](8)
	go func() {
		defer release()
		defer span.End()
		defer reader.Close()
		defer streamWriter.Close()

		var (
			lastUsage *schema.TokenUsage
			streamErr error
		)

		for {
			msg, recvErr := reader.Recv()
			if recvErr != nil {
				if !errors.Is(recvErr, io.EOF) {
					streamErr = recvErr
					streamWriter.Send(nil, recvErr)
				}
				break
			}

			lastUsage = maxUsage(lastUsage, usageFromMessage(msg))
			if closed := streamWriter.Send(msg, nil); closed {
				streamErr = context.Canceled
				break
			}
		}

		status := llmStatus(streamErr)
		metrics.ObserveLLMCall(agent, m.modelName, status, time.Since(started))
		if streamErr != nil {
			span.RecordError(streamErr)
			span.SetStatus(codes.Error, streamErr.Error())
			emitLLMEvent(ctx, agent, "llm_stream_failed", map[string]any{
				"model":  m.modelName,
				"status": status,
				"error":  streamErr.Error(),
			})
			return
		}

		recordLLMUsage(ctx, agent, m.modelName, lastUsage)
		if lastUsage != nil {
			tokenAuditRecorder(ctx, m.modelName, lastUsage.PromptTokens, lastUsage.CompletionTokens)
		}
		annotateUsage(span, lastUsage)
	}()

	return streamReader, nil
}

func (m *instrumentedChatModel) WithTools(tools []*schema.ToolInfo) (einomodel.ToolCallingChatModel, error) {
	wrapped, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return wrapToolCallingChatModel(wrapped, m.modelName), nil
}

func llmCallOption(agent, modelName string) resilience.CallOption {
	return resilience.CallOption{
		Timeout:    30 * time.Second,
		MaxRetries: 2,
		RetryDelay: time.Second,
		Name:       fmt.Sprintf("llm:%s:%s", agent, modelName),
	}
}

func llmAgentLabel(ctx context.Context) string {
	if task, ok := runtime.TaskFromContext(ctx); ok && task.Assignee != "" {
		return task.Assignee
	}
	if sessionID, ok := ctx.Value(consts.CtxKeySessionID).(string); ok && sessionID != "" {
		return "chat"
	}
	return "unknown"
}

func llmStatus(err error) string {
	switch {
	case err == nil:
		return "success"
	case strings.Contains(strings.ToLower(err.Error()), "daily token limit exceeded"):
		return "token_limit"
	case resilience.IsConcurrencyLimitError(err):
		return "queued_timeout"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case strings.Contains(strings.ToLower(err.Error()), "circuit breaker open"):
		return "circuit_open"
	default:
		return "error"
	}
}

func usageFromMessage(msg *schema.Message) *schema.TokenUsage {
	if msg == nil || msg.ResponseMeta == nil {
		return nil
	}
	return msg.ResponseMeta.Usage
}

func maxUsage(current, next *schema.TokenUsage) *schema.TokenUsage {
	if next == nil {
		return current
	}
	if current == nil {
		return &schema.TokenUsage{
			PromptTokens:     next.PromptTokens,
			CompletionTokens: next.CompletionTokens,
			TotalTokens:      next.TotalTokens,
		}
	}
	if next.PromptTokens > current.PromptTokens {
		current.PromptTokens = next.PromptTokens
	}
	if next.CompletionTokens > current.CompletionTokens {
		current.CompletionTokens = next.CompletionTokens
	}
	if next.TotalTokens > current.TotalTokens {
		current.TotalTokens = next.TotalTokens
	}
	return current
}

func recordLLMUsage(ctx context.Context, agent, modelName string, usage *schema.TokenUsage) {
	if usage == nil {
		return
	}
	metrics.AddLLMTokens(agent, modelName, "prompt", usage.PromptTokens)
	metrics.AddLLMTokens(agent, modelName, "completion", usage.CompletionTokens)
	emitLLMEvent(ctx, agent, "llm_usage", map[string]any{
		"model":             modelName,
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	})
}

func annotateUsage(span interface{ SetAttributes(...attribute.KeyValue) }, usage *schema.TokenUsage) {
	if usage == nil {
		return
	}
	span.SetAttributes(
		attribute.Int("llm.prompt_tokens", usage.PromptTokens),
		attribute.Int("llm.completion_tokens", usage.CompletionTokens),
		attribute.Int("llm.total_tokens", usage.TotalTokens),
	)
}

func emitLLMEvent(ctx context.Context, agent, message string, payload map[string]any) {
	rt, ok := runtime.FromContext(ctx)
	if !ok {
		return
	}
	task, ok := runtime.TaskFromContext(ctx)
	if !ok || task == nil {
		return
	}
	rt.EmitInfo(ctx, task, agent, message, payload)
}
