package actionexecutor

// DefaultActions returns the standard set of predefined actions.
func DefaultActions() []*ActionDefinition {
	return []*ActionDefinition{
		{
			ID:          "query_service_status",
			Name:        "查询服务状态",
			Description: "查询指定服务的 Pod 运行状态",
			Category:    CategoryQuery,
			RiskLevel:   RiskLow,
			Executor:    "http",
			Parameters: []ActionParam{
				{Name: "service", Required: true, Description: "服务名称"},
				{Name: "namespace", Required: false, Default: "default", Description: "Kubernetes 命名空间"},
			},
			Config: map[string]string{
				"method": "GET",
				"url":    "${K8S_API_BASE}/api/v1/namespaces/{namespace}/pods?labelSelector=app={service}",
			},
		},
		{
			ID:          "restart_service",
			Name:        "重启服务",
			Description: "重启指定服务的 Deployment（触发滚动更新）",
			Category:    CategoryRestart,
			RiskLevel:   RiskHigh,
			Executor:    "http",
			Parameters: []ActionParam{
				{Name: "service", Required: true, Description: "服务名称"},
				{Name: "namespace", Required: false, Default: "default", Description: "Kubernetes 命名空间"},
			},
			Config: map[string]string{
				"method":       "POST",
				"url":          "${K8S_API_BASE}/apis/apps/v1/namespaces/{namespace}/deployments/{service}/restart",
				"content_type": "application/json",
			},
		},
		{
			ID:          "scale_deployment",
			Name:        "扩缩容",
			Description: "调整指定 Deployment 的副本数",
			Category:    CategoryScale,
			RiskLevel:   RiskHigh,
			Executor:    "http",
			Parameters: []ActionParam{
				{Name: "service", Required: true, Description: "服务名称"},
				{Name: "replicas", Required: true, Description: "目标副本数"},
				{Name: "namespace", Required: false, Default: "default", Description: "Kubernetes 命名空间"},
			},
			Config: map[string]string{
				"method":       "PATCH",
				"url":          "${K8S_API_BASE}/apis/apps/v1/namespaces/{namespace}/deployments/{service}",
				"content_type": "application/merge-patch+json",
			},
		},
		{
			ID:          "query_deployment_status",
			Name:        "查询 Deployment 状态",
			Description: "查询 Deployment 的详细状态（副本数、就绪情况、更新历史）",
			Category:    CategoryQuery,
			RiskLevel:   RiskLow,
			Executor:    "http",
			Parameters: []ActionParam{
				{Name: "service", Required: true, Description: "Deployment 名称"},
				{Name: "namespace", Required: false, Default: "default", Description: "Kubernetes 命名空间"},
			},
			Config: map[string]string{
				"method": "GET",
				"url":    "${K8S_API_BASE}/apis/apps/v1/namespaces/{namespace}/deployments/{service}",
			},
		},
		{
			ID:          "rollback_deployment",
			Name:        "回滚 Deployment",
			Description: "将 Deployment 回滚到上一个修订版本",
			Category:    CategoryRollback,
			RiskLevel:   RiskHigh,
			Executor:    "http",
			Parameters: []ActionParam{
				{Name: "service", Required: true, Description: "Deployment 名称"},
				{Name: "namespace", Required: false, Default: "default", Description: "Kubernetes 命名空间"},
				{Name: "revision", Required: false, Default: "0", Description: "回滚到的修订版本（0=上一个）"},
			},
			Config: map[string]string{
				"method":       "POST",
				"url":          "${K8S_API_BASE}/apis/apps/v1/namespaces/{namespace}/deployments/{service}/rollback",
				"content_type": "application/json",
			},
		},
	}
}

// NewDefaultRegistry creates a registry with all default actions and HTTP executor.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	r.RegisterExecutor("http", NewHTTPExecutor())
	for _, action := range DefaultActions() {
		r.Register(action)
	}
	return r
}
