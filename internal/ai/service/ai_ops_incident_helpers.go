package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"SuperBizAgent/internal/ai/protocol"
)

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

func incidentStoreDir(ctx context.Context) string {
	if dir, ok := incidentConfigString(ctx, "aiops.incident.store_dir"); ok {
		return dir
	}
	if dir, ok := incidentConfigString(ctx, "multi_agent.data_dir"); ok {
		return filepath.Join(dir, "incidents")
	}
	return filepath.Join(".", "var", "runtime", "incidents")
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

func firstNonEmptyIncident(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
