package supervisor

import (
	"context"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/specialists/knowledge"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

type captureTriageAgent struct {
	lastGoal string
}

func (a *captureTriageAgent) Name() string {
	return triage.AgentName
}

func (a *captureTriageAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *captureTriageAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	a.lastGoal = task.Goal
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    "triaged",
		Confidence: 1,
		Metadata: map[string]any{
			"intent":  "kb_qa",
			"domains": []string{"knowledge"},
		},
	}, nil
}

type captureKnowledgeAgent struct {
	lastGoal string
}

func (a *captureKnowledgeAgent) Name() string {
	return knowledge.AgentName
}

func (a *captureKnowledgeAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *captureKnowledgeAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	a.lastGoal = task.Goal
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    "knowledge ok",
		Confidence: 1,
	}, nil
}

type fakeReporterAgent struct{}

func (a *fakeReporterAgent) Name() string {
	return reporter.AgentName
}

func (a *fakeReporterAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *fakeReporterAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    "final report",
		Confidence: 1,
	}, nil
}

func TestSupervisorRoutesOnRawQueryButPassesMemoryToSpecialists(t *testing.T) {
	rt := runtime.New()
	supervisorAgent := New()
	triageAgent := &captureTriageAgent{}
	knowledgeAgent := &captureKnowledgeAgent{}

	for _, agent := range []runtime.Agent{
		supervisorAgent,
		triageAgent,
		knowledgeAgent,
		&fakeReporterAgent{},
	} {
		if err := rt.Register(agent); err != nil {
			t.Fatalf("register %s: %v", agent.Name(), err)
		}
	}

	task := protocol.NewRootTask("session-test", "请查询知识库中的 SOP 文档", AgentName)
	task.Input = map[string]any{
		"memory_context": "- [fact] 支付服务最近出现超时历史",
	}
	task.MemoryRefs = []protocol.MemoryRef{{ID: "mem-1", Type: "fact"}}

	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil || result.Summary == "" {
		t.Fatalf("expected result, got %#v", result)
	}
	if triageAgent.lastGoal != task.Goal {
		t.Fatalf("expected triage to see raw query %q, got %q", task.Goal, triageAgent.lastGoal)
	}
	if knowledgeAgent.lastGoal == task.Goal {
		t.Fatalf("expected specialist query to include memory context, got raw query %q", knowledgeAgent.lastGoal)
	}
	if !strings.Contains(knowledgeAgent.lastGoal, "可参考的历史上下文") {
		t.Fatalf("expected specialist query to include memory section, got %q", knowledgeAgent.lastGoal)
	}
}
