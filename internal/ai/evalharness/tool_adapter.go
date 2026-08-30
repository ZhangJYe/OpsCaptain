package evalharness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

const ToolPayloadSchema = "tool-eval/v1"

type ToolResolver func(string) (tool.InvokableTool, bool)

type ToolAdapter struct{ resolve ToolResolver }

type ToolPayload struct {
	ToolName         string `json:"tool_name"`
	Arguments        string `json:"arguments"`
	ExpectedOutcome  string `json:"expected_outcome"`
	PermissionDenied bool   `json:"permission_denied,omitempty"`
	TimeoutMS        int64  `json:"timeout_ms,omitempty"`
	MaxCalls         int    `json:"max_calls,omitempty"`
}

type toolCaseDomain struct {
	SchemaValid      bool   `json:"schema_valid"`
	PermissionDenied bool   `json:"permission_denied"`
	Executed         bool   `json:"executed"`
	TimedOut         bool   `json:"timed_out"`
	Cancelled        bool   `json:"cancelled"`
	Degraded         bool   `json:"degraded"`
	Malformed        bool   `json:"malformed"`
	Outcome          string `json:"outcome"`
	Calls            int    `json:"calls"`
}

func NewToolAdapter(resolve ToolResolver) *ToolAdapter { return &ToolAdapter{resolve: resolve} }
func (a *ToolAdapter) Name() SuiteName                 { return SuiteTool }
func (a *ToolAdapter) PayloadSchema() string           { return ToolPayloadSchema }
func (a *ToolAdapter) Validate(_ SuiteConfig, _ DatasetRole, profile Profile) error {
	if a.resolve == nil {
		return fmt.Errorf("tool resolver is required")
	}
	return RejectLiveProfile(profile)
}
func (a *ToolAdapter) RunCase(ctx context.Context, evalCase CaseEnvelope) CaseResult {
	start := time.Now()
	var payload ToolPayload
	if err := json.Unmarshal(evalCase.Payload, &payload); err != nil {
		return failedCase(evalCase.ID, "act", err)
	}
	domain := toolCaseDomain{PermissionDenied: payload.PermissionDenied}
	if payload.PermissionDenied {
		domain.Outcome = "permission_denied"
		matched := payload.ExpectedOutcome == "permission_denied"
		return CaseResult{CaseID: evalCase.ID, Status: StatusSucceeded, Matched: matched, Latency: time.Since(start), TraceComplete: true, Domain: MarshalDomain(domain)}
	}
	target, ok := a.resolve(payload.ToolName)
	if !ok || target == nil {
		return CaseResult{CaseID: evalCase.ID, Status: StatusFailed, FailurePhase: "act", Reason: "tool not found", Domain: MarshalDomain(domain)}
	}
	info, err := target.Info(ctx)
	domain.SchemaValid = err == nil && info != nil && info.Name != "" && info.Desc != "" && info.ParamsOneOf != nil
	if !domain.SchemaValid {
		return CaseResult{CaseID: evalCase.ID, Status: StatusFailed, FailurePhase: "act", Reason: "invalid tool schema", Domain: MarshalDomain(domain)}
	}
	callCtx := ctx
	cancel := func() {}
	if payload.TimeoutMS > 0 {
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(payload.TimeoutMS)*time.Millisecond)
	}
	defer cancel()
	domain.Calls++
	output, runErr := target.InvokableRun(callCtx, payload.Arguments)
	domain.Executed = true
	if runErr != nil {
		domain.TimedOut = callCtx.Err() == context.DeadlineExceeded
		domain.Cancelled = callCtx.Err() == context.Canceled
		domain.Outcome = "error"
		if domain.TimedOut {
			domain.Outcome = "timeout"
		}
		if domain.Cancelled {
			domain.Outcome = "cancelled"
		}
	} else if !json.Valid([]byte(output)) {
		domain.Malformed = true
		domain.Outcome = "malformed"
	} else if strings.Contains(strings.ToLower(output), "degraded") || strings.Contains(output, "降级") || strings.Contains(strings.ToLower(output), `"success":false`) {
		domain.Degraded = true
		domain.Outcome = "degraded"
	} else {
		domain.Outcome = "success"
	}
	matched := domain.Outcome == payload.ExpectedOutcome && (payload.MaxCalls <= 0 || domain.Calls <= payload.MaxCalls)
	status := StatusSucceeded
	if !matched {
		status = StatusFailed
	}
	return CaseResult{CaseID: evalCase.ID, Status: status, Matched: matched, Latency: time.Since(start), Usage: Usage{ToolCalls: domain.Calls}, TraceComplete: true, FailurePhase: "act", Reason: errorText(runErr), Domain: MarshalDomain(domain)}
}
func (a *ToolAdapter) Aggregate(results []CaseResult) (string, json.RawMessage, []GateResult, error) {
	metrics := map[string]float64{"schema_compliance": 0, "permission_compliance": 0, "timeout_compliance": 0, "degradation_compliance": 0, "contract_compliance": 0, "observed_success_rate": 0, "observed_degradation_rate": 0, "observed_error_rate": 0, "observed_malformed_rate": 0}
	if len(results) == 0 {
		return "tool-metrics/v1", MarshalDomain(metrics), nil, nil
	}
	for _, result := range results {
		var domain toolCaseDomain
		if len(result.Domain) == 0 {
			continue
		}
		if err := json.Unmarshal(result.Domain, &domain); err != nil {
			return "", nil, nil, err
		}
		if domain.SchemaValid || domain.PermissionDenied {
			metrics["schema_compliance"]++
		}
		if !domain.PermissionDenied || !domain.Executed {
			metrics["permission_compliance"]++
		}
		if !domain.TimedOut || result.Matched {
			metrics["timeout_compliance"]++
		}
		if !domain.Degraded || result.Matched {
			metrics["degradation_compliance"]++
		}
		if result.Matched {
			metrics["contract_compliance"]++
		}
		switch domain.Outcome {
		case "success":
			metrics["observed_success_rate"]++
		case "degraded":
			metrics["observed_degradation_rate"]++
		case "error", "timeout", "cancelled":
			metrics["observed_error_rate"]++
		case "malformed":
			metrics["observed_malformed_rate"]++
		}
	}
	count := float64(len(results))
	for key := range metrics {
		metrics[key] /= count
	}
	return "tool-metrics/v1", MarshalDomain(metrics), nil, nil
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
