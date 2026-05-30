package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"SuperBizAgent/internal/ai/memory"
	"SuperBizAgent/internal/ai/protocol"
	"SuperBizAgent/internal/ai/runtime"
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
	IncidentID     string          `json:"incident_id"`
	SessionID      string          `json:"session_id"`
	Title          string          `json:"title"`
	Status         IncidentStatus  `json:"status"`
	EngineStrategy string          `json:"engine_strategy"`
	LatestSummary  string          `json:"latest_summary,omitempty"`
	Turns          []IncidentTurn  `json:"turns,omitempty"`
	Events         []IncidentEvent `json:"events,omitempty"`
	CreatedAt      int64           `json:"created_at"`
	UpdatedAt      int64           `json:"updated_at"`
}

type IncidentStore interface {
	Create(context.Context, *IncidentSession) error
	Get(context.Context, string) (*IncidentSession, error)
	List(context.Context) ([]IncidentSession, error)
	Update(context.Context, string, func(*IncidentSession) error) (*IncidentSession, error)
}

type FileIncidentStore struct {
	dir string
	mu  sync.Mutex
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

func NewFileIncidentStore(dir string) (*FileIncidentStore, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("incident store dir is empty")
	}
	return &FileIncidentStore{dir: dir}, nil
}

func (s *FileIncidentStore) Create(_ context.Context, incident *IncidentSession) error {
	if incident == nil || strings.TrimSpace(incident.IncidentID) == "" {
		return fmt.Errorf("incident id is empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	path := s.path(incident.IncidentID)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("incident %s already exists", incident.IncidentID)
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeIncidentJSON(path, incident)
}

func (s *FileIncidentStore) Get(_ context.Context, incidentID string) (*IncidentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(incidentID)
}

func (s *FileIncidentStore) List(_ context.Context) ([]IncidentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	items := make([]IncidentSession, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		incident, err := s.read(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		items = append(items, *incident)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items, nil
}

func (s *FileIncidentStore) Update(_ context.Context, incidentID string, update func(*IncidentSession) error) (*IncidentSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	incident, err := s.read(incidentID)
	if err != nil {
		return nil, err
	}
	if update != nil {
		if err := update(incident); err != nil {
			return nil, err
		}
	}
	incident.UpdatedAt = time.Now().UnixMilli()
	if err := writeIncidentJSON(s.path(incidentID), incident); err != nil {
		return nil, err
	}
	return incident, nil
}

func (s *FileIncidentStore) read(incidentID string) (*IncidentSession, error) {
	data, err := os.ReadFile(s.path(incidentID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrIncidentNotFound
		}
		return nil, err
	}
	var incident IncidentSession
	if err := json.Unmarshal(data, &incident); err != nil {
		return nil, err
	}
	return &incident, nil
}

func (s *FileIncidentStore) path(incidentID string) string {
	return filepath.Join(s.dir, strings.TrimSpace(incidentID)+".json")
}

func CreateAIOpsIncident(ctx context.Context, query, engine string) (*IncidentSession, error) {
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
		TurnID:     uuid.NewString(),
		IncidentID: incidentID,
		UserQuery:  query,
		Status:     IncidentStatusRunning,
		CreatedAt:  now,
	}
	sessionID := memory.GenerateSessionID()
	if value, ok := ctx.Value(consts.CtxKeySessionID).(string); ok && strings.TrimSpace(value) != "" {
		sessionID = strings.TrimSpace(value)
	}
	incident := &IncidentSession{
		IncidentID:     incidentID,
		SessionID:      sessionID,
		Title:          incidentTitle(query),
		Status:         IncidentStatusRunning,
		EngineStrategy: incidentEngine(ctx, engine),
		Turns:          []IncidentTurn{turn},
		Events:         []IncidentEvent{newIncidentEvent(incidentID, turn.TurnID, "", "turn_started", "", "开始事故排障", nil)},
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := store.Create(ctx, incident); err != nil {
		return nil, err
	}
	startIncidentTurn(ctx, incidentID, turn.TurnID)
	return incident, nil
}

func AppendAIOpsIncidentTurn(ctx context.Context, incidentID, query string) (*IncidentSession, error) {
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
			TurnID:     turnID,
			IncidentID: incident.IncidentID,
			UserQuery:  query,
			Status:     IncidentStatusRunning,
			CreatedAt:  now,
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

func startIncidentTurn(ctx context.Context, incidentID, turnID string) {
	runCtx := context.WithoutCancel(ctx)
	startIncidentRun(func() {
		executeIncidentTurn(runCtx, incidentID, turnID)
	})
}

func executeIncidentTurn(ctx context.Context, incidentID, turnID string) {
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return
	}
	incident, err := store.Get(ctx, incidentID)
	if err != nil {
		return
	}
	turn, ok := incidentTurnByID(incident, turnID)
	if !ok {
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, incidentTurnTimeout(ctx))
	defer cancel()
	runCtx = context.WithValue(runCtx, consts.CtxKeySessionID, incident.SessionID)
	runCtx = WithAIOpsEngine(runCtx, incident.EngineStrategy)
	runCtx = WithAIOpsIncidentContext(runCtx, incidentContext(ctx, incident, turnID))
	runCtx = withIncidentEventSink(runCtx, store, incidentID, turnID)
	response, runErr := runIncidentAIOps(runCtx, turn.UserQuery)
	_, _ = store.Update(context.Background(), incidentID, func(current *IncidentSession) error {
		currentTurn, ok := incidentTurnPointer(current, turnID)
		if !ok {
			return nil
		}
		status := incidentStatusForResponse(response, runErr)
		currentTurn.Status = status
		currentTurn.TraceID = firstNonEmptyIncident(response.TraceID, currentTurn.TraceID)
		currentTurn.Result = strings.TrimSpace(response.Content)
		currentTurn.Detail = append([]string{}, response.Detail...)
		currentTurn.Engine = strings.TrimSpace(response.Engine)
		currentTurn.ApprovalRequestID = strings.TrimSpace(response.ApprovalRequestID)
		currentTurn.ApprovalStatus = strings.TrimSpace(response.ApprovalStatus)
		currentTurn.DegradationReason = strings.TrimSpace(response.DegradationReason)
		if runErr != nil && currentTurn.Result == "" {
			currentTurn.Result = runErr.Error()
		}
		if status != IncidentStatusWaitingApproval {
			currentTurn.FinishedAt = time.Now().UnixMilli()
		}
		current.Status = status
		if currentTurn.Result != "" {
			current.LatestSummary = currentTurn.Result
		}
		current.Events = append(current.Events, incidentCompletionEvent(current.IncidentID, currentTurn, runErr))
		return nil
	})
}

func withIncidentApprovalRun(ctx context.Context, requestID string) context.Context {
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return ctx
	}
	incidents, err := store.List(ctx)
	if err != nil {
		return ctx
	}
	for idx := range incidents {
		incident := &incidents[idx]
		for _, turn := range incident.Turns {
			if turn.ApprovalRequestID != requestID {
				continue
			}
			runCtx := WithAIOpsEngine(ctx, incident.EngineStrategy)
			runCtx = WithAIOpsIncidentContext(runCtx, incidentContext(ctx, incident, turn.TurnID))
			return withIncidentEventSink(runCtx, store, incident.IncidentID, turn.TurnID)
		}
	}
	return ctx
}

func withIncidentEventSink(ctx context.Context, store IncidentStore, incidentID, turnID string) context.Context {
	return runtime.WithTaskEventSink(ctx, func(event *protocol.TaskEvent) {
		if event == nil {
			return
		}
		_, _ = store.Update(context.Background(), incidentID, func(current *IncidentSession) error {
			current.Events = append(current.Events, IncidentEvent{
				EventID:    event.EventID,
				IncidentID: current.IncidentID,
				TurnID:     turnID,
				TraceID:    event.TraceID,
				Type:       event.Type,
				Agent:      event.Agent,
				Message:    event.Message,
				Payload:    clonePayload(event.Payload),
				CreatedAt:  event.CreatedAt,
			})
			if existing, ok := incidentTurnPointer(current, turnID); ok && existing.TraceID == "" {
				existing.TraceID = strings.TrimSpace(event.TraceID)
			}
			return nil
		})
	})
}

func updateIncidentApproval(ctx context.Context, requestID string, update func(*IncidentSession, *IncidentTurn)) error {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil
	}
	store, err := getOrCreateIncidentStore(ctx)
	if err != nil {
		return err
	}
	incidents, err := store.List(ctx)
	if err != nil {
		return err
	}
	for _, incident := range incidents {
		found := false
		for _, turn := range incident.Turns {
			if turn.ApprovalRequestID == requestID {
				found = true
				break
			}
		}
		if !found {
			continue
		}
		_, err := store.Update(ctx, incident.IncidentID, func(current *IncidentSession) error {
			for idx := range current.Turns {
				if current.Turns[idx].ApprovalRequestID != requestID {
					continue
				}
				update(current, &current.Turns[idx])
				return nil
			}
			return nil
		})
		return err
	}
	return nil
}

func getOrCreateIncidentStore(ctx context.Context) (IncidentStore, error) {
	dir := incidentStoreDir(ctx)
	incidentStoreMu.Lock()
	defer incidentStoreMu.Unlock()
	if store, ok := incidentStores[dir]; ok {
		return store, nil
	}
	store, err := newIncidentStore(dir)
	if err != nil {
		return nil, err
	}
	incidentStores[dir] = store
	return store, nil
}

func incidentStoreDir(ctx context.Context) string {
	if dir, ok := incidentConfigString(ctx, "aiops.incident.store_dir"); ok {
		return dir
	}
	if dir, ok := incidentConfigString(ctx, "multi_agent.data_dir"); ok {
		return filepath.Join(dir, "incidents")
	}
	return filepath.Join(".", "var", "runtime", "incidents")
}

func incidentEnabled(ctx context.Context) bool {
	if enabled, ok := incidentConfigBool(ctx, "aiops.incident.enabled"); ok {
		return enabled
	}
	return true
}

func incidentTurnTimeout(ctx context.Context) time.Duration {
	if timeout, ok := incidentConfigInt(ctx, "aiops.incident.turn_timeout_ms"); ok {
		return time.Duration(timeout) * time.Millisecond
	}
	return 2 * time.Minute
}

func incidentMaxContextTurns(ctx context.Context) int {
	if limit, ok := incidentConfigInt(ctx, "aiops.incident.max_context_turns"); ok {
		return limit
	}
	return 6
}

func incidentEventReplayLimit(ctx context.Context) int {
	if limit, ok := incidentConfigInt(ctx, "aiops.incident.event_replay_limit"); ok {
		return limit
	}
	return 200
}

func incidentEngine(ctx context.Context, requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested == "gos" || requested == "gos_engine" {
		return "gos_engine"
	}
	if requested == "plan_execute_replan" {
		return requested
	}
	if configured, ok := incidentConfigString(ctx, "aiops.engine"); ok {
		configured = strings.ToLower(strings.TrimSpace(configured))
		if configured == "gos" || configured == "gos_engine" {
			return "gos_engine"
		}
	}
	return "plan_execute_replan"
}

func incidentContext(ctx context.Context, incident *IncidentSession, currentTurnID string) string {
	if incident == nil {
		return ""
	}
	turns := make([]IncidentTurn, 0, len(incident.Turns))
	for _, turn := range incident.Turns {
		if turn.TurnID == currentTurnID || strings.TrimSpace(turn.Result) == "" {
			continue
		}
		turns = append(turns, turn)
	}
	limit := incidentMaxContextTurns(ctx)
	if limit > 0 && len(turns) > limit {
		turns = turns[len(turns)-limit:]
	}
	if len(turns) == 0 {
		return ""
	}
	var builder strings.Builder
	for idx, turn := range turns {
		builder.WriteString(fmt.Sprintf("%d. 用户补充：%s\n", idx+1, previewIncident(turn.UserQuery, 420)))
		builder.WriteString(fmt.Sprintf("   已有结论：%s\n", previewIncident(turn.Result, 700)))
	}
	return strings.TrimSpace(builder.String())
}

func hasRunningIncidentTurn(incident *IncidentSession) bool {
	if incident == nil {
		return false
	}
	for _, turn := range incident.Turns {
		if turn.Status == IncidentStatusRunning {
			return true
		}
	}
	return false
}

func incidentTurnByID(incident *IncidentSession, turnID string) (IncidentTurn, bool) {
	if incident == nil {
		return IncidentTurn{}, false
	}
	for _, turn := range incident.Turns {
		if turn.TurnID == turnID {
			return turn, true
		}
	}
	return IncidentTurn{}, false
}

func incidentTurnPointer(incident *IncidentSession, turnID string) (*IncidentTurn, bool) {
	if incident == nil {
		return nil, false
	}
	for idx := range incident.Turns {
		if incident.Turns[idx].TurnID == turnID {
			return &incident.Turns[idx], true
		}
	}
	return nil, false
}

func incidentStatusForResponse(response ExecutionResponse, err error) IncidentStatus {
	if response.ApprovalRequired {
		return IncidentStatusWaitingApproval
	}
	if response.Status == protocol.ResultStatusDegraded {
		return IncidentStatusDegraded
	}
	if err != nil || response.Status == protocol.ResultStatusFailed {
		return IncidentStatusFailed
	}
	return IncidentStatusCompleted
}

func incidentCompletionEvent(incidentID string, turn *IncidentTurn, err error) IncidentEvent {
	eventType := "turn_completed"
	message := "排障轮次已完成"
	if turn.Status == IncidentStatusWaitingApproval {
		eventType = "approval_waiting"
		message = "排障已暂停，等待审批"
	} else if turn.Status == IncidentStatusDegraded {
		eventType = "turn_degraded"
		message = "排障轮次降级完成"
	} else if turn.Status == IncidentStatusFailed || err != nil {
		eventType = "turn_failed"
		message = "排障轮次执行失败"
	}
	return newIncidentEvent(incidentID, turn.TurnID, turn.TraceID, eventType, turn.Engine, message, map[string]any{
		"status":              turn.Status,
		"approval_request_id": turn.ApprovalRequestID,
		"degradation_reason":  turn.DegradationReason,
	})
}

func newIncidentEvent(incidentID, turnID, traceID, eventType, agent, message string, payload map[string]any) IncidentEvent {
	return IncidentEvent{
		EventID:    uuid.NewString(),
		IncidentID: incidentID,
		TurnID:     turnID,
		TraceID:    traceID,
		Type:       eventType,
		Agent:      agent,
		Message:    strings.TrimSpace(message),
		Payload:    clonePayload(payload),
		CreatedAt:  time.Now().UnixMilli(),
	}
}

func clonePayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return nil
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		out[key] = value
	}
	return out
}

func incidentTitle(query string) string {
	return previewIncident(query, 56)
}

func previewIncident(text string, max int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if max <= 0 || len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "..."
}

func writeIncidentJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func firstNonEmptyIncident(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
