package service

import (
	"context"
	"testing"

	"SuperBizAgent/internal/ai/agent/supervisor"
	"SuperBizAgent/internal/ai/contextengine"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

type fakeTraceAgent struct{}

func (a *fakeTraceAgent) Name() string {
	return "fake-supervisor"
}

func (a *fakeTraceAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *fakeTraceAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    "ok",
		Confidence: 1,
	}, nil
}

type captureSupervisorAgent struct {
	lastTask *protocol.TaskEnvelope
}

func (a *captureSupervisorAgent) Name() string {
	return supervisor.AgentName
}

func (a *captureSupervisorAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *captureSupervisorAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	a.lastTask = task
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    "ok",
		Confidence: 1,
	}, nil
}

type stubAIOpsMemory struct {
	sessionID       string
	memoryContext   string
	refs            []protocol.MemoryRef
	contextDetail   []string
	persistedQuery  string
	persistedResult string
}

func (s *stubAIOpsMemory) ResolveSessionID(context.Context) string {
	return s.sessionID
}

func (s *stubAIOpsMemory) BuildContextPlan(context.Context, string, string, string) (string, []protocol.MemoryRef, []string) {
	return s.memoryContext, s.refs, s.contextDetail
}

func (s *stubAIOpsMemory) PersistOutcome(_ context.Context, _ string, query, summary string) {
	s.persistedQuery = query
	s.persistedResult = summary
}

func TestGetOrCreateAIOpsRuntimeReusesInstance(t *testing.T) {
	oldFactory := newPersistentRuntime
	oldRuntimes := aiOpsRuntimes
	defer func() {
		newPersistentRuntime = oldFactory
		aiOpsRuntimes = oldRuntimes
	}()

	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	created := 0
	newPersistentRuntime = func(baseDir string) (*runtime.Runtime, error) {
		created++
		return runtime.NewPersistent(baseDir)
	}

	dir := t.TempDir()
	first, err := getOrCreateAIOpsRuntimeForDir(dir)
	if err != nil {
		t.Fatalf("first runtime: %v", err)
	}
	second, err := getOrCreateAIOpsRuntimeForDir(dir)
	if err != nil {
		t.Fatalf("second runtime: %v", err)
	}
	if first != second {
		t.Fatal("expected runtime instance reuse for same data dir")
	}
	if created != 1 {
		t.Fatalf("expected constructor to run once, got %d", created)
	}
}

func TestGetAIOpsTraceReadsPersistedTrace(t *testing.T) {
	oldRuntimes := aiOpsRuntimes
	defer func() {
		aiOpsRuntimes = oldRuntimes
	}()
	aiOpsRuntimes = make(map[string]*runtime.Runtime)

	dir := t.TempDir()
	rt, err := runtime.NewPersistent(dir)
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	if err := rt.Register(&fakeTraceAgent{}); err != nil {
		t.Fatalf("register fake trace agent: %v", err)
	}
	aiOpsRuntimes[dir] = rt

	task := protocol.NewRootTask("session-trace", "trace test", "fake-supervisor")
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}

	events, detail, err := getAIOpsTraceForDir(context.Background(), dir, task.TraceID)
	if err != nil {
		t.Fatalf("trace lookup: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected trace events, got %d", len(events))
	}
	if len(detail) < 2 {
		t.Fatalf("expected detail lines, got %d", len(detail))
	}
}

func TestRunAIOpsMultiAgentApprovalDenialReturnsReason(t *testing.T) {
	result, detail, traceID, err := RunAIOpsMultiAgent(context.Background(), "请删除生产环境中的历史记录")
	if err != nil {
		t.Fatalf("run aiops multi-agent: %v", err)
	}
	if traceID != "" {
		t.Fatalf("expected no trace id for denied request, got %q", traceID)
	}
	if result == "" {
		t.Fatal("expected denial reason in result")
	}
	if len(detail) == 0 || detail[0] != result {
		t.Fatalf("expected detail to include denial reason, got result=%q detail=%v", result, detail)
	}
}

func TestRunAIOpsMultiAgentKeepsRawQueryForRouting(t *testing.T) {
	oldFactory := newPersistentRuntime
	oldRegister := registerAIOpsAgentsFn
	oldMemoryFactory := newMemoryService
	oldRuntimes := aiOpsRuntimes
	defer func() {
		newPersistentRuntime = oldFactory
		registerAIOpsAgentsFn = oldRegister
		newMemoryService = oldMemoryFactory
		aiOpsRuntimes = oldRuntimes
	}()

	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	supervisorAgent := &captureSupervisorAgent{}
	memorySvc := &stubAIOpsMemory{
		sessionID:     "session-aiops",
		memoryContext: "- [fact] 最近支付服务有超时历史",
		contextDetail: []string{"context profile=aiops-default"},
		refs: []protocol.MemoryRef{
			{ID: "mem-1", Type: "fact"},
		},
	}

	newPersistentRuntime = func(string) (*runtime.Runtime, error) {
		return runtime.New(), nil
	}
	registerAIOpsAgentsFn = func(rt *runtime.Runtime) error {
		return rt.Register(supervisorAgent)
	}
	newMemoryService = func() aiOpsMemory {
		return memorySvc
	}

	query := "请查询支付告警的 SOP"
	result, _, _, err := RunAIOpsMultiAgent(context.Background(), query)
	if err != nil {
		t.Fatalf("run aiops multi-agent: %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected result ok, got %q", result)
	}
	if supervisorAgent.lastTask == nil {
		t.Fatal("expected supervisor to receive root task")
	}
	if supervisorAgent.lastTask.Goal != query {
		t.Fatalf("expected raw query for routing, got %q", supervisorAgent.lastTask.Goal)
	}
	if got, _ := supervisorAgent.lastTask.Input["memory_context"].(string); got != memorySvc.memoryContext {
		t.Fatalf("expected memory context in task input, got %q", got)
	}
	if len(supervisorAgent.lastTask.MemoryRefs) != 1 {
		t.Fatalf("expected memory refs to be preserved, got %v", supervisorAgent.lastTask.MemoryRefs)
	}
	if memorySvc.persistedQuery != query {
		t.Fatalf("expected raw query to persist, got %q", memorySvc.persistedQuery)
	}
}

func TestRunAIOpsMultiAgentPrependsContextDetail(t *testing.T) {
	oldFactory := newPersistentRuntime
	oldRegister := registerAIOpsAgentsFn
	oldMemoryFactory := newMemoryService
	oldRuntimes := aiOpsRuntimes
	defer func() {
		newPersistentRuntime = oldFactory
		registerAIOpsAgentsFn = oldRegister
		newMemoryService = oldMemoryFactory
		aiOpsRuntimes = oldRuntimes
	}()

	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	newPersistentRuntime = func(string) (*runtime.Runtime, error) {
		return runtime.New(), nil
	}
	registerAIOpsAgentsFn = func(rt *runtime.Runtime) error {
		return rt.Register(&captureSupervisorAgent{})
	}
	newMemoryService = func() aiOpsMemory {
		return &stubAIOpsMemory{
			sessionID:     "sess-aiops",
			contextDetail: contextengine.TraceDetails(contextengine.ContextAssemblyTrace{Profile: "aiops-default"}),
		}
	}

	_, detail, _, err := RunAIOpsMultiAgent(context.Background(), "请分析当前告警")
	if err != nil {
		t.Fatalf("run aiops multi-agent: %v", err)
	}
	if len(detail) == 0 || detail[0] != "context profile=aiops-default" {
		t.Fatalf("expected context detail to lead response detail, got %v", detail)
	}
}
