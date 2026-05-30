package app

import (
	"SuperBizAgent/internal/ai/memory"
	"fmt"
)

// ValidateSessionID checks whether a session ID is well-formed.
func ValidateSessionID(id string) error {
	return memory.ValidateSessionID(id)
}

// GenerateSessionID creates a new random session ID.
func GenerateSessionID() string {
	return memory.GenerateSessionID()
}

// ChatInput is the application-layer input for a synchronous chat request.
type ChatInput struct {
	SessionID string
	Question  string
	SkillIDs  []string
}

// ChatResult is the application-layer output for a chat request.
type ChatResult struct {
	Answer            string
	Detail            []string
	TraceID           string
	Mode              string
	Degraded          bool
	DegradationReason string
	Cached            bool
	HTTPStatus        int // non-zero when Controller should write this status (429/503/504)
}

// PromptRejectedError is returned when the prompt guard blocks the request.
type PromptRejectedError struct {
	Reason    string
	RiskScore float64
	RiskLevel string
	Pattern   string
}

func (e *PromptRejectedError) Error() string {
	return fmt.Sprintf("prompt rejected: %s", e.Reason)
}
