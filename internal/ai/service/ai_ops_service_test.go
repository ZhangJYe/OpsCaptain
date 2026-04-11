package service

import (
	"context"
	"strings"
	"testing"

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

func TestRunAIOpsMultiAgentApprovalDenialReturnsReason(t *testing.T) {
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

func TestRunAIOpsCallsBuildPlanAgent(t *testing.T) {
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
		return "analysis complete", []string{"step1: queried alerts", "step2: found root cause"}, nil
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
}

func TestRunAIOpsWithEmptyMemoryContext(t *testing.T) {
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

func TestGetAIOpsTraceReturnsRuntimeEvents(t *testing.T) {
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
