package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	aiservice "SuperBizAgent/internal/ai/service"
	"SuperBizAgent/internal/consts"
	"SuperBizAgent/utility/mem"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"
)

var (
	submitChatTask = aiservice.SubmitChatTask
	getChatTask    = aiservice.GetChatTask
)

func (c *ControllerV1) ChatSubmit(ctx context.Context, req *v1.ChatSubmitReq) (res *v1.ChatSubmitRes, err error) {
	id := req.Id
	msg := req.Question

	if err := mem.ValidateSessionID(id); err != nil {
		return nil, fmt.Errorf("invalid session ID: %w", err)
	}

	requestID := guid.S()
	ctx = context.WithValue(ctx, consts.CtxKeySessionID, id)
	ctx = context.WithValue(ctx, consts.CtxKeyRequestID, requestID)
	ctx = enrichRequestContext(ctx, id, requestID)

	g.Log().Infof(ctx, "[session:%s][req:%s] ChatSubmit request received, question length: %d", id, requestID, len(msg))

	if err := rejectSuspiciousPrompt(ctx, msg); err != nil {
		return nil, err
	}

	task, err := submitChatTask(ctx, id, msg)
	if err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "not enabled") || strings.Contains(lower, "not ready") || strings.Contains(lower, "unavailable") {
			writeStatus(ctx, http.StatusServiceUnavailable)
		}
		return nil, err
	}

	return &v1.ChatSubmitRes{
		TaskID:    task.ID,
		Status:    string(task.Status),
		CreatedAt: task.CreatedAt,
	}, nil
}

func (c *ControllerV1) ChatTask(ctx context.Context, req *v1.ChatTaskReq) (res *v1.ChatTaskRes, err error) {
	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("task id is empty")
	}

	record, err := getChatTask(ctx, taskID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeStatus(ctx, http.StatusNotFound)
		}
		return nil, err
	}

	sessionID := strings.TrimSpace(req.Session)
	if sessionID != "" && sessionID != strings.TrimSpace(record.SessionID) {
		writeStatus(ctx, http.StatusForbidden)
		return nil, fmt.Errorf("task %s does not belong to session %s", taskID, sessionID)
	}

	return &v1.ChatTaskRes{
		TaskID:            record.ID,
		SessionID:         record.SessionID,
		Query:             record.Query,
		Status:            string(record.Status),
		Answer:            record.Answer,
		TraceID:           record.TraceID,
		Detail:            append([]string{}, record.Detail...),
		Mode:              record.Mode,
		Degraded:          record.Degraded,
		DegradationReason: record.DegradationReason,
		Error:             record.Error,
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
		StartedAt:         record.StartedAt,
		FinishedAt:        record.FinishedAt,
	}, nil
}
