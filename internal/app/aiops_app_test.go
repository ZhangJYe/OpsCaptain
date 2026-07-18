package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"SuperBizAgent/internal/ai/protocol"
	aiservice "SuperBizAgent/internal/ai/service"
)

const testSensitiveAIOpsOutput = "sk-abcdefghijklmnop"

func TestHandleAIOpsFiltersEveryUserVisibleTextField(t *testing.T) {
	a := NewAIOpsApp()
	a.SetDegradationCheck(func(context.Context, string) aiservice.DegradationDecision {
		return aiservice.DegradationDecision{}
	})
	a.SetRunMultiAgent(func(context.Context, string) (aiservice.ExecutionResponse, error) {
		return sensitiveExecutionResponse(), nil
	})

	result, err := a.HandleAIOps(context.Background(), &AIOpsInput{Query: "检查服务异常"})
	if err != nil {
		t.Fatalf("HandleAIOps() error = %v", err)
	}
	assertSensitiveOutputRedacted(t, result)
}

func TestHandleAIOpsResultFiltersPersistedOutput(t *testing.T) {
	a := NewAIOpsApp()
	a.SetGetResult(func(context.Context, string) (*aiservice.ExecutionResponse, error) {
		result := sensitiveExecutionResponse()
		return &result, nil
	})

	result, err := a.HandleAIOpsResult(context.Background(), "trace-1")
	if err != nil {
		t.Fatalf("HandleAIOpsResult() error = %v", err)
	}
	assertSensitiveOutputRedacted(t, result)
}

func TestHandleAIOpsTraceFiltersMessageDetailAndPayload(t *testing.T) {
	a := NewAIOpsApp()
	a.SetGetTrace(func(context.Context, string) ([]*protocol.TaskEvent, []string, error) {
		return []*protocol.TaskEvent{{
			Message: "trace " + testSensitiveAIOpsOutput,
			Payload: map[string]any{
				"summary": "payload " + testSensitiveAIOpsOutput,
				"nested": map[string]any{
					"actions": []any{"retry " + testSensitiveAIOpsOutput},
				},
			},
		}}, []string{"detail " + testSensitiveAIOpsOutput}, nil
	})

	result, err := a.HandleAIOpsTrace(context.Background(), "trace-1")
	if err != nil {
		t.Fatalf("HandleAIOpsTrace() error = %v", err)
	}
	assertSensitiveOutputRedacted(t, result)
}

func TestHandleAIOpsRunsFiltersDegradationReason(t *testing.T) {
	a := NewAIOpsApp()
	a.SetRunAsync(func(context.Context, string) (*aiservice.AIOpsRunInfo, error) {
		return &aiservice.AIOpsRunInfo{DegradationReason: "failed " + testSensitiveAIOpsOutput}, nil
	})

	result, err := a.HandleAIOpsRuns(context.Background(), &AIOpsRunsInput{Query: "检查服务异常"})
	if err != nil {
		t.Fatalf("HandleAIOpsRuns() error = %v", err)
	}
	assertSensitiveOutputRedacted(t, result)
}

func sensitiveExecutionResponse() aiservice.ExecutionResponse {
	return aiservice.ExecutionResponse{
		Content:           "result " + testSensitiveAIOpsOutput,
		Detail:            []string{"detail " + testSensitiveAIOpsOutput},
		Status:            protocol.ResultStatusDegraded,
		DegradationReason: "reason " + testSensitiveAIOpsOutput,
		ExecutionPlan:     []string{"step " + testSensitiveAIOpsOutput},
		Evidence: []protocol.EvidenceItem{{
			SourceType: "tool " + testSensitiveAIOpsOutput,
			SourceID:   "source " + testSensitiveAIOpsOutput,
			Title:      "title " + testSensitiveAIOpsOutput,
			Snippet:    "snippet " + testSensitiveAIOpsOutput,
			URI:        "uri " + testSensitiveAIOpsOutput,
		}},
		NextActions: []string{"next " + testSensitiveAIOpsOutput},
	}
}

func assertSensitiveOutputRedacted(t *testing.T, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal user-visible output: %v", err)
	}
	got := string(encoded)
	if strings.Contains(got, testSensitiveAIOpsOutput) {
		t.Fatalf("user-visible output still contains sensitive value: %s", got)
	}
	if !strings.Contains(got, "[REDACTED_API_KEY]") {
		t.Fatalf("user-visible output did not contain redaction marker: %s", got)
	}
}
