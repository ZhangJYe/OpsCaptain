package tools

import (
	"SuperBizAgent/internal/ai/skills"

	"github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
)

func BuildTieredTools() []skills.TieredTool {
	var tiered []skills.TieredTool

	tiered = append(tiered, skills.TieredTool{
		Tool:    NewGetCurrentTimeTool(),
		Tier:    skills.TierAlwaysOn,
		Domains: nil,
	})

	tiered = append(tiered, skills.TieredTool{
		Tool:    NewQueryInternalDocsTool(),
		Tier:    skills.TierAlwaysOn,
		Domains: nil,
	})

	mcpTools, err := GetLogMcpTool()
	if err != nil {
		g.Log().Warningf(nil, "progressive disclosure: MCP log tools unavailable: %v", err)
	}
	for _, mt := range mcpTools {
		tiered = append(tiered, skills.TieredTool{
			Tool:    mt,
			Tier:    skills.TierSkillGate,
			Domains: []string{"logs"},
		})
	}

	tiered = append(tiered, skills.TieredTool{
		Tool:    NewPrometheusAlertsQueryTool(),
		Tier:    skills.TierSkillGate,
		Domains: []string{"metrics"},
	})

	if MySQLToolEnabled() {
		tiered = append(tiered, skills.TieredTool{
			Tool:    NewMysqlCrudTool(),
			Tier:    skills.TierOnDemand,
			Domains: []string{"logs", "metrics", "knowledge"},
		})
	} else {
		g.Log().Warningf(nil, "progressive disclosure: mysql tool disabled because mysql.allowed_tables is empty")
	}

	return tiered
}

func ToolNames(tools []tool.BaseTool) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		info, _ := t.Info(nil)
		names = append(names, info.Name)
	}
	return names
}
