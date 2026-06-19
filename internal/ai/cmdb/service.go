package cmdb

import (
	"fmt"
	"strings"
)

// FormatServiceInfo 将单个服务格式化为可读字符串
func FormatServiceInfo(svc *ServiceInfo) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("服务: %s", svc.Name))
	if svc.DisplayName != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", svc.DisplayName))
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("  负责人: %s\n", svc.Owner))
	sb.WriteString(fmt.Sprintf("  团队: %s\n", svc.Team))
	sb.WriteString(fmt.Sprintf("  集群: %s\n", svc.Cluster))
	sb.WriteString(fmt.Sprintf("  环境: %s\n", svc.Env))
	if svc.Region != "" {
		sb.WriteString(fmt.Sprintf("  地域: %s\n", svc.Region))
	}
	if svc.Language != "" {
		sb.WriteString(fmt.Sprintf("  技术栈: %s\n", svc.Language))
	}
	if len(svc.Dependencies) > 0 {
		sb.WriteString(fmt.Sprintf("  上游依赖: %s\n", strings.Join(svc.Dependencies, ", ")))
	}
	if len(svc.Dependents) > 0 {
		sb.WriteString(fmt.Sprintf("  下游被依赖: %s\n", strings.Join(svc.Dependents, ", ")))
	}
	if svc.Description != "" {
		sb.WriteString(fmt.Sprintf("  描述: %s\n", svc.Description))
	}
	if svc.OnCall != "" {
		sb.WriteString(fmt.Sprintf("  On-Call: %s\n", svc.OnCall))
	}
	if svc.LastDeploy != "" {
		sb.WriteString(fmt.Sprintf("  最近部署: %s\n", svc.LastDeploy))
	}
	if len(svc.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("  标签: %s\n", strings.Join(svc.Tags, ", ")))
	}
	return sb.String()
}

// FormatServiceList 将服务列表格式化为可读字符串
func FormatServiceList(services []ServiceInfo) string {
	if len(services) == 0 {
		return "未找到匹配的服务"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("共 %d 个服务:\n", len(services)))
	for i, svc := range services {
		sb.WriteString(fmt.Sprintf("%d. %s", i+1, svc.Name))
		if svc.DisplayName != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", svc.DisplayName))
		}
		sb.WriteString(fmt.Sprintf(" — %s / %s", svc.Owner, svc.Team))
		if len(svc.Tags) > 0 {
			sb.WriteString(fmt.Sprintf(" [%s]", strings.Join(svc.Tags, ", ")))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
