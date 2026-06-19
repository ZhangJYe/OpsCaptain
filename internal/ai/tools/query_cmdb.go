package tools

import (
	"SuperBizAgent/internal/ai/cmdb"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultCMDBMaxResults = 10
	maxCMDBResultsLimit   = 50
	defaultCMDBTimeoutMs  = 3000
)

type QueryCMDBInput struct {
	Action      string `json:"action" jsonschema:"description=查询动作：get_service(查单个服务), search(模糊搜索), list_by_cluster(集群查询), list_by_team(团队查询), get_dependencies(查依赖), list_all(列全部)"`
	ServiceName string `json:"service_name,omitempty" jsonschema:"description=服务名称，用于 get_service / get_dependencies"`
	Cluster     string `json:"cluster,omitempty" jsonschema:"description=集群名称，用于 list_by_cluster"`
	Team        string `json:"team,omitempty" jsonschema:"description=团队名称，用于 list_by_team"`
	Keyword     string `json:"keyword,omitempty" jsonschema:"description=搜索关键词，用于 search"`
	Limit       int    `json:"limit,omitempty" jsonschema:"description=返回结果数量上限，默认 10"`
}

type QueryCMDBOutput struct {
	Success    bool               `json:"success"`
	Degraded   bool               `json:"degraded,omitempty"`
	Action     string             `json:"action"`
	Services   []cmdb.ServiceInfo `json:"services,omitempty"`
	Service    *cmdb.ServiceInfo  `json:"service,omitempty"`
	Upstream   []string           `json:"upstream,omitempty"`
	Downstream []string           `json:"downstream,omitempty"`
	Message    string             `json:"message,omitempty"`
	Error      string             `json:"error,omitempty"`
}

// UnavailableRepository implements ServiceRepository with empty results.
// Used when CMDB is disabled or YAML fails to load, so the tool is still
// registered and can return degraded responses instead of being invisible.
type UnavailableRepository struct{}

func (u *UnavailableRepository) GetService(name string) (*cmdb.ServiceInfo, bool) {
	return nil, false
}
func (u *UnavailableRepository) SearchServices(keyword string, limit int) []cmdb.ServiceInfo {
	return nil
}
func (u *UnavailableRepository) ListServicesByCluster(cluster string) []cmdb.ServiceInfo {
	return nil
}
func (u *UnavailableRepository) ListServicesByTeam(team string) []cmdb.ServiceInfo {
	return nil
}
func (u *UnavailableRepository) GetDependents(name string) []string {
	return nil
}
func (u *UnavailableRepository) ListAll() []cmdb.ServiceInfo {
	return nil
}

func NewQueryCMDBTool(repo cmdb.ServiceRepository) tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_cmdb",
		"Query CMDB (Configuration Management Database) for service information. Use this tool to look up service details, search for services, list services by cluster or team, check service dependencies, or list all registered services.",
		func(ctx context.Context, input *QueryCMDBInput, opts ...tool.Option) (output string, err error) {
			if _, unavailable := repo.(*UnavailableRepository); unavailable {
				out := QueryCMDBOutput{
					Success:  false,
					Degraded: true,
					Action:   input.Action,
					Error:    "CMDB is not available (disabled or failed to load)",
					Message:  "CMDB 未启用或数据加载失败，无法查询服务资产信息。",
				}
				jsonBytes, _ := json.Marshal(out)
				return string(jsonBytes), nil
			}
			if repo == nil {
				out := QueryCMDBOutput{
					Success:  false,
					Degraded: true,
					Action:   input.Action,
					Error:    "CMDB repository is not configured",
					Message:  "CMDB service is unavailable. The repository may not be configured or initialized.",
				}
				jsonBytes, _ := json.MarshalIndent(out, "", "  ")
				return string(jsonBytes), nil
			}

			if input.Limit <= 0 {
				input.Limit = LoadCMDBMaxResults()
			}
			if input.Limit > maxCMDBResultsLimit {
				input.Limit = maxCMDBResultsLimit
			}

			timeout := loadCMDBTimeout()
			queryCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			return runCMDBActionWithTimeout(queryCtx, repo, input)
		})
	if err != nil {
		return nil
	}
	return t
}

type cmdbActionResult struct {
	output string
	err    error
}

func runCMDBActionWithTimeout(ctx context.Context, repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	resultCh := make(chan cmdbActionResult, 1)
	go func() {
		output, err := runCMDBAction(repo, input)
		resultCh <- cmdbActionResult{output: output, err: err}
	}()

	select {
	case result := <-resultCh:
		return result.output, result.err
	case <-ctx.Done():
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   input.Action,
			Error:    "query timeout",
			Message:  "CMDB 查询超时，请稍后重试或检查 CMDB 数据源状态。",
		}
		jsonBytes, _ := json.Marshal(out)
		return string(jsonBytes), nil
	}
}

func runCMDBAction(repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	switch input.Action {
	case "get_service":
		return handleGetService(repo, input)
	case "search":
		return handleSearch(repo, input)
	case "list_by_cluster":
		return handleListByCluster(repo, input)
	case "list_by_team":
		return handleListByTeam(repo, input)
	case "get_dependencies":
		return handleGetDependencies(repo, input)
	case "list_all":
		return handleListAll(repo, input)
	default:
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   input.Action,
			Error:    fmt.Sprintf("unknown action: %s", input.Action),
			Message:  "Supported actions: get_service, search, list_by_cluster, list_by_team, get_dependencies, list_all",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}
}

// LoadCMDBMaxResults reads max_results from config, used by both tool and HTTP API
func LoadCMDBMaxResults() int {
	if v, err := g.Cfg().Get(context.Background(), "cmdb.search.max_results"); err == nil && v.Int() > 0 {
		return v.Int()
	}
	return defaultCMDBMaxResults
}

func loadCMDBTimeout() time.Duration {
	if v, err := g.Cfg().Get(context.Background(), "cmdb.tool.timeout_ms"); err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultCMDBTimeoutMs * time.Millisecond
}

func handleGetService(repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	if input.ServiceName == "" {
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   "get_service",
			Error:    "service_name is required",
			Message:  "Please provide service_name parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}

	svc, ok := repo.GetService(input.ServiceName)
	if !ok {
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   "get_service",
			Error:    fmt.Sprintf("service %q not found", input.ServiceName),
			Message:  fmt.Sprintf("Service %q does not exist in CMDB.", input.ServiceName),
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}

	out := QueryCMDBOutput{
		Success: true,
		Action:  "get_service",
		Service: svc,
		Message: fmt.Sprintf("Successfully retrieved service %q", input.ServiceName),
	}
	jsonBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func handleSearch(repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	if input.Keyword == "" {
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   "search",
			Error:    "keyword is required",
			Message:  "Please provide keyword parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}

	services := repo.SearchServices(input.Keyword, input.Limit)
	out := QueryCMDBOutput{
		Success:  true,
		Action:   "search",
		Services: services,
		Message:  fmt.Sprintf("Found %d services matching %q", len(services), input.Keyword),
	}
	jsonBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func handleListByCluster(repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	if input.Cluster == "" {
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   "list_by_cluster",
			Error:    "cluster is required",
			Message:  "Please provide cluster parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}

	services := repo.ListServicesByCluster(input.Cluster)
	out := QueryCMDBOutput{
		Success:  true,
		Action:   "list_by_cluster",
		Services: services,
		Message:  fmt.Sprintf("Found %d services in cluster %q", len(services), input.Cluster),
	}
	jsonBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func handleListByTeam(repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	if input.Team == "" {
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   "list_by_team",
			Error:    "team is required",
			Message:  "Please provide team parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}

	services := repo.ListServicesByTeam(input.Team)
	out := QueryCMDBOutput{
		Success:  true,
		Action:   "list_by_team",
		Services: services,
		Message:  fmt.Sprintf("Found %d services owned by team %q", len(services), input.Team),
	}
	jsonBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func handleGetDependencies(repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	if input.ServiceName == "" {
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   "get_dependencies",
			Error:    "service_name is required",
			Message:  "Please provide service_name parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}

	svc, ok := repo.GetService(input.ServiceName)
	if !ok {
		out := QueryCMDBOutput{
			Success:  false,
			Degraded: true,
			Action:   "get_dependencies",
			Error:    fmt.Sprintf("service %q not found", input.ServiceName),
			Message:  fmt.Sprintf("Service %q does not exist in CMDB.", input.ServiceName),
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes), nil
	}

	downstream := repo.GetDependents(input.ServiceName)
	out := QueryCMDBOutput{
		Success:    true,
		Action:     "get_dependencies",
		Service:    svc,
		Upstream:   svc.Dependencies,
		Downstream: downstream,
		Message:    fmt.Sprintf("Retrieved dependencies for %q", input.ServiceName),
	}
	jsonBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func handleListAll(repo cmdb.ServiceRepository, input *QueryCMDBInput) (string, error) {
	services := repo.ListAll()
	out := QueryCMDBOutput{
		Success:  true,
		Action:   "list_all",
		Services: services,
		Message:  fmt.Sprintf("Listed all %d services", len(services)),
	}
	jsonBytes, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}
