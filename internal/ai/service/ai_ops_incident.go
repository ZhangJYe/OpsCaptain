package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/consts"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
)

type IncidentStatus string

const (
	IncidentStatusActive          IncidentStatus = "active"
	IncidentStatusRunning         IncidentStatus = "running"
	IncidentStatusWaitingApproval IncidentStatus = "waiting_approval"
	IncidentStatusCompleted       IncidentStatus = "completed"
	IncidentStatusDegraded        IncidentStatus = "degraded"
	IncidentStatusFailed          IncidentStatus = "failed"
)

var (
	ErrIncidentTurnRunning = errors.New("incident already has a running turn")
	ErrIncidentNotFound    = errors.New("incident not found")
)

type IncidentTurn struct {
	TurnID            string         `json:"turn_id"`
	IncidentID        string         `json:"incident_id"`
	UserQuery         string         `json:"user_query"`
	TraceID           string         `json:"trace_id,omitempty"`
	Status            IncidentStatus `json:"status"`
	Result            string         `json:"result,omitempty"`
	Detail            []string       `json:"detail,omitempty"`
	Engine            string         `json:"engine,omitempty"`
	SelectedSkillIds  []string       `json:"selected_skill_ids,omitempty"`
	ApprovalRequestID string         `json:"approval_request_id,omitempty"`
	ApprovalStatus    string         `json:"approval_status,omitempty"`
	DegradationReason string         `json:"degradation_reason,omitempty"`
	CreatedAt         int64          `json:"created_at"`
	FinishedAt        int64          `json:"finished_at,omitempty"`
}

type IncidentEvent struct {
	EventID    string         `json:"event_id"`
	IncidentID string         `json:"incident_id"`
	TurnID     string         `json:"turn_id,omitempty"`
	TraceID    string         `json:"trace_id,omitempty"`
	Type       string         `json:"type"`
	Agent      string         `json:"agent,omitempty"`
	Message    string         `json:"message,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
	CreatedAt  int64          `json:"created_at"`
}

type IncidentSession struct {
	IncidentID       string          `json:"incident_id"`
	SessionID        string          `json:"session_id"`
	Title            string          `json:"title"`
	Status           IncidentStatus  `json:"status"`
	EngineStrategy   string          `json:"engine_strategy"`
	LatestSummary    string          `json:"latest_summary,omitempty"`
	SelectedSkillIds []string        `json:"selected_skill_ids,omitempty"`
	Turns            []IncidentTurn  `json:"turns,omitempty"`
	Events           []IncidentEvent `json:"events,omitempty"`
	CreatedAt        int64           `json:"created_at"`
	UpdatedAt        int64           `json:"updated_at"`
}

type IncidentStore interface {
	Create(context.Context, *IncidentSession) error
	Get(context.Context, string) (*IncidentSession, error)
	List(context.Context) ([]IncidentSession, error)
	Update(context.Context, string, func(*IncidentSession) error) (*IncidentSession, error)
}

var (
	incidentStoreMu sync.Mutex
	incidentStores  = map[string]IncidentStore{}

	newIncidentStore = func(dir string) (IncidentStore, error) {
		return NewFileIncidentStore(dir)
	}
	runIncidentAIOps = RunAIOpsMultiAgent
	startIncidentRun = func(run func()) {
		go run()
	}
	incidentConfigString = func(ctx context.Context, key string) (string, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return "", false
		}
		return strings.TrimSpace(v.String()), true
	}
	incidentConfigBool = func(ctx context.Context, key string) (bool, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || strings.TrimSpace(v.String()) == "" {
			return false, false
		}
		return v.Bool(), true
	}
	incidentConfigInt = func(ctx context.Context, key string) (int, bool) {
		v, err := g.Cfg().Get(ctx, key)
		if err != nil || v.Int() <= 0 {
			return 0, false
		}
		return v.Int(), true
	}
)

func CreateAIOpsIncident(ctx context.Context, query, engine string, selectedSkillIds []string) (*IncidentSession, error) {
	if !incidentEnabled(ctx) {
		return nil, fmt.Errorf("aiops incident sessions are disabled")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("incident query is empty")
	}
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	incidentID := uuid.NewString()
	turn := IncidentTurn{
		TurnID:           uuid.NewString(),
		IncidentID:       incidentID,
		UserQuery:        query,
		Status:           IncidentStatusRunning,
		SelectedSkillIds: selectedSkillIds,
		CreatedAt:        now,
	}
	sessionID := memory.GenerateSessionID()
	if value, ok := ctx.Value(consts.CtxKeySessionID).(string); ok && strings.TrimSpace(value) != "" {
		sessionID = strings.TrimSpace(value)
	}
	incident := &IncidentSession{
		IncidentID:       incidentID,
		SessionID:        sessionID,
		Title:            incidentTitle(query),
		Status:           IncidentStatusRunning,
		EngineStrategy:   incidentEngine(ctx, engine),
		SelectedSkillIds: selectedSkillIds,
		Turns:            []IncidentTurn{turn},
		Events:           []IncidentEvent{newIncidentEvent(incidentID, turn.TurnID, "", "turn_started", "", "开始事故排障", nil)},
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := store.Create(ctx, incident); err != nil {
		return nil, err
	}
	startIncidentTurn(ctx, incidentID, turn.TurnID)
	return incident, nil
}

func AppendAIOpsIncidentTurn(ctx context.Context, incidentID, query string, selectedSkillIds []string) (*IncidentSession, error) {
	if !incidentEnabled(ctx) {
		return nil, fmt.Errorf("aiops incident sessions are disabled")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("incident query is empty")
	}
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	turnID := uuid.NewString()
	incident, err := store.Update(ctx, incidentID, func(incident *IncidentSession) error {
		if hasRunningIncidentTurn(incident) {
			return ErrIncidentTurnRunning
		}
		incident.Turns = append(incident.Turns, IncidentTurn{
			TurnID:           turnID,
			IncidentID:       incident.IncidentID,
			UserQuery:        query,
			Status:           IncidentStatusRunning,
			SelectedSkillIds: selectedSkillIds,
			CreatedAt:        now,
		})
		incident.Status = IncidentStatusRunning
		incident.Events = append(incident.Events, newIncidentEvent(incident.IncidentID, turnID, "", "turn_started", "", "继续事故排障", nil))
		return nil
	})
	if err != nil {
		return incident, err
	}
	startIncidentTurn(ctx, incident.IncidentID, turnID)
	return incident, nil
}

func GetAIOpsIncident(ctx context.Context, incidentID string) (*IncidentSession, error) {
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return nil, err
	}
	return store.Get(ctx, incidentID)
}

func ListAIOpsIncidents(ctx context.Context) ([]IncidentSession, error) {
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return nil, err
	}
	return store.List(ctx)
}

func GetAIOpsIncidentEvents(ctx context.Context, incidentID, turnID string) ([]IncidentEvent, error) {
	incident, err := GetAIOpsIncident(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	turnID = strings.TrimSpace(turnID)
	out := make([]IncidentEvent, 0, len(incident.Events))
	for _, event := range incident.Events {
		if turnID != "" && event.TurnID != "" && event.TurnID != turnID {
			continue
		}
		out = append(out, event)
	}
	limit := incidentEventReplayLimit(ctx)
	if limit > 0 && len(out) > limit {
		out = append([]IncidentEvent(nil), out[len(out)-limit:]...)
	}
	return out, nil
}

func RecordIncidentApprovalExecution(ctx context.Context, requestID string, response ExecutionResponse) error {
	return updateIncidentApproval(ctx, requestID, func(incident *IncidentSession, turn *IncidentTurn) {
		status := incidentStatusForResponse(response, nil)
		turn.TraceID = strings.TrimSpace(response.TraceID)
		turn.Engine = strings.TrimSpace(response.Engine)
		turn.Result = strings.TrimSpace(response.Content)
		turn.Detail = append([]string{}, response.Detail...)
		turn.Status = status
		turn.ApprovalStatus = response.ApprovalStatus
		turn.DegradationReason = strings.TrimSpace(response.DegradationReason)
		turn.FinishedAt = time.Now().UnixMilli()
		incident.Status = status
		incident.LatestSummary = turn.Result
		incident.Events = append(incident.Events, newIncidentEvent(incident.IncidentID, turn.TurnID, turn.TraceID, "approval_executed", turn.Engine, "审批后执行完成", map[string]any{
			"approval_request_id": requestID,
			"approval_status":     response.ApprovalStatus,
			"status":              status,
		}))
	})
}

func RecordIncidentApprovalRejection(ctx context.Context, requestID, reason string) error {
	return updateIncidentApproval(ctx, requestID, func(incident *IncidentSession, turn *IncidentTurn) {
		turn.Status = IncidentStatusActive
		turn.ApprovalStatus = string(ApprovalStatusRejected)
		turn.FinishedAt = time.Now().UnixMilli()
		incident.Status = IncidentStatusActive
		incident.Events = append(incident.Events, newIncidentEvent(incident.IncidentID, turn.TurnID, turn.TraceID, "approval_rejected", "", "审批已拒绝", map[string]any{
			"approval_request_id": requestID,
			"review_reason":       strings.TrimSpace(reason),
		}))
	})
}

func IncidentTurnTerminal(turn IncidentTurn) bool {
	switch turn.Status {
	case IncidentStatusCompleted, IncidentStatusDegraded, IncidentStatusFailed, IncidentStatusWaitingApproval, IncidentStatusActive:
		return true
	default:
		return false
	}
}

func IncidentLatestTurn(incident *IncidentSession) *IncidentTurn {
	if incident == nil || len(incident.Turns) == 0 {
		return nil
	}
	turn := incident.Turns[len(incident.Turns)-1]
	return &turn
}
