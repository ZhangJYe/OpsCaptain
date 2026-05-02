package events

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// BeforeToolCallFunc 工具调用前的拦截函数
// 返回修改后的 args，或 error 来阻断调用
type BeforeToolCallFunc func(ctx context.Context, toolName string, args string) (string, error)

// AfterToolCallFunc 工具调用后的处理函数
// 返回修改后的 result，或 error 表示处理失败
type AfterToolCallFunc func(ctx context.Context, toolName string, args string, result string, err error) (string, error)

// ToolWrapper 工具包装器，支持 beforeToolCall / afterToolCall 拦截
type ToolWrapper struct {
	inner      tool.InvokableTool
	before     BeforeToolCallFunc
	after      AfterToolCallFunc
	emitter    Emitter
	traceID    string
	cachedName string
}

// WrapTool 包装单个工具（必须是 InvokableTool）
func WrapTool(t tool.InvokableTool, emitter Emitter, traceID string, before BeforeToolCallFunc, after AfterToolCallFunc) *ToolWrapper {
	return &ToolWrapper{
		inner:   t,
		before:  before,
		after:   after,
		emitter: emitter,
		traceID: traceID,
	}
}

// WrapTools 批量包装工具，自动过滤非 InvokableTool
func WrapTools(tools []tool.BaseTool, emitter Emitter, traceID string, before BeforeToolCallFunc, after AfterToolCallFunc) []tool.BaseTool {
	result := make([]tool.BaseTool, 0, len(tools))
	for _, t := range tools {
		if it, ok := t.(tool.InvokableTool); ok {
			result = append(result, WrapTool(it, emitter, traceID, before, after))
		} else {
			result = append(result, t)
		}
	}
	return result
}

// Info 返回工具信息（透传）
func (w *ToolWrapper) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return w.inner.Info(ctx)
}

// InvokableRun 执行工具调用，支持拦截
func (w *ToolWrapper) InvokableRun(ctx context.Context, args string, opts ...tool.Option) (string, error) {
	toolName := w.toolName(ctx)
	startTime := time.Now()

	// 发射 tool_call_start 事件
	if w.emitter != nil {
		w.emitter.Emit(ctx, NewEvent(EventToolCallStart, w.traceID, toolName, map[string]any{
			"tool_name": toolName,
		}))
	}

	// beforeToolCall 拦截
	if w.before != nil {
		modifiedArgs, err := w.before(ctx, toolName, args)
		if err != nil {
			w.emitToolEnd(ctx, toolName, args, startTime, "", err)
			return "", fmt.Errorf("tool %s blocked by beforeToolCall: %w", toolName, err)
		}
		args = modifiedArgs
	}

	// 执行实际工具
	result, execErr := w.inner.InvokableRun(ctx, args, opts...)

	// afterToolCall 处理（无论成功失败都执行）
	if w.after != nil {
		modifiedResult, afterErr := w.after(ctx, toolName, args, result, execErr)
		if execErr == nil && afterErr != nil {
			// 工具本身成功，但 after hook 失败（脱敏/校验/审计失败）
			// 不暴露原始 result，只返回错误
			w.emitToolEndWithAfterError(ctx, toolName, args, startTime, afterErr)
			return fmt.Sprintf("[工具结果处理失败] %s: %s\n该工具的结果未能通过安全校验，请告知用户。", toolName, afterErr.Error()), nil
		}
		if afterErr == nil {
			result = modifiedResult
		}
	}

	// 发射 tool_call_end 事件
	w.emitToolEnd(ctx, toolName, args, startTime, result, execErr)

	if execErr != nil {
		return fmt.Sprintf("[工具调用失败] %s: %s\n请告知用户该工具返回了错误，不要用通用建议替代实际数据。", toolName, execErr.Error()), nil
	}
	return result, nil
}

// StreamableRun 执行工具调用（流式），统一走 InvokableRun 保证事件发射
func (w *ToolWrapper) StreamableRun(ctx context.Context, args string, opts ...tool.Option) (*schema.StreamReader[string], error) {
	result, err := w.InvokableRun(ctx, args, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]string{result}), nil
}

func (w *ToolWrapper) emitToolEnd(ctx context.Context, toolName, args string, start time.Time, result string, err error) {
	if w.emitter == nil {
		return
	}

	payload := map[string]any{
		"tool_name":   toolName,
		"duration_ms": time.Since(start).Milliseconds(),
		"success":     err == nil,
	}
	if err != nil {
		payload["error"] = err.Error()
	}
	if len(result) > 200 {
		payload["summary"] = result[:200] + "..."
	} else if result != "" {
		payload["summary"] = result
	}

	w.emitter.Emit(ctx, NewEvent(EventToolCallEnd, w.traceID, toolName, payload))
}

// emitToolEndWithAfterError 发射 after hook 失败的事件（工具本身成功，after hook 失败）
// 不包含原始 result summary，避免泄露未脱敏的数据
func (w *ToolWrapper) emitToolEndWithAfterError(ctx context.Context, toolName, args string, start time.Time, afterErr error) {
	if w.emitter == nil {
		return
	}

	payload := map[string]any{
		"tool_name":   toolName,
		"duration_ms": time.Since(start).Milliseconds(),
		"success":     false,
		"error":       afterErr.Error(),
		"after_error": true,
	}

	w.emitter.Emit(ctx, NewEvent(EventToolCallEnd, w.traceID, toolName, payload))
}

func (w *ToolWrapper) toolName(ctx context.Context) string {
	if w.cachedName != "" {
		return w.cachedName
	}
	info, err := w.inner.Info(ctx)
	if err != nil {
		return "unknown"
	}
	w.cachedName = info.Name
	return w.cachedName
}

// --- 常用的 before/after 函数 ---

// ValidateBeforeToolCall 参数基础校验 beforeToolCall
// 校验 JSON 格式、非空，防止 LLM 生成无效参数直接打到后端
func ValidateBeforeToolCall() BeforeToolCallFunc {
	return func(ctx context.Context, toolName string, args string) (string, error) {
		args = strings.TrimSpace(args)
		if args == "" {
			return "", fmt.Errorf("tool %s received empty arguments", toolName)
		}
		// 基础 JSON 格式校验（不校验 schema，只看是不是合法 JSON）
		if !json.Valid([]byte(args)) {
			return "", fmt.Errorf("tool %s received invalid JSON arguments: %.100s", toolName, args)
		}
		return args, nil
	}
}

// SummaryAfterToolCall 结果摘要 afterToolCall
func SummaryAfterToolCall(maxLen int) AfterToolCallFunc {
	return func(ctx context.Context, toolName string, args string, result string, err error) (string, error) {
		if err != nil {
			return result, err
		}
		// 结果过长时截断，减少 token 消耗
		if maxLen > 0 && len(result) > maxLen {
			return result[:maxLen] + "...", nil
		}
		return result, nil
	}
}
