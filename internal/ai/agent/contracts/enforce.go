package contracts

import (
	"fmt"
	"strings"

	"SuperBizAgent/internal/ai/protocol"
)

func ValidateAgainstContract(result *protocol.TaskResult) error {
	if result == nil {
		return fmt.Errorf("task result is nil")
	}
	if err := protocol.ValidateTaskResult(result); err != nil {
		return fmt.Errorf("schema validation: %w", err)
	}
	contract, ok := Get(result.Agent)
	if !ok {
		return nil
	}
	if strings.TrimSpace(contract.Agent) == "" {
		return nil
	}
	if result.Status == protocol.ResultStatusDegraded {
		if strings.TrimSpace(result.DegradationReason) == "" {
			return fmt.Errorf("contract %q requires degradation_reason when status is degraded", contract.Agent)
		}
	}
	if result.Status == protocol.ResultStatusSucceeded && len(contract.Outputs) > 0 && strings.TrimSpace(result.Summary) == "" {
		return fmt.Errorf("contract %q requires non-empty summary", contract.Agent)
	}
	return nil
}

func EnforceContract(result *protocol.TaskResult) *protocol.TaskResult {
	if result == nil {
		return nil
	}
	if err := ValidateAgainstContract(result); err != nil {
		degraded := *result
		degraded.Status = protocol.ResultStatusDegraded
		existing := strings.TrimSpace(result.DegradationReason)
		msg := fmt.Sprintf("contract enforcement failed: %v", err)
		if existing != "" {
			msg = existing + " ; " + msg
		}
		degraded.DegradationReason = msg
		degraded.Confidence = clampConfidence(degraded.Confidence * 0.5)
		return &degraded
	}
	return result
}

func clampConfidence(c float64) float64 {
	if c < 0.0 {
		return 0.0
	}
	if c > 1.0 {
		return 1.0
	}
	return c
}
