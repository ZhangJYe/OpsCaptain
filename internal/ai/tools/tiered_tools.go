package tools

import (
	"SuperBizAgent/internal/ai/cmdb"
	"SuperBizAgent/internal/ai/skills"
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/gogf/gf/v2/frame/g"
)

var cmdbRepository cmdb.ServiceRepository

func SetCMDBRepository(repo cmdb.ServiceRepository) {
	cmdbRepository = repo
}

func BuildTieredTools(ctx context.Context, userToolStore skills.UserSkillStore, dynamicMCPReg *DynamicMCPRegistry) []skills.TieredTool {
	if ctx == nil {
		ctx = context.Background()
	}
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

	if t := NewPrometheusMetricsDiscoveryTool(); t != nil {
		tiered = append(tiered, skills.TieredTool{
			Tool:    t,
			Tier:    skills.TierSkillGate,
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

	if cmdbRepository != nil {
		if t := NewQueryCMDBTool(cmdbRepository); t != nil {
			tiered = append(tiered, skills.TieredTool{
				Tool:    t,
				Tier:    skills.TierSkillGate,
				Domains: []string{"metrics", "logs", "knowledge"},
			})
		}
	}

	// Append approved user-defined MCP tools
	if userToolStore != nil {
		if data, loadErr := userToolStore.Load(ctx); loadErr == nil {
			for _, t := range data.Tools {
				if t.Status != skills.StatusApproved {
					continue
				}
				if dynamicMCPReg == nil {
					continue
				}
				if tool, ok := dynamicMCPReg.Get(t.ID); ok {
					tiered = append(tiered, skills.TieredTool{
						Tool:    tool,
						Tier:    skills.TierSkillGate,
						Domains: []string{skills.DomainCustom},
					})
				}
			}
		} else {
			g.Log().Warningf(ctx, "progressive disclosure: load user tools: %v", loadErr)
		}
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
