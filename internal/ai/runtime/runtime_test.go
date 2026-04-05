package runtime

import (
	"context"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
)

type fakeAgent struct {
	name    string
	summary string
}

func (f *fakeAgent) Name() string {
	return f.name
}

func (f *fakeAgent) Capabilities() []string {
	return []string{"test"}
}

func (f *fakeAgent) Handle(_ context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error) {
	summary := f.summary
	if summary == "" {
		summary = "ok"
	}
	return &protocol.TaskResult{
		TaskID:     task.TaskID,
		Agent:      f.name,
		Status:     protocol.ResultStatusSucceeded,
		Summary:    summary,
		Confidence: 1,
	}, nil
}

func TestRuntimeDispatchRecordsDetails(t *testing.T) {
	rt := New()
	if err := rt.Register(&fakeAgent{name: "fake"}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	task := protocol.NewRootTask("session-test", "do something", "fake")
	result, err := rt.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result == nil || result.Summary != "ok" {
		t.Fatalf("unexpected result: %#v", result)
	}

	details := rt.DetailMessages(context.Background(), task.TraceID)
	if len(details) < 2 {
		t.Fatalf("expected runtime to record start/completion details, got %d", len(details))
	}
}

func TestRuntimeDetailMessagesOmitVerboseSummaryBodies(t *testing.T) {
	rt := New()
	longSummary := "# 告警分析报告\n\n## 执行摘要\n这里是一段很长的汇总内容，用于模拟 reporter/supervisor 生成的完整报告。"
	if err := rt.Register(&fakeAgent{name: "reporter", summary: longSummary}); err != nil {
		t.Fatalf("register agent: %v", err)
	}

	task := protocol.NewRootTask("session-test", "do something", "reporter")
	if _, err := rt.Dispatch(context.Background(), task); err != nil {
		t.Fatalf("dispatch: %v", err)
	}

	details := rt.DetailMessages(context.Background(), task.TraceID)
	joined := strings.Join(details, "\n")
	if strings.Contains(joined, "## 执行摘要") || strings.Contains(joined, "这里是一段很长的汇总内容") {
		t.Fatalf("expected verbose report body to be omitted from details, got:\n%s", joined)
	}
	if !strings.Contains(joined, "详细摘要已折叠") {
		t.Fatalf("expected compact completion message, got:\n%s", joined)
	}
}
