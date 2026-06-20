package v1

import "github.com/gogf/gf/v2/frame/g"

type FeedbackSubmitReq struct {
	g.Meta    `path:"/feedback" method:"post" summary:"提交反馈"`
	SessionID string `json:"session_id" v:"required"`
	Query     string `json:"query" v:"required"`
	Rating    string `json:"rating" v:"required|in:helpful,not_helpful"`
	Comment   string `json:"comment,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
}

type FeedbackSubmitRes struct {
	Success bool   `json:"success"`
	ID      string `json:"id,omitempty"`
}

type FeedbackStatsReq struct {
	g.Meta `path:"/feedback/stats" method:"get" summary:"获取反馈统计"`
}

type FeedbackStatsRes struct {
	Success bool        `json:"success"`
	Stats   interface{} `json:"stats,omitempty"`
}
