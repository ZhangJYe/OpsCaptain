package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"
	"context"
	"errors"
	"net/http"

	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) AIOps(ctx context.Context, req *v1.AIOpsReq) (res *v1.AIOpsRes, err error) {
	result, err := c.aiopsApp.HandleAIOps(ctx, &app.AIOpsInput{
		SessionID: req.SessionID,
		Query:     req.Query,
		Engine:    req.Engine,
	})
	if err != nil {
		var rejected *app.PromptRejectedError
		if errors.As(err, &rejected) {
			if r := g.RequestFromCtx(ctx); r != nil {
				r.Response.WriteStatus(http.StatusBadRequest)
			}
			return nil, err
		}
		return nil, err
	}
	if result.HTTPStatus != 0 {
		if r := g.RequestFromCtx(ctx); r != nil {
			r.Response.WriteStatus(result.HTTPStatus)
		}
	}

	evidence := make([]v1.EvidenceItem, 0, len(result.Evidence))
	for _, e := range result.Evidence {
		evidence = append(evidence, v1.EvidenceItem{
			SourceType: e.SourceType,
			SourceID:   e.SourceID,
			Title:      e.Title,
			Snippet:    e.Snippet,
			Score:      e.Score,
			URI:        e.URI,
		})
	}

	return &v1.AIOpsRes{
		TraceID:           result.TraceID,
		Result:            result.Result,
		Detail:            result.Detail,
		Engine:            result.Engine,
		ApprovalRequired:  result.ApprovalRequired,
		ApprovalRequestID: result.ApprovalRequestID,
		ApprovalStatus:    result.ApprovalStatus,
		ExecutionPlan:     result.ExecutionPlan,
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
		Confidence:        result.Confidence,
		Evidence:          evidence,
		NextActions:       result.NextActions,
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
	}, nil
}

func (c *ControllerV1) AIOpsTrace(ctx context.Context, req *v1.AIOpsTraceReq) (res *v1.AIOpsTraceRes, err error) {
	result, err := c.aiopsApp.HandleAIOpsTrace(ctx, req.TraceID)
	if err != nil {
		return nil, err
	}

	out := make([]v1.AIOpsTraceEvent, 0, len(result.Events))
	for _, event := range result.Events {
		if event == nil {
			continue
		}
		out = append(out, v1.AIOpsTraceEvent{
			EventID:   event.EventID,
			TaskID:    event.TaskID,
			TraceID:   event.TraceID,
			Type:      event.Type,
			Agent:     event.Agent,
			Message:   event.Message,
			Payload:   event.Payload,
			CreatedAt: event.CreatedAt,
		})
	}

	return &v1.AIOpsTraceRes{
		TraceID: req.TraceID,
		Detail:  result.Detail,
		Events:  out,
	}, nil
}

func (c *ControllerV1) AIOpsRuns(ctx context.Context, req *v1.AIOpsRunsReq) (res *v1.AIOpsRunsRes, err error) {
	result, err := c.aiopsApp.HandleAIOpsRuns(ctx, &app.AIOpsRunsInput{
		Query:  req.Query,
		Engine: req.Engine,
	})
	if err != nil {
		var rejected *app.PromptRejectedError
		if errors.As(err, &rejected) {
			if r := g.RequestFromCtx(ctx); r != nil {
				r.Response.WriteStatus(http.StatusBadRequest)
			}
			return nil, err
		}
		return nil, err
	}

	return &v1.AIOpsRunsRes{
		TraceID:           result.TraceID,
		TaskID:            result.TaskID,
		Engine:            result.Engine,
		Status:            result.Status,
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
		ApprovalRequired:  result.ApprovalRequired,
		ApprovalRequestID: result.ApprovalRequestID,
		ApprovalStatus:    result.ApprovalStatus,
	}, nil
}

func (c *ControllerV1) AIOpsResult(ctx context.Context, req *v1.AIOpsResultReq) (res *v1.AIOpsResultRes, err error) {
	result, err := c.aiopsApp.HandleAIOpsResult(ctx, req.TraceID)
	if err != nil {
		return nil, err
	}
	if !result.Found {
		return &v1.AIOpsResultRes{Found: false, TraceID: req.TraceID}, nil
	}

	evidence := make([]v1.EvidenceItem, 0, len(result.Evidence))
	for _, e := range result.Evidence {
		evidence = append(evidence, v1.EvidenceItem{
			SourceType: e.SourceType,
			SourceID:   e.SourceID,
			Title:      e.Title,
			Snippet:    e.Snippet,
			Score:      e.Score,
			URI:        e.URI,
		})
	}

	return &v1.AIOpsResultRes{
		Found:             true,
		Status:            result.Status,
		TraceID:           result.TraceID,
		Result:            result.Result,
		Detail:            result.Detail,
		Engine:            result.Engine,
		Confidence:        result.Confidence,
		Evidence:          evidence,
		NextActions:       result.NextActions,
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
	}, nil
}
