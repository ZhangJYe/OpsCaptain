package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"context"
)

func (c *ControllerV1) DashboardStats(ctx context.Context, req *v1.DashboardStatsReq) (res *v1.DashboardStatsRes, err error) {
	// Sync feedback score into analytics collector
	if c.feedbackStore != nil && c.analyticsCollector != nil {
		fbStats := c.feedbackStore.Stats()
		c.analyticsCollector.SetFeedbackScore(fbStats.Score)
	}
	stats := c.analyticsCollector.Stats()
	return &v1.DashboardStatsRes{
		Success: true,
		Stats:   stats,
	}, nil
}
