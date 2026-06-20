package chat

import (
	v1 "SuperBizAgent/api/chat/v1"
	"context"
)

func (c *ControllerV1) DashboardStats(ctx context.Context, req *v1.DashboardStatsReq) (res *v1.DashboardStatsRes, err error) {
	stats := c.analyticsCollector.Stats()
	return &v1.DashboardStatsRes{
		Success: true,
		Stats:   stats,
	}, nil
}
