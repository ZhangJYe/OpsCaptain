package app

import (
	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/ai/skills"
	"SuperBizAgent/internal/ai/tools"
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

// --- Re-exports from ai/skills ---

type UserSkillStore = skills.UserSkillStore
type UserSkillLoader = skills.UserSkillLoader
type UserSkill = skills.UserSkill
type UserMCPTool = skills.UserMCPTool

const (
	StatusPending  = skills.StatusPending
	StatusApproved = skills.StatusApproved
	StatusRejected = skills.StatusRejected
)

// --- Re-exports from ai/tools ---

type DynamicMCPRegistry = tools.DynamicMCPRegistry

// --- Re-exports from ai/protocol ---

type ChangeEvent = protocol.ChangeEvent

// --- Re-exports from ai/service ---

type SessionTokenAudit = service.SessionTokenAudit
type ChatTaskStatus = service.ChatTaskStatus
type IncidentEvent = service.IncidentEvent
type IncidentSession = service.IncidentSession
type ApprovalRequest = service.ApprovalRequest
type ExecutionResponse = service.ExecutionResponse
type MemoryListOptions = service.MemoryListOptions
type MemoryItemView = service.MemoryItemView
type MemoryPromoteOptions = service.MemoryPromoteOptions

var (
	NewMemoryService          = service.NewMemoryService
	ListApprovalRequests      = service.ListApprovalRequests
	ApproveQueuedAIOpsRequest = service.ApproveQueuedAIOpsRequest
	RejectQueuedAIOpsRequest  = service.RejectQueuedAIOpsRequest
	GetSessionTokenAudit      = service.GetSessionTokenAudit
	IsDailyTokenLimitError    = service.IsDailyTokenLimitError
	SubmitChatTask            = service.SubmitChatTask
	GetChatTask               = service.GetChatTask
	CreateAIOpsIncident       = service.CreateAIOpsIncident
	AppendAIOpsIncidentTurn   = service.AppendAIOpsIncidentTurn
	ListAIOpsIncidents        = service.ListAIOpsIncidents
	GetAIOpsIncident          = service.GetAIOpsIncident
	GetAIOpsIncidentEvents    = service.GetAIOpsIncidentEvents
	IncidentLatestTurn        = service.IncidentLatestTurn
	IncidentTurnTerminal      = service.IncidentTurnTerminal
	ErrIncidentTurnRunning    = service.ErrIncidentTurnRunning
)
