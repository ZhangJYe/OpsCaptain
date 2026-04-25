package service

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/ai/agent/supervisor"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/consts"

	"github.com/gogf/gf/v2/frame/g"
)

var (
	chatRuntimeMu        sync.Mutex
	chatRuntimes         = make(map[string]*runtime.Runtime)
	newPersistentRuntime = runtime.NewPersistent
	registerChatAgentsFn = registerChatAgents
	multiAgentConfigBool = func(ctx context.Context, key string) (bool, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return false, false
		}
		return v.Bool(), true
	}
)

var chatMultiAgentKeywords = []string{
	"告警", "alert", "prometheus", "日志", "log", "排查", "故障", "incident",
	"知识库", "文档", "sop", "runbook", "mysql", "sql", "数据库", "指标", "metric",
	"metrics", "运维", "oncall", "根因", "服务异常",
}

func ShouldUseMultiAgentForChat(ctx context.Context, query string) bool {
	if !chatMultiAgentEnabled(ctx) {
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
	if !chatMultiAgentEnabled(ctx) {
		return multiAgentDisabledResponse(), nil
	}
	if decision := GetDegradationDecision(ctx, "chat"); decision.Enabled {
		return NewDegradedExecutionResponse(decision), nil
	}
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, sessionID)

	rt, err := getOrCreateChatRuntime(ctx)
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

func getOrCreateChatRuntime(ctx context.Context) (*runtime.Runtime, error) {
	dataDir := chatRuntimeDataDir(ctx)
	chatRuntimeMu.Lock()
	defer chatRuntimeMu.Unlock()

	if rt, ok := chatRuntimes[dataDir]; ok {
		return rt, nil
	}

	rt, err := newPersistentRuntime(dataDir)
	if err != nil {
		return nil, err
	}
	if err := registerChatAgentsFn(rt); err != nil {
		return nil, err
	}
	chatRuntimes[dataDir] = rt
	return rt, nil
}

func registerChatAgents(rt *runtime.Runtime) error {
	for _, agent := range []runtime.Agent{
		supervisor.New(),
		triage.New(),
		metrics.New(),
		logs.New(),
		knowledge.New(),
		reporter.New(),
	} {
		if err := rt.Register(agent); err != nil {
			return err
		}
	}
	return nil
}

func chatRuntimeDataDir(ctx context.Context) string {
	v, err := g.Cfg().Get(ctx, "multi_agent.data_dir")
	if err == nil && v.String() != "" {
		return v.String()
	}
	return filepath.Join(".", "var", "runtime")
}

func chatMultiAgentEnabled(ctx context.Context) bool {
	if !multiAgentEnabled(ctx) {
		return false
	}
	enabled, configured := multiAgentConfigBool(ctx, "multi_agent.chat_route_enabled")
	if configured {
		return enabled
	}
	return true
}

func aiOpsMultiAgentEnabled(ctx context.Context) bool {
	if !multiAgentEnabled(ctx) {
		return false
	}
	enabled, configured := multiAgentConfigBool(ctx, "multi_agent.ai_ops_enabled")
	if configured {
		return enabled
	}
	return true
}

func multiAgentEnabled(ctx context.Context) bool {
	enabled, configured := multiAgentConfigBool(ctx, "multi_agent.enabled")
	if configured {
		return enabled
	}
	return true
}

func multiAgentDisabledResponse() ExecutionResponse {
	return ExecutionResponse{
		Content:           "Multi-agent runtime is disabled; use the Chat/RAG baseline route.",
		Detail:            []string{"multi_agent.enabled=false"},
		Status:            protocol.ResultStatusDegraded,
		DegradationReason: "multi_agent_disabled",
	}
}
