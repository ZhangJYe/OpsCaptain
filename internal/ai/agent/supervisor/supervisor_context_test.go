package supervisor

import (
	"context"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/agent/reporter"
	"SuperBizAgent/internal/ai/agent/skillspecialists/knowledge"
	"SuperBizAgent/internal/ai/agent/skillspecialists/logs"
	"SuperBizAgent/internal/ai/agent/triage"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
)

type captureTriageAgent struct {
	lastGoal string
	metadata map[string]any
}

func (a *captureTriageAgent) Name() string {
	return triage.AgentName
}

func (a *captureTriageAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *captureTriageAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	a.lastGoal = task.Goal
	metadata := a.metadata
	if metadata == nil {
		metadata = map[string]any{
			"intent":  "kb_qa",
			"domains": []string{"knowledge"},
		}
	}
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.Name(),
		Status:     protocol.ResultStatusSucceeded,
		Summary:    "triaged",
		Confidence: 1,
		Metadata:   metadata,
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

type captureSpecialistAgent struct {
	name      string
	lastPrior []string
}

func (a *captureSpecialistAgent) Name() string {
	return a.name
}

func (a *captureSpecialistAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *captureSpecialistAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	prior, _ := task.Input["prior_results"].([]string)
	a.lastPrior = append([]string(nil), prior...)
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      a.name,
		Status:     protocol.ResultStatusSucceeded,
		Summary:    a.name + " ok",
		Confidence: 1,
	}, nil
}

type fakeReporterAgent struct {
	resultsLen int
}

func (a *fakeReporterAgent) Name() string {
	return reporter.AgentName
}

func (a *fakeReporterAgent) Capabilities() []string {
	return []string{"test"}
}

func (a *fakeReporterAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	raw, _ := task.Input["results"].([]*protocol.TaskResult)
	a.resultsLen = len(raw)
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

func TestSupervisorHonorsTriageUseMultiAgentFalse(t *testing.T) {
	rt := runtime.New()
	supervisorAgent := New()
	triageAgent := &captureTriageAgent{
		metadata: map[string]any{
			"intent":          "kb_qa",
			"domains":         []string{},
			"use_multi_agent": false,
			"triage_source":   "llm",
			"triage_fallback": false,
		},
	}
	reporterAgent := &fakeReporterAgent{}

	for _, agent := range []runtime.Agent{
		supervisorAgent,
		triageAgent,
		reporterAgent,
	} {
		if err := rt.Register(agent); err != nil {
			t.Fatalf("register %s: %v", agent.Name(), err)
		}
	}

	task := protocol.NewRootTask("session-test", "你好，介绍一下你自己", AgentName)
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if reporterAgent.resultsLen != 0 {
		t.Fatalf("expected no specialist fan-out, got %d results", reporterAgent.resultsLen)
	}
	if result.Metadata["use_multi_agent"] != false {
		t.Fatalf("expected use_multi_agent=false metadata, got %#v", result.Metadata)
	}
	if result.Metadata["triage_source"] != "llm" || result.Metadata["triage_fallback"] != false {
		t.Fatalf("expected triage metadata to be propagated, got %#v", result.Metadata)
	}
	domains, _ := result.Metadata["domains"].([]string)
	if len(domains) != 0 {
		t.Fatalf("expected empty routed domains, got %#v", domains)
	}
}

func TestSupervisorStagedExecutionPassesPriorResults(t *testing.T) {
	oldMode := supervisorExecutionMode
	oldReflect := supervisorSelfReflectEnabled
	defer func() {
		supervisorExecutionMode = oldMode
		supervisorSelfReflectEnabled = oldReflect
	}()
	supervisorExecutionMode = func(context.Context) string { return "staged" }
	supervisorSelfReflectEnabled = func(context.Context) bool { return true }

	rt := runtime.New()
	triageAgent := &captureTriageAgent{
		metadata: map[string]any{
			"intent":  "incident_analysis",
			"domains": []string{"logs", "knowledge"},
		},
	}
	logsAgent := &captureSpecialistAgent{name: logs.AgentName}
	knowledgeAgent := &captureSpecialistAgent{name: knowledge.AgentName}
	reporterAgent := &fakeReporterAgent{}

	for _, agent := range []runtime.Agent{
		New(),
		triageAgent,
		logsAgent,
		knowledgeAgent,
		reporterAgent,
	} {
		if err := rt.Register(agent); err != nil {
			t.Fatalf("register %s: %v", agent.Name(), err)
		}
	}

	task := protocol.NewRootTask("session-test", "排查 checkout 504", AgentName)
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.Metadata["execution_mode"] != "staged" {
		t.Fatalf("expected staged metadata, got %#v", result.Metadata)
	}
	if len(logsAgent.lastPrior) != 0 {
		t.Fatalf("expected first specialist to have no prior results, got %#v", logsAgent.lastPrior)
	}
	if len(knowledgeAgent.lastPrior) != 1 || !strings.Contains(knowledgeAgent.lastPrior[0], logs.AgentName) {
		t.Fatalf("expected knowledge to receive logs prior result, got %#v", knowledgeAgent.lastPrior)
	}
	if result.Metadata["reflection_status"] != "complete" {
		t.Fatalf("expected complete reflection, got %#v", result.Metadata)
	}
}
