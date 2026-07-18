package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/app"
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func (c *ControllerV1) Chat(ctx context.Context, req *v1.ChatReq) (res *v1.ChatRes, err error) {
	startMs := time.Now().UnixMilli()
	result, err := c.chatApp.HandleChat(ctx, &app.ChatInput{
		SessionID: req.Id,
		Question:  req.Question,
		SkillIDs:  req.SelectedSkillIds,
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
	// Record analytics
	if c.analyticsCollector != nil {
		durationMs := time.Now().UnixMilli() - startMs
		c.analyticsCollector.RecordQuery(req.Question, result.Mode, nil, durationMs)
	}
	return &v1.ChatRes{
		Answer:            result.Answer,
		Detail:            result.Detail,
		TraceID:           result.TraceID,
		Mode:              result.Mode,
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
		Cached:            result.Cached,
	}, nil
}

func (c *ControllerV1) Agent(ctx context.Context, req *v1.AgentReq) (res *v1.AgentRes, err error) {
	startMs := time.Now().UnixMilli()
	result, err := c.agentApp.HandleAgent(ctx, &app.AgentInput{
		SessionID: req.SessionID,
		Query:     req.Query,
		Mode:      app.AgentMode(req.Mode),
		SkillIDs:  req.SelectedSkillIds,
	})
	if err != nil {
		var rejected *app.PromptRejectedError
		if errors.As(err, &rejected) {
			if r := g.RequestFromCtx(ctx); r != nil {
				r.Response.WriteStatus(http.StatusBadRequest)
			}
		}
		return nil, err
	}
	if result.HTTPStatus != 0 {
		if r := g.RequestFromCtx(ctx); r != nil {
			r.Response.WriteStatus(result.HTTPStatus)
		}
	}
	if c.analyticsCollector != nil {
		c.analyticsCollector.RecordQuery(req.Query, string(result.Mode), nil, time.Now().UnixMilli()-startMs)
	}
	return mapAgentResult(result), nil
}

func mapAgentResult(result *app.AgentResult) *v1.AgentRes {
	res := &v1.AgentRes{
		TraceID:           result.TraceID,
		Mode:              string(result.Mode),
		Degraded:          result.Degraded,
		DegradationReason: result.DegradationReason,
	}
	if result.Chat != nil {
		res.Chat = &v1.AgentChatPayload{
			Answer: result.Chat.Answer,
			Detail: result.Chat.Detail,
			Cached: result.Chat.Cached,
		}
	}
	if result.Diagnosis != nil {
		res.Diagnosis = mapAgentDiagnosis(result.Diagnosis)
	}
	return res
}

func mapAgentDiagnosis(result *app.AIOpsResult) *v1.AgentDiagnosisPayload {
	evidence := make([]v1.EvidenceItem, 0, len(result.Evidence))
	for _, item := range result.Evidence {
		evidence = append(evidence, v1.EvidenceItem{
			SourceType: item.SourceType,
			SourceID:   item.SourceID,
			Title:      item.Title,
			Snippet:    item.Snippet,
			Score:      item.Score,
			URI:        item.URI,
		})
	}
	return &v1.AgentDiagnosisPayload{
		Result:            result.Result,
		Detail:            result.Detail,
		ApprovalRequired:  result.ApprovalRequired,
		ApprovalRequestID: result.ApprovalRequestID,
		ApprovalStatus:    result.ApprovalStatus,
		ExecutionPlan:     result.ExecutionPlan,
		Confidence:        result.Confidence,
		Evidence:          evidence,
		NextActions:       result.NextActions,
		StartedAt:         result.StartedAt,
		FinishedAt:        result.FinishedAt,
	}
}
