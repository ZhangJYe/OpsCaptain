package service

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/skillspecialists/metrics"
	"SuperBizAgent/internal/ai/agent/supervisor"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"

	"github.com/gogf/gf/v2/frame/g"
)

var (
	aiOpsRuntimeMu        sync.Mutex
	aiOpsRuntimes         = make(map[string]*runtime.Runtime)
	newPersistentRuntime  = runtime.NewPersistent
	registerAIOpsAgentsFn = registerAIOpsAgents
	newMemoryService      = func() aiOpsMemory {
		return NewMemoryService()
	}
)

type aiOpsMemory interface {
	ResolveSessionID(ctx context.Context) string
	BuildContextPlan(ctx context.Context, mode, sessionID, query string) (string, []protocol.MemoryRef, []string)
	PersistOutcome(ctx context.Context, sessionID, query, summary string)
}

func RunAIOpsMultiAgent(ctx context.Context, query string) (string, []string, string, error) {
	approval := NewApprovalGate()
	if decision := approval.Check(ctx, query); !decision.Approved {
		return decision.Reason, []string{decision.Reason}, "", nil
	}

	rt, err := getOrCreateAIOpsRuntime(ctx)
	if err != nil {
		return "", nil, "", err
	}

	memorySvc := newMemoryService()
	sessionID := memorySvc.ResolveSessionID(ctx)
	memoryContext, refs, contextDetail := memorySvc.BuildContextPlan(ctx, "aiops", sessionID, query)

	rootTask := protocol.NewRootTask(sessionID, query, supervisor.AgentName)
	if memoryContext != "" || len(refs) > 0 || len(contextDetail) > 0 {
		rootTask.Input = map[string]any{
			"raw_query":      query,
			"memory_context": memoryContext,
			"context_detail": contextDetail,
		}
	}
	rootTask.MemoryRefs = refs
	result, err := rt.Dispatch(ctx, rootTask)
	detail := append([]string{}, contextDetail...)
	detail = append(detail, rt.DetailMessages(ctx, rootTask.TraceID)...)
	if result == nil {
		return "", detail, rootTask.TraceID, err
	}
	memorySvc.PersistOutcome(ctx, sessionID, query, result.Summary)
	return result.Summary, detail, rootTask.TraceID, err
}

func GetAIOpsTrace(ctx context.Context, traceID string) ([]*protocol.TaskEvent, []string, error) {
	return getAIOpsTraceForDir(ctx, runtimeDataDir(ctx), traceID)
}

func getAIOpsTraceForDir(ctx context.Context, dataDir, traceID string) ([]*protocol.TaskEvent, []string, error) {
	if traceID == "" {
		return nil, nil, fmt.Errorf("traceID is empty")
	}
	rt, err := getOrCreateAIOpsRuntimeForDir(dataDir)
	if err != nil {
		return nil, nil, err
	}
	events, err := rt.TraceEvents(ctx, traceID)
	if err != nil {
		return nil, nil, err
	}
	if len(events) == 0 {
		return nil, nil, fmt.Errorf("trace %s not found", traceID)
	}
	return events, rt.DetailMessages(ctx, traceID), nil
}

func getOrCreateAIOpsRuntime(ctx context.Context) (*runtime.Runtime, error) {
	return getOrCreateAIOpsRuntimeForDir(runtimeDataDir(ctx))
}

func getOrCreateAIOpsRuntimeForDir(dataDir string) (*runtime.Runtime, error) {
	aiOpsRuntimeMu.Lock()
	defer aiOpsRuntimeMu.Unlock()

	if rt, ok := aiOpsRuntimes[dataDir]; ok {
		return rt, nil
	}

	rt, err := newPersistentRuntime(dataDir)
	if err != nil {
		return nil, err
	}
	if err := registerAIOpsAgentsFn(rt); err != nil {
		return nil, err
	}
	aiOpsRuntimes[dataDir] = rt
	return rt, nil
}

func registerAIOpsAgents(rt *runtime.Runtime) error {
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

func runtimeDataDir(ctx context.Context) string {
	v, err := g.Cfg().Get(ctx, "multi_agent.data_dir")
	if err == nil && v.String() != "" {
		return v.String()
	}
	return filepath.Join(".", "var", "runtime")
}
