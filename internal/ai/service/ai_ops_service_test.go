package service

import (
	"context"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/agent/experts"
	"SuperBizAgent/internal/ai/agent/gos_engine"
	"SuperBizAgent/internal/ai/belief"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
	"SuperBizAgent/internal/consts"
)

type stubAIOpsMemory struct {
	sessionID       string
	memoryContext   string
	refs            []protocol.MemoryRef
	contextDetail   []string
	persistedQuery  string
	persistedResult string
}

func (s *stubAIOpsMemory) ResolveSessionID(context.Context) string { return s.sessionID }
func (s *stubAIOpsMemory) BuildContextPlan(context.Context, string, string, string) (string, []protocol.MemoryRef, []string) {
	return s.memoryContext, s.refs, s.contextDetail
}
func (s *stubAIOpsMemory) PersistOutcome(_ context.Context, _ string, query, summary string) {
	s.persistedQuery = query
	s.persistedResult = summary
}

func enableMultiAgentForTest(t *testing.T) {
	t.Helper()
	oldConfigBool := multiAgentConfigBool
	multiAgentConfigBool = func(context.Context, string) (bool, bool) {
		return true, true
	}
	t.Cleanup(func() {
		multiAgentConfigBool = oldConfigBool
	})
}

func TestRunAIOpsMultiAgentApprovalDenialReturnsReason(t *testing.T) {
	enableMultiAgentForTest(t)
	response, err := RunAIOpsMultiAgent(context.Background(), "delete production history")
	if err != nil {
		t.Fatalf("run aiops: %v", err)
	}
	if response.Content == "" {
		t.Fatal("expected denial reason in result")
	}
	if len(response.Detail) == 0 || response.Detail[0] != response.Content {
		t.Fatalf("expected detail to include denial reason, got result=%q detail=%v", response.Content, response.Detail)
	}
}

func TestRunAIOpsMultiAgentDisabledByConfig(t *testing.T) {
	oldConfigBool := multiAgentConfigBool
	multiAgentConfigBool = func(context.Context, string) (bool, bool) {
		return false, true
	}
	t.Cleanup(func() {
		multiAgentConfigBool = oldConfigBool
	})

	response, err := RunAIOpsMultiAgent(context.Background(), "check alerts")
	if err != nil {
		t.Fatalf("run aiops: %v", err)
	}
	if response.Status != protocol.ResultStatusDegraded || response.DegradationReason != "aiops_disabled" {
		t.Fatalf("expected disabled degraded response, got %+v", response)
	}
}

func TestSelectAIOpsAgentNameDefaultsToPlan(t *testing.T) {
	oldString := aiOpsConfigString
	oldBool := aiOpsConfigBool
	aiOpsConfigString = func(context.Context, string) (string, bool) { return "", false }
	aiOpsConfigBool = func(context.Context, string) (bool, bool) { return false, false }
	t.Cleanup(func() {
		aiOpsConfigString = oldString
		aiOpsConfigBool = oldBool
	})

	if got := selectAIOpsAgentName(context.Background()); got != aiOpsPlanAgentName {
		t.Fatalf("expected default agent %q, got %q", aiOpsPlanAgentName, got)
	}
}

func TestSelectAIOpsAgentNameRequiresEnabledGOS(t *testing.T) {
	oldString := aiOpsConfigString
	oldBool := aiOpsConfigBool
	aiOpsConfigString = func(_ context.Context, key string) (string, bool) {
		if key == "aiops.engine" {
			return "gos_engine", true
		}
		return "", false
	}
	aiOpsConfigBool = func(_ context.Context, key string) (bool, bool) {
		if key == "aiops.gos.enabled" {
			return false, true
		}
		return false, false
	}
	t.Cleanup(func() {
		aiOpsConfigString = oldString
		aiOpsConfigBool = oldBool
	})

	if got := selectAIOpsAgentName(context.Background()); got != aiOpsPlanAgentName {
		t.Fatalf("expected disabled gos to fall back to %q, got %q", aiOpsPlanAgentName, got)
	}
}

func TestSelectAIOpsAgentNameUsesGOSWhenEnabled(t *testing.T) {
	oldString := aiOpsConfigString
	oldBool := aiOpsConfigBool
	aiOpsConfigString = func(_ context.Context, key string) (string, bool) {
		if key == "aiops.engine" {
			return "gos_engine", true
		}
		return "", false
	}
	aiOpsConfigBool = func(_ context.Context, key string) (bool, bool) {
		if key == "aiops.gos.enabled" {
			return true, true
		}
		return false, false
	}
	t.Cleanup(func() {
		aiOpsConfigString = oldString
		aiOpsConfigBool = oldBool
	})

	if got := selectAIOpsAgentName(context.Background()); got != aiOpsGOSAgentName {
		t.Fatalf("expected gos agent %q, got %q", aiOpsGOSAgentName, got)
	}
}

func TestSelectAIOpsAgentNameUsesRequestOverride(t *testing.T) {
	oldString := aiOpsConfigString
	oldBool := aiOpsConfigBool
	aiOpsConfigString = func(_ context.Context, key string) (string, bool) {
		if key == "aiops.engine" {
			return "plan_execute_replan", true
		}
		return "", false
	}
	aiOpsConfigBool = func(_ context.Context, key string) (bool, bool) {
		if key == "aiops.gos.enabled" {
			return true, true
		}
		return false, false
	}
	t.Cleanup(func() {
		aiOpsConfigString = oldString
		aiOpsConfigBool = oldBool
	})

	ctx := WithAIOpsEngine(context.Background(), "gos_engine")
	if got := selectAIOpsAgentName(ctx); got != aiOpsGOSAgentName {
		t.Fatalf("expected request override to use gos agent %q, got %q", aiOpsGOSAgentName, got)
	}
}

func TestResolveAIOpsAgentNameRejectsExplicitDisabledGOS(t *testing.T) {
	oldString := aiOpsConfigString
	oldBool := aiOpsConfigBool
	aiOpsConfigString = func(_ context.Context, key string) (string, bool) {
		if key == "aiops.engine" {
			return "plan_execute_replan", true
		}
		return "", false
	}
	aiOpsConfigBool = func(_ context.Context, key string) (bool, bool) {
		if key == "aiops.gos.enabled" {
			return false, true
		}
		return false, false
	}
	t.Cleanup(func() {
		aiOpsConfigString = oldString
		aiOpsConfigBool = oldBool
	})

	agentName, available, reason := resolveAIOpsAgentName(WithAIOpsEngine(context.Background(), "gos_engine"))
	if agentName != aiOpsGOSAgentName {
		t.Fatalf("expected gos agent %q, got %q", aiOpsGOSAgentName, agentName)
	}
	if available {
		t.Fatal("expected explicit disabled gos request to be unavailable")
	}
	if !strings.Contains(reason, "GoS") {
		t.Fatalf("expected user-facing reason, got %q", reason)
	}
}

func TestRunAIOpsAsyncGOSDisabledReturnsEngine(t *testing.T) {
	enableMultiAgentForTest(t)
	oldString := aiOpsConfigString
	oldBool := aiOpsConfigBool
	aiOpsConfigString = func(_ context.Context, key string) (string, bool) {
		if key == "aiops.engine" {
			return "plan_execute_replan", true
		}
		return "", false
	}
	aiOpsConfigBool = func(_ context.Context, key string) (bool, bool) {
		if key == "aiops.gos.enabled" {
			return false, true
		}
		return false, false
	}
	t.Cleanup(func() {
		aiOpsConfigString = oldString
		aiOpsConfigBool = oldBool
	})

	info, err := RunAIOpsAsync(WithAIOpsEngine(context.Background(), "gos_engine"), "check alerts")
	if err != nil {
		t.Fatalf("run async: %v", err)
	}
	if info.Status != "degraded" || !info.Degraded {
		t.Fatalf("expected degraded run info, got %+v", info)
	}
	if info.Engine != aiOpsGOSAgentName {
		t.Fatalf("expected engine %q, got %q", aiOpsGOSAgentName, info.Engine)
	}
	if !strings.Contains(info.DegradationReason, "GoS") {
		t.Fatalf("expected GoS reason, got %q", info.DegradationReason)
	}
}

func TestRunAIOpsCallsBuildPlanAgent(t *testing.T) {
	enableMultiAgentForTest(t)
	oldBuild := buildPlanAgent
	oldMemoryFactory := newMemoryService
	oldCfgBool := degradationConfigBool
	oldCfgString := degradationConfigString
	oldFactory := newPersistentRuntime
	oldAIOpsRuntimes := aiOpsRuntimes
	defer func() {
		buildPlanAgent = oldBuild
		newMemoryService = oldMemoryFactory
		degradationConfigBool = oldCfgBool
		degradationConfigString = oldCfgString
		newPersistentRuntime = oldFactory
		aiOpsRuntimes = oldAIOpsRuntimes
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(context.Context, string) string { return "" }
	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	newPersistentRuntime = func(string) (*runtime.Runtime, error) { return runtime.New(), nil }

	var capturedQuery string
	buildPlanAgent = func(_ context.Context, query string) (string, []string, error) {
		capturedQuery = query
		return "analysis complete", []string{
			"assistant: checking alerts\ntool_calls:\nindex[0]:{Type:function Function:{Name:query_prometheus_alerts Arguments:{}}}",
			`tool: {"success":true,"alerts":[{"alert_name":"HighLatency"}]}`,
		}, nil
	}

	memorySvc := &stubAIOpsMemory{
		sessionID:     "session-plan",
		memoryContext: "- [fact] recent payment timeout",
		contextDetail: []string{"context profile=aiops"},
	}
	newMemoryService = func() aiOpsMemory { return memorySvc }

	query := "analyze current alerts"
	response, err := RunAIOpsMultiAgent(context.Background(), query)
	if err != nil {
		t.Fatalf("run aiops: %v", err)
	}
	if response.Content != "analysis complete" {
		t.Fatalf("expected content 'analysis complete', got %q", response.Content)
	}
	if response.TraceID == "" {
		t.Fatal("expected trace id to be set")
	}
	if response.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %q", response.Status)
	}
	if capturedQuery == query {
		t.Fatal("expected enriched query with memory context, got raw query")
	}
	if memorySvc.persistedQuery != query {
		t.Fatalf("expected raw query to persist, got %q", memorySvc.persistedQuery)
	}
	if memorySvc.persistedResult != "analysis complete" {
		t.Fatalf("expected result to persist, got %q", memorySvc.persistedResult)
	}
	if len(response.Detail) < 3 {
		t.Fatalf("expected context detail + plan detail, got %v", response.Detail)
	}
	if len(response.Evidence) != 1 {
		t.Fatalf("expected one structured tool evidence item, got %+v", response.Evidence)
	}
	if response.Evidence[0].Title != "query_prometheus_alerts 工具结果" || !strings.Contains(response.Evidence[0].Snippet, "HighLatency") {
		t.Fatalf("unexpected structured evidence: %+v", response.Evidence[0])
	}
}

func TestRunAIOpsWithEmptyMemoryContext(t *testing.T) {
	enableMultiAgentForTest(t)
	oldBuild := buildPlanAgent
	oldMemoryFactory := newMemoryService
	oldCfgBool := degradationConfigBool
	oldCfgString := degradationConfigString
	oldFactory := newPersistentRuntime
	oldAIOpsRuntimes := aiOpsRuntimes
	defer func() {
		buildPlanAgent = oldBuild
		newMemoryService = oldMemoryFactory
		degradationConfigBool = oldCfgBool
		degradationConfigString = oldCfgString
		newPersistentRuntime = oldFactory
		aiOpsRuntimes = oldAIOpsRuntimes
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(context.Context, string) string { return "" }
	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	newPersistentRuntime = func(string) (*runtime.Runtime, error) { return runtime.New(), nil }

	var capturedQuery string
	buildPlanAgent = func(_ context.Context, query string) (string, []string, error) {
		capturedQuery = query
		return "done", nil, nil
	}
	newMemoryService = func() aiOpsMemory {
		return &stubAIOpsMemory{sessionID: "sess-empty"}
	}

	query := "check alerts"
	_, err := RunAIOpsMultiAgent(context.Background(), query)
	if err != nil {
		t.Fatalf("run aiops: %v", err)
	}
	if !strings.HasPrefix(capturedQuery, query) {
		t.Fatalf("expected query to start with %q, got %q", query, capturedQuery)
	}
	if strings.Contains(capturedQuery, "历史上下文") {
		t.Fatalf("expected no memory context, got %q", capturedQuery)
	}
}

type serviceTestGOSLogger struct{}

func (serviceTestGOSLogger) Info(string, ...interface{})  {}
func (serviceTestGOSLogger) Error(string, ...interface{}) {}

type serviceTestGOSExpert struct{}

func (serviceTestGOSExpert) Name() string {
	return "linux_sre"
}

func (serviceTestGOSExpert) Run(_ context.Context, frontier *belief.Frontier, _ *belief.BeliefGraph) *experts.ExpertAnalysis {
	return &experts.ExpertAnalysis{
		ExpertName: "linux_sre",
		Status:     "succeeded",
		Analysis:   "GoS analysis complete",
		Confidence: 0.9,
		Evidence: []experts.EvidenceItem{{
			SourceType:         "test",
			SourceID:           "evidence-1",
			Title:              "evidence",
			Snippet:            "supporting evidence",
			Score:              1,
			Relation:           experts.EvidenceRelationSupport,
			TargetHypothesisID: frontier.NodeID,
			Strength:           1,
		}},
	}
}

func TestRunAIOpsUsesGOSWhenEnabled(t *testing.T) {
	enableMultiAgentForTest(t)
	oldBuildGOS := buildAIOpsGoSEngine
	oldMemoryFactory := newMemoryService
	oldCfgBool := degradationConfigBool
	oldCfgString := degradationConfigString
	oldAIOpsString := aiOpsConfigString
	oldAIOpsBool := aiOpsConfigBool
	oldFactory := newPersistentRuntime
	oldAIOpsRuntimes := aiOpsRuntimes
	defer func() {
		buildAIOpsGoSEngine = oldBuildGOS
		newMemoryService = oldMemoryFactory
		degradationConfigBool = oldCfgBool
		degradationConfigString = oldCfgString
		aiOpsConfigString = oldAIOpsString
		aiOpsConfigBool = oldAIOpsBool
		newPersistentRuntime = oldFactory
		aiOpsRuntimes = oldAIOpsRuntimes
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(context.Context, string) string { return "" }
	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	newPersistentRuntime = func(string) (*runtime.Runtime, error) { return runtime.New(), nil }
	newMemoryService = func() aiOpsMemory {
		return &stubAIOpsMemory{sessionID: "gos-session"}
	}
	aiOpsConfigString = func(_ context.Context, key string) (string, bool) {
		if key == "aiops.engine" {
			return "gos_engine", true
		}
		return "", false
	}
	aiOpsConfigBool = func(_ context.Context, key string) (bool, bool) {
		if key == "aiops.gos.enabled" {
			return true, true
		}
		return false, false
	}
	buildAIOpsGoSEngine = func(context.Context) *gos_engine.GoSEngine {
		cfg := gos_engine.DefaultConfig()
		cfg.SessionMaxSteps = 1
		cfg.FSM.GapDelta = 0.1
		cfg.FSM.MinSupport = 1
		cfg.FSM.MaxSteps = 1
		cfg.FSM.MinConfidence = 0.1
		engine := gos_engine.NewGoSEngine(cfg, serviceTestGOSLogger{})
		engine.RegisterExpert("linux_sre", serviceTestGOSExpert{})
		return engine
	}

	response, err := RunAIOpsMultiAgent(context.Background(), "check gos")
	if err != nil {
		t.Fatalf("run aiops: %v", err)
	}
	if response.Status != protocol.ResultStatusSucceeded {
		t.Fatalf("expected succeeded status, got %q", response.Status)
	}
	if !strings.Contains(response.Content, "根因候选：资源耗尽") || !strings.Contains(response.Content, "test:evidence-1") {
		t.Fatalf("expected gos result, got %q", response.Content)
	}
	if response.TraceID == "" {
		t.Fatal("expected trace id to be set")
	}
}

func TestGetAIOpsTraceReturnsRuntimeEvents(t *testing.T) {
	enableMultiAgentForTest(t)
	oldBuild := buildPlanAgent
	oldMemoryFactory := newMemoryService
	oldCfgBool := degradationConfigBool
	oldCfgString := degradationConfigString
	oldFactory := newPersistentRuntime
	oldAIOpsRuntimes := aiOpsRuntimes
	defer func() {
		buildPlanAgent = oldBuild
		newMemoryService = oldMemoryFactory
		degradationConfigBool = oldCfgBool
		degradationConfigString = oldCfgString
		newPersistentRuntime = oldFactory
		aiOpsRuntimes = oldAIOpsRuntimes
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(context.Context, string) string { return "" }
	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	newPersistentRuntime = func(string) (*runtime.Runtime, error) { return runtime.New(), nil }
	newMemoryService = func() aiOpsMemory {
		return &stubAIOpsMemory{sessionID: "trace-session"}
	}
	buildPlanAgent = func(_ context.Context, query string) (string, []string, error) {
		return "analysis complete", []string{"step1", "step2"}, nil
	}

	response, err := RunAIOpsMultiAgent(context.Background(), "check alerts")
	if err != nil {
		t.Fatalf("run aiops: %v", err)
	}
	events, detail, err := GetAIOpsTrace(context.Background(), response.TraceID)
	if err != nil {
		t.Fatalf("get trace: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected trace events")
	}
	if len(detail) == 0 {
		t.Fatal("expected trace detail")
	}
}

func TestApproveQueuedAIOpsRequestRestoresOriginalUserID(t *testing.T) {
	enableMultiAgentForTest(t)
	oldApprove := approveApprovalRequest
	oldMarkExecuted := markApprovalRequestExecuted
	oldBuild := buildPlanAgent
	oldMemoryFactory := newMemoryService
	oldCfgBool := degradationConfigBool
	oldCfgString := degradationConfigString
	oldFactory := newPersistentRuntime
	oldAIOpsRuntimes := aiOpsRuntimes
	defer func() {
		approveApprovalRequest = oldApprove
		markApprovalRequestExecuted = oldMarkExecuted
		buildPlanAgent = oldBuild
		newMemoryService = oldMemoryFactory
		degradationConfigBool = oldCfgBool
		degradationConfigString = oldCfgString
		newPersistentRuntime = oldFactory
		aiOpsRuntimes = oldAIOpsRuntimes
	}()

	degradationConfigBool = func(context.Context, string) bool { return false }
	degradationConfigString = func(context.Context, string) string { return "" }
	aiOpsRuntimes = make(map[string]*runtime.Runtime)
	newPersistentRuntime = func(string) (*runtime.Runtime, error) { return runtime.New(), nil }
	newMemoryService = func() aiOpsMemory {
		return &stubAIOpsMemory{sessionID: "approval-session"}
	}

	approveApprovalRequest = func(context.Context, string, string) (*ApprovalRequest, error) {
		return &ApprovalRequest{
			ID:        "approval-1",
			Query:     "check alerts",
			Status:    ApprovalStatusApproved,
			SessionID: "original-session",
			UserID:    "requester-user",
		}, nil
	}
	markApprovalRequestExecuted = func(context.Context, string, string) error { return nil }

	var capturedUserID string
	buildPlanAgent = func(ctx context.Context, query string) (string, []string, error) {
		capturedUserID, _ = ctx.Value(consts.CtxKeyUserID).(string)
		return "analysis complete", nil, nil
	}

	ctx := context.WithValue(context.Background(), consts.CtxKeyUserID, "reviewer-user")
	response, err := ApproveQueuedAIOpsRequest(ctx, "approval-1")
	if err != nil {
		t.Fatalf("approve queued request: %v", err)
	}
	if capturedUserID != "requester-user" {
		t.Fatalf("expected original requester user id, got %q", capturedUserID)
	}
	if response.ApprovalStatus != string(ApprovalStatusExecuted) {
		t.Fatalf("expected executed approval status, got %q", response.ApprovalStatus)
	}
}

func TestLoadAIOpsGOSConfigLoadsGraphResourcePolicy(t *testing.T) {
	oldInt := aiOpsConfigInt
	defer func() { aiOpsConfigInt = oldInt }()
	values := map[string]int{
		"aiops.gos.graph.checkpoint_interval": 7,
		"aiops.gos.graph.max_nodes":           100,
		"aiops.gos.graph.max_edges":           180,
		"aiops.gos.graph.max_depth":           3,
		"aiops.gos.graph.max_snapshots":       12,
		"aiops.gos.graph.max_deltas":          6,
	}
	aiOpsConfigInt = func(_ context.Context, key string) (int, bool) {
		value, ok := values[key]
		return value, ok
	}

	cfg := loadAIOpsGOSConfig(context.Background())

	if cfg.Graph.CheckpointInterval != 7 || cfg.Graph.MaxNodes != 100 || cfg.Graph.MaxEdges != 180 ||
		cfg.Graph.MaxDepth != 3 || cfg.Graph.MaxSnapshots != 12 || cfg.Graph.MaxDeltas != 6 {
		t.Fatalf("unexpected graph resource policy: %+v", cfg.Graph)
	}
}

func TestLoadAIOpsGOSConfigLoadsStructuredFlowDefaults(t *testing.T) {
	oldBool := aiOpsConfigBool
	defer func() { aiOpsConfigBool = oldBool }()
	values := map[string]bool{
		"aiops.gos.structured_cognition.enabled": true,
		"aiops.gos.state_conversion.enabled":     true,
	}
	aiOpsConfigBool = func(_ context.Context, key string) (bool, bool) {
		value, ok := values[key]
		return value, ok
	}

	cfg := loadAIOpsGOSConfig(context.Background())

	if !cfg.StructuredCognition.Enabled || !cfg.StateConversion.Enabled {
		t.Fatalf("expected structured GoS flow enabled, got structured=%t state_conversion=%t", cfg.StructuredCognition.Enabled, cfg.StateConversion.Enabled)
	}
}

func TestRegisterAIOpsGOSToolsIncludesIndependentMetricEvidence(t *testing.T) {
	registry := experts.NewToolRegistry()

	registerAIOpsGOSTools(registry)

	for _, name := range []string{"query_logs", "query_prometheus_alerts", "query_prometheus_instant", "query_internal_docs"} {
		if _, ok := registry.Get(name); !ok {
			t.Fatalf("expected GoS tool %q to be registered", name)
		}
	}
}
