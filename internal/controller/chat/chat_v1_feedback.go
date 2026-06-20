package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"SuperBizAgent/internal/ai/feedback"
	"context"
)

func (c *ControllerV1) FeedbackSubmit(ctx context.Context, req *v1.FeedbackSubmitReq) (res *v1.FeedbackSubmitRes, err error) {
	entry := &feedback.FeedbackEntry{
		SessionID: req.SessionID,
		Query:     req.Query,
		Rating:    feedback.FeedbackRating(req.Rating),
		Comment:   req.Comment,
		TraceID:   req.TraceID,
	}
	if err := c.feedbackStore.Submit(entry); err != nil {
		return nil, err
	}
	return &v1.FeedbackSubmitRes{
		Success: true,
		ID:      entry.ID,
	}, nil
}

func (c *ControllerV1) FeedbackStats(ctx context.Context, req *v1.FeedbackStatsReq) (res *v1.FeedbackStatsRes, err error) {
	stats := c.feedbackStore.Stats()
	return &v1.FeedbackStatsRes{
		Success: true,
		Stats:   stats,
	}, nil
}
