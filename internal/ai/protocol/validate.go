package protocol

import (
	"fmt"
	"strings"
)

func ValidateTaskResult(r *TaskResult) error {
	if r == nil {
		return fmt.Errorf("task result is nil")
	}
	if strings.TrimSpace(r.TaskID) == "" {
		return fmt.Errorf("task_id is required")
	}
	if strings.TrimSpace(r.Agent) == "" {
		return fmt.Errorf("agent is required")
	}
	switch r.Status {
	case ResultStatusSucceeded, ResultStatusFailed, ResultStatusDegraded:
	default:
		return fmt.Errorf("invalid status %q, must be succeeded/failed/degraded", r.Status)
	}
	if strings.TrimSpace(r.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	if len(r.Summary) > 4096 {
		return fmt.Errorf("summary exceeds 4096 characters")
	}
	if r.Status == ResultStatusDegraded && strings.TrimSpace(r.DegradationReason) == "" {
		return fmt.Errorf("degradation_reason is required when status is degraded")
	}
	if r.Status == ResultStatusFailed && r.Error == nil {
		return fmt.Errorf("error is required when status is failed")
	}
	if r.Confidence < 0.0 || r.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got %.2f", r.Confidence)
	}
	for i, ev := range r.Evidence {
		if strings.TrimSpace(ev.SourceType) == "" {
			return fmt.Errorf("evidence[%d]: source_type is required", i)
		}
		if strings.TrimSpace(ev.Title) == "" {
			return fmt.Errorf("evidence[%d]: title is required", i)
		}
	}
	if r.Error != nil {
		if strings.TrimSpace(r.Error.Code) == "" && strings.TrimSpace(r.Error.Message) == "" {
			return fmt.Errorf("error must have code or message")
		}
	}
	return nil
}
