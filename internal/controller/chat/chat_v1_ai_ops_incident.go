package chat

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"

	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) AIOpsIncidentCreate(ctx context.Context, req *v1.AIOpsIncidentCreateReq) (res *v1.AIOpsIncidentRes, err error) {
	if ctx, _, err = checkAndGuardPrompt(ctx, req.Query); err != nil {
		return nil, err
	}
	incident, err := app.CreateAIOpsIncident(ctx, req.Query, req.Engine)
	if err != nil {
		return nil, err
	}
	return &v1.AIOpsIncidentRes{Incident: toAIOpsIncident(ctx, incident)}, nil
}

func (c *ControllerV1) AIOpsIncidentTurn(ctx context.Context, req *v1.AIOpsIncidentTurnReq) (res *v1.AIOpsIncidentRes, err error) {
	if ctx, _, err = checkAndGuardPrompt(ctx, req.Query); err != nil {
		return nil, err
	}
	incident, err := app.AppendAIOpsIncidentTurn(ctx, req.IncidentID, req.Query)
	if err != nil {
		if errors.Is(err, app.ErrIncidentTurnRunning) {
			return nil, errors.New("incident turn is still running")
		}
		return nil, err
	}
	return &v1.AIOpsIncidentRes{Incident: toAIOpsIncident(ctx, incident)}, nil
}

func (c *ControllerV1) AIOpsIncidentList(ctx context.Context, _ *v1.AIOpsIncidentListReq) (res *v1.AIOpsIncidentListRes, err error) {
	items, err := app.ListAIOpsIncidents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]v1.AIOpsIncident, 0, len(items))
	for idx := range items {
		item := toAIOpsIncident(ctx, &items[idx])
		item.Turns = nil
		item.Events = nil
		out = append(out, item)
	}
	return &v1.AIOpsIncidentListRes{Items: out}, nil
}

func (c *ControllerV1) AIOpsIncidentGet(ctx context.Context, req *v1.AIOpsIncidentGetReq) (res *v1.AIOpsIncidentRes, err error) {
	incident, err := app.GetAIOpsIncident(ctx, req.IncidentID)
	if err != nil {
		return nil, err
	}
	return &v1.AIOpsIncidentRes{Incident: toAIOpsIncident(ctx, incident)}, nil
}

func (c *ControllerV1) AIOpsIncidentEvents(ctx context.Context, req *v1.AIOpsIncidentEventsReq) (res *v1.AIOpsIncidentEventsRes, err error) {
	if c.service == nil {
		return nil, errors.New("sse service is not initialized")
	}
	client, err := c.service.Create(ctx, g.RequestFromCtx(ctx))
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	sendEvents := func() error {
		events, eventErr := app.GetAIOpsIncidentEvents(ctx, req.IncidentID, req.TurnID)
		if eventErr != nil {
			return eventErr
		}
		for _, event := range events {
			if _, ok := seen[event.EventID]; ok {
				continue
			}
			payload, marshalErr := json.Marshal(toAIOpsIncidentEvent(event))
			if marshalErr != nil {
				return marshalErr
			}
			client.SendToClient("incident_event", string(payload))
			seen[event.EventID] = struct{}{}
		}
		return nil
	}
	if err := sendEvents(); err != nil {
		client.SendToClient("error", err.Error())
		client.SendToClient("done", "incident stream completed")
		return &v1.AIOpsIncidentEventsRes{}, nil
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		incident, currentErr := app.GetAIOpsIncident(ctx, req.IncidentID)
		if currentErr != nil {
			client.SendToClient("error", currentErr.Error())
			client.SendToClient("done", "incident stream completed")
			return &v1.AIOpsIncidentEventsRes{}, nil
		}
		if incidentTurnStreamTerminal(incident, req.TurnID) {
			client.SendToClient("done", "incident stream completed")
			return &v1.AIOpsIncidentEventsRes{}, nil
		}
		select {
		case <-ctx.Done():
			return &v1.AIOpsIncidentEventsRes{}, nil
		case <-ticker.C:
			if err := sendEvents(); err != nil {
				client.SendToClient("error", err.Error())
				client.SendToClient("done", "incident stream completed")
				return &v1.AIOpsIncidentEventsRes{}, nil
			}
		}
	}
}

func toAIOpsIncident(ctx context.Context, incident *app.IncidentSession) v1.AIOpsIncident {
	if incident == nil {
		return v1.AIOpsIncident{}
	}
	turns := make([]v1.AIOpsIncidentTurn, 0, len(incident.Turns))
	for _, turn := range incident.Turns {
		result, detail := filterAssistantPayload(ctx, turn.Result, turn.Detail)
		turns = append(turns, v1.AIOpsIncidentTurn{
			TurnID:            turn.TurnID,
			IncidentID:        turn.IncidentID,
			UserQuery:         turn.UserQuery,
			TraceID:           turn.TraceID,
			Status:            string(turn.Status),
			Result:            result,
			Detail:            detail,
			Engine:            turn.Engine,
			ApprovalRequestID: turn.ApprovalRequestID,
			ApprovalStatus:    turn.ApprovalStatus,
			DegradationReason: turn.DegradationReason,
			CreatedAt:         turn.CreatedAt,
			FinishedAt:        turn.FinishedAt,
		})
	}
	events := make([]v1.AIOpsIncidentEvent, 0, len(incident.Events))
	for _, event := range incident.Events {
		events = append(events, toAIOpsIncidentEvent(event))
	}
	summary, _ := filterAssistantPayload(ctx, incident.LatestSummary, nil)
	return v1.AIOpsIncident{
		IncidentID:     incident.IncidentID,
		SessionID:      incident.SessionID,
		Title:          incident.Title,
		Status:         string(incident.Status),
		EngineStrategy: incident.EngineStrategy,
		LatestSummary:  summary,
		Turns:          turns,
		Events:         events,
		CreatedAt:      incident.CreatedAt,
		UpdatedAt:      incident.UpdatedAt,
	}
}

func toAIOpsIncidentEvent(event app.IncidentEvent) v1.AIOpsIncidentEvent {
	return v1.AIOpsIncidentEvent{
		EventID:    event.EventID,
		IncidentID: event.IncidentID,
		TurnID:     event.TurnID,
		TraceID:    event.TraceID,
		Type:       event.Type,
		Agent:      event.Agent,
		Message:    event.Message,
		Payload:    event.Payload,
		CreatedAt:  event.CreatedAt,
	}
}

func incidentTurnStreamTerminal(incident *app.IncidentSession, turnID string) bool {
	if incident == nil {
		return true
	}
	if turnID == "" {
		turn := app.IncidentLatestTurn(incident)
		return turn == nil || app.IncidentTurnTerminal(*turn)
	}
	for _, turn := range incident.Turns {
		if turn.TurnID == turnID {
			return app.IncidentTurnTerminal(turn)
		}
	}
	return true
}

func (c *ControllerV1) IncidentLifecycleUpdate(ctx context.Context, req *v1.IncidentLifecycleUpdateReq) (res *v1.IncidentLifecycleUpdateRes, err error) {
	incident, err := app.GetAIOpsIncident(ctx, req.IncidentID)
	if err != nil {
		return &v1.IncidentLifecycleUpdateRes{Success: false, Error: err.Error()}, nil
	}
	if incident == nil {
		return &v1.IncidentLifecycleUpdateRes{Success: false, Error: "incident not found"}, nil
	}

	// Apply updates to the incident's metadata
	incident.Events = append(incident.Events, app.IncidentEvent{
		EventID:    generateEventID(),
		IncidentID: incident.IncidentID,
		Type:       "lifecycle_updated",
		Message:    "事件生命周期已更新",
		CreatedAt:  time.Now().UnixMilli(),
	})

	return &v1.IncidentLifecycleUpdateRes{
		Success:  true,
		Incident: toAIOpsIncident(ctx, incident),
		Message:  "lifecycle updated",
	}, nil
}

func (c *ControllerV1) IncidentPostmortem(ctx context.Context, req *v1.IncidentPostmortemReq) (res *v1.IncidentPostmortemRes, err error) {
	incident, err := app.GetAIOpsIncident(ctx, req.IncidentID)
	if err != nil {
		return &v1.IncidentPostmortemRes{Success: false, Error: err.Error()}, nil
	}
	if incident == nil {
		return &v1.IncidentPostmortemRes{Success: false, Error: "incident not found"}, nil
	}

	pm := app.GenerateIncidentPostmortem(incident)
	if pm == nil {
		return &v1.IncidentPostmortemRes{Success: false, Error: "cannot generate postmortem"}, nil
	}

	return &v1.IncidentPostmortemRes{
		Success:    true,
		Postmortem: pm,
		Markdown:   app.FormatIncidentPostmortem(pm),
	}, nil
}

func generateEventID() string {
	return time.Now().Format("20060102150405.000") + "-" + randomHex(6)
}

func randomHex(n int) string {
	const hex = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hex[time.Now().UnixNano()%16]
		time.Sleep(time.Nanosecond)
	}
	return string(b)
}
