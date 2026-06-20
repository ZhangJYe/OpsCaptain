package v1

import "github.com/gogf/gf/v2/frame/g"

type DashboardStatsReq struct {
	g.Meta `path:"/analytics/dashboard" method:"get" summary:"获取运营数据概览"`
}

type DashboardStatsRes struct {
	Success bool        `json:"success"`
	Stats   interface{} `json:"stats,omitempty"`
}
