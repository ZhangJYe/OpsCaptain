package tools

import (
	"SuperBizAgent/internal/ai/skills"
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
)

func BuildTieredTools() []skills.TieredTool {
	ctx := context.Background()
	var tiered []skills.TieredTool

	if t := NewGetCurrentTimeTool(); t != nil {
		tiered = append(tiered, skills.TieredTool{
			Tool:    t,
			Tier:    skills.TierAlwaysOn,
			Domains: nil,
		})
	}

	if t := NewQueryInternalDocsTool(); t != nil {
		tiered = append(tiered, skills.TieredTool{
			Tool:    t,
			Tier:    skills.TierAlwaysOn,
			Domains: nil,
		})
	}

	mcpTools, err := GetLogMcpTool()
	if err != nil {
		g.Log().Warningf(ctx, "progressive disclosure: MCP log tools unavailable: %v", err)
	}
	for _, mt := range mcpTools {
		tiered = append(tiered, skills.TieredTool{
			Tool:    mt,
			Tier:    skills.TierAlwaysOn,
			Domains: []string{"logs"},
		})
	}

	if t := NewPrometheusAlertsQueryTool(); t != nil {
		tiered = append(tiered, skills.TieredTool{
			Tool:    t,
			Tier:    skills.TierAlwaysOn,
			Domains: []string{"metrics"},
		})
	}

	if t := NewPrometheusRangeQueryTool(); t != nil {
		tiered = append(tiered, skills.TieredTool{
			Tool:    t,
			Tier:    skills.TierSkillGate,
			Domains: []string{"metrics"},
		})
	}

	if t := NewPrometheusInstantQueryTool(); t != nil {
		tiered = append(tiered, skills.TieredTool{
			Tool:    t,
			Tier:    skills.TierSkillGate,
			Domains: []string{"metrics"},
		})
	}

	if MySQLToolEnabled() {
		if t := NewMysqlCrudTool(); t != nil {
			tiered = append(tiered, skills.TieredTool{
				Tool:    t,
				Tier:    skills.TierOnDemand,
				Domains: []string{"logs", "metrics", "knowledge"},
			})
		}
	} else {
		g.Log().Warningf(ctx, "progressive disclosure: mysql tool disabled because mysql.allowed_tables is empty")
	}

	return tiered
}

func ToolNames(tools []tool.BaseTool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		info, err := t.Info(nil)
		if err != nil || info == nil {
			g.Log().Warningf(context.Background(), "ToolNames: failed to get tool info: err=%v info=%v", err, info)
			continue
		}
		names = append(names, info.Name)
	}
	return names
}
