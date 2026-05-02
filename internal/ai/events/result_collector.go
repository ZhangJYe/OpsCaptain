package events

import (
	"context"
	"sync"
)

// ResultCollector 收集工具调用结果，用于输出后校验
type ResultCollector struct {
	mu      sync.Mutex
	results []collectedResult
}

type collectedResult struct {
	toolName string
	summary  string
	success  bool
}

// NewResultCollector 创建结果收集器
func NewResultCollector() *ResultCollector {
	return &ResultCollector{}
}

// Emit 实现 Emitter 接口，收集 tool_call_end 事件中的结果
func (r *ResultCollector) Emit(ctx context.Context, event AgentEvent) {
	if event.Type != EventToolCallEnd {
		return
	}

	toolName, _ := event.Payload["tool_name"].(string)
	summary, _ := event.Payload["summary"].(string)
	errMsg, _ := event.Payload["error"].(string)

	if toolName == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, collectedResult{
		toolName: toolName,
		summary:  summary,
		success:  errMsg == "", // 与 ContractCollector 保持一致：无 error 即成功
	})
}

// ToolResults 返回所有收集到的工具结果摘要
func (r *ResultCollector) ToolResults() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var results []string
	for _, cr := range r.results {
		if cr.success && cr.summary != "" {
			results = append(results, cr.summary)
		}
	}
	return results
}

// HasToolCalls 是否有工具调用
func (r *ResultCollector) HasToolCalls() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.results) > 0
}

// FailedTools 返回失败的工具名列表
func (r *ResultCollector) FailedTools() []string {
	r.mu.Lock()
	defer r.mu.Unlock()

	var failed []string
	for _, cr := range r.results {
		if !cr.success {
			failed = append(failed, cr.toolName)
		}
	}
	return failed
}

// Reset 清空收集器
func (r *ResultCollector) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = nil
}
