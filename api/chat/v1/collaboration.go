package v1

import "github.com/gogf/gf/v2/frame/g"

type ShareCreateReq struct {
	g.Meta    `path:"/share" method:"post" summary:"创建分享链接"`
	SessionID string `json:"session_id" v:"required"`
	TTLHours  int    `json:"ttl_hours,omitempty" d:"24"`
}

type ShareCreateRes struct {
	Success bool        `json:"success"`
	Share   interface{} `json:"share,omitempty"`
	URL     string      `json:"url,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type ShareGetReq struct {
	g.Meta  `path:"/share/{share_id}" method:"get" summary:"查看分享内容"`
	ShareID string `json:"share_id" v:"required"`
}

type ShareGetRes struct {
	Success bool        `json:"success"`
	Session interface{} `json:"session,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type ShareRevokeReq struct {
	g.Meta  `path:"/share/{share_id}" method:"delete" summary:"撤销分享链接"`
	ShareID string `json:"share_id" v:"required"`
}

type ShareRevokeRes struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
