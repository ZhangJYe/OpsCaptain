package service

import (
	"context"
	"strings"

	"SuperBizAgent/internal/ai/agent/supervisor"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/consts"

	"github.com/gogf/gf/v2/frame/g"
)

var chatMultiAgentKeywords = []string{
	"告警", "alert", "prometheus", "日志", "log", "排查", "故障", "incident",
	"知识库", "文档", "sop", "runbook", "mysql", "sql", "数据库", "指标", "metric",
	"metrics", "运维", "oncall", "根因", "服务异常",
}

func ShouldUseMultiAgentForChat(ctx context.Context, query string) bool {
	v, err := g.Cfg().Get(ctx, "multi_agent.chat_route_enabled")
	if err == nil && v.String() != "" && !v.Bool() {
		return false
	}

	lower := strings.ToLower(strings.TrimSpace(query))
	if lower == "" {
		return false
	}
	for _, keyword := range chatMultiAgentKeywords {
		if strings.Contains(lower, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func RunChatMultiAgent(ctx context.Context, sessionID, query string) (ExecutionResponse, error) {
	if decision := GetDegradationDecision(ctx, "chat"); decision.Enabled {
		return NewDegradedExecutionResponse(decision), nil
	}
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, sessionID)

	rt, err := getOrCreateAIOpsRuntime(ctx)
	if err != nil {
		return ExecutionResponse{}, err
	}

	memorySvc := newMemoryService()
	memoryContext, refs, contextDetail := memorySvc.BuildContextPlan(ctx, "chat_multi_agent", sessionID, query)

	rootTask := protocol.NewRootTask(sessionID, query, supervisor.AgentName)
	rootTask.Input = map[string]any{
		"raw_query":      query,
		"memory_context": memoryContext,
		"context_detail": contextDetail,
		"response_mode":  "chat",
		"entrypoint":     "chat",
	}
	rootTask.MemoryRefs = refs

	result, err := rt.Dispatch(ctx, rootTask)
	detail := append([]string{}, contextDetail...)
	detail = append(detail, rt.DetailMessages(ctx, rootTask.TraceID)...)
	if result == nil {
		return ExecutionResponse{
			Detail:  detail,
			TraceID: rootTask.TraceID,
		}, err
	}

	memorySvc.PersistOutcome(ctx, sessionID, query, result.Summary)
	return ExecutionResponseFromResult(result, detail, rootTask.TraceID), err
}
