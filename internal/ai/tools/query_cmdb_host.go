package tools

import (
	"SuperBizAgent/internal/ai/cmdb"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

const (
	defaultHostCMDBMaxResults = 10
	maxHostCMDBResultsLimit   = 50
	defaultHostCMDBTimeoutMs  = 3000
)

type QueryCMDBHostInput struct {
	Action    string `json:"action" jsonschema:"description=查询动作：get_host(查单个主机), list_by_service(按服务查主机), list_by_cluster(按集群查主机), list_all(列全部)"`
	HostName  string `json:"host_name,omitempty" jsonschema:"description=主机名称，用于 get_host"`
	Service   string `json:"service,omitempty" jsonschema:"description=服务名称，用于 list_by_service"`
	Cluster   string `json:"cluster,omitempty" jsonschema:"description=集群名称，用于 list_by_cluster"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=返回结果数量上限，默认 10"`
}

type QueryCMDBHostOutput struct {
	Success  bool              `json:"success"`
	Degraded bool              `json:"degraded,omitempty"`
	Action   string            `json:"action"`
	Hosts    []cmdb.HostInfo   `json:"hosts,omitempty"`
	Host     *cmdb.HostInfo    `json:"host,omitempty"`
	Message  string            `json:"message,omitempty"`
	Error    string            `json:"error,omitempty"`
}

func NewQueryCMDBHostTool(repo cmdb.HostRepository) tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_cmdb_host",
		"Query CMDB for host information. Use this tool to look up host details, list hosts by service, list hosts by cluster, or list all hosts.",
		func(ctx context.Context, input *QueryCMDBHostInput, opts ...tool.Option) (output string, err error) {
			if repo == nil {
				out := QueryCMDBHostOutput{
					Success:  false,
					Degraded: true,
					Action:   input.Action,
					Error:    "host repository is not configured",
					Message:  "CMDB host query is unavailable. The host repository may not be configured or initialized.",
				}
				jsonBytes, _ := json.MarshalIndent(out, "", "  ")
				return string(jsonBytes), nil
			}

			if input.Limit <= 0 {
				input.Limit = defaultHostCMDBMaxResults
			}
			if input.Limit > maxHostCMDBResultsLimit {
				input.Limit = maxHostCMDBResultsLimit
			}

			timeout := time.Duration(defaultHostCMDBTimeoutMs) * time.Millisecond
			queryCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			resultCh := make(chan string, 1)
			go func() {
				resultCh <- runHostCMDBAction(repo, input)
			}()

			select {
			case result := <-resultCh:
				return result, nil
			case <-queryCtx.Done():
				out := QueryCMDBHostOutput{
					Success:  false,
					Degraded: true,
					Action:   input.Action,
					Error:    "query timeout",
					Message:  "CMDB host query timeout, please try again later.",
				}
				jsonBytes, _ := json.Marshal(out)
				return string(jsonBytes), nil
			}
		})
	if err != nil {
		return nil
	}
	return t
}

func runHostCMDBAction(repo cmdb.HostRepository, input *QueryCMDBHostInput) string {
	switch input.Action {
	case "get_host":
		return handleGetHost(repo, input)
	case "list_by_service":
		return handleHostListByService(repo, input)
	case "list_by_cluster":
		return handleHostListByCluster(repo, input)
	case "list_all":
		return handleHostListAll(repo, input)
	default:
		out := QueryCMDBHostOutput{
			Success:  false,
			Degraded: true,
			Action:   input.Action,
			Error:    fmt.Sprintf("unknown action: %s", input.Action),
			Message:  "Supported actions: get_host, list_by_service, list_by_cluster, list_all",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes)
	}
}

func handleGetHost(repo cmdb.HostRepository, input *QueryCMDBHostInput) string {
	if input.HostName == "" {
		out := QueryCMDBHostOutput{
			Success:  false,
			Degraded: true,
			Action:   "get_host",
			Error:    "host_name is required",
			Message:  "Please provide host_name parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes)
	}

	host, ok := repo.GetHost(input.HostName)
	if !ok {
		out := QueryCMDBHostOutput{
			Success:  false,
			Degraded: true,
			Action:   "get_host",
			Error:    fmt.Sprintf("host %q not found", input.HostName),
			Message:  fmt.Sprintf("Host %q does not exist in CMDB.", input.HostName),
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes)
	}

	out := QueryCMDBHostOutput{
		Success: true,
		Action:  "get_host",
		Host:    host,
		Message: fmt.Sprintf("Successfully retrieved host %q", input.HostName),
	}
	jsonBytes, _ := json.MarshalIndent(out, "", "  ")
	return string(jsonBytes)
}

func handleHostListByService(repo cmdb.HostRepository, input *QueryCMDBHostInput) string {
	if input.Service == "" {
		out := QueryCMDBHostOutput{
			Success:  false,
			Degraded: true,
			Action:   "list_by_service",
			Error:    "service is required",
			Message:  "Please provide service parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes)
	}

	hosts := repo.ListHostsByService(input.Service)
	out := QueryCMDBHostOutput{
		Success: true,
		Action:  "list_by_service",
		Hosts:   hosts,
		Message: fmt.Sprintf("Found %d hosts for service %q", len(hosts), input.Service),
	}
	jsonBytes, _ := json.MarshalIndent(out, "", "  ")
	return string(jsonBytes)
}

func handleHostListByCluster(repo cmdb.HostRepository, input *QueryCMDBHostInput) string {
	if input.Cluster == "" {
		out := QueryCMDBHostOutput{
			Success:  false,
			Degraded: true,
			Action:   "list_by_cluster",
			Error:    "cluster is required",
			Message:  "Please provide cluster parameter.",
		}
		jsonBytes, _ := json.MarshalIndent(out, "", "  ")
		return string(jsonBytes)
	}

	hosts := repo.ListHostsByCluster(input.Cluster)
	out := QueryCMDBHostOutput{
		Success: true,
		Action:  "list_by_cluster",
		Hosts:   hosts,
		Message: fmt.Sprintf("Found %d hosts in cluster %q", len(hosts), input.Cluster),
	}
	jsonBytes, _ := json.MarshalIndent(out, "", "  ")
	return string(jsonBytes)
}

func handleHostListAll(repo cmdb.HostRepository, input *QueryCMDBHostInput) string {
	hosts := repo.ListAllHosts()
	out := QueryCMDBHostOutput{
		Success: true,
		Action:  "list_all",
		Hosts:   hosts,
		Message: fmt.Sprintf("Listed all %d hosts", len(hosts)),
	}
	jsonBytes, _ := json.MarshalIndent(out, "", "  ")
	return string(jsonBytes)
}
