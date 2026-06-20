package tools

import (
	"SuperBizAgent/internal/ai/alertcorrelation"
	"SuperBizAgent/internal/ai/cmdb"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

type CorrelateAlertsInput struct {
	LookbackMinutes int    `json:"lookback_minutes,omitempty" jsonschema:"description=回溯分钟数，默认 60"`
	Cluster         string `json:"cluster,omitempty" jsonschema:"description=集群过滤"`
}

type CorrelateAlertsOutput struct {
	Success        bool                         `json:"success"`
	Degraded       bool                         `json:"degraded,omitempty"`
	Result         *alertcorrelation.CorrelationResult `json:"result,omitempty"`
	Message        string                       `json:"message,omitempty"`
	Error          string                       `json:"error,omitempty"`
}

func NewCorrelateAlertsTool(repo cmdb.ServiceRepository) tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"correlate_alerts",
		"分析当前告警的关联关系。自动检测告警是否沿服务依赖链传播，识别根因候选，给出故障传播路径。用于回答'这些告警有关系吗'、'哪个服务先挂的'、'故障是怎么传播的'等问题。",
		func(ctx context.Context, input *CorrelateAlertsInput, opts ...tool.Option) (output string, err error) {
			if repo == nil {
				return marshalCorrelateOutput(CorrelateAlertsOutput{
					Success:  false,
					Degraded: true,
					Error:    "CMDB not available",
					Message:  "告警关联分析需要 CMDB 服务拓扑数据，当前不可用。",
				})
			}

			lookback := input.LookbackMinutes
			if lookback <= 0 {
				lookback = 60
			}

			// 1. Fetch current alerts from Prometheus
			alerts, err := fetchPrometheusAlerts(ctx)
			if err != nil {
				return marshalCorrelateOutput(CorrelateAlertsOutput{
					Success:  false,
					Degraded: true,
					Error:    fmt.Sprintf("failed to fetch alerts: %v", err),
					Message:  "无法获取 Prometheus 告警数据。",
				})
			}

			// Filter by lookback window
			cutoff := time.Now().Add(-time.Duration(lookback) * time.Minute)
			var filtered []alertcorrelation.SimplifiedAlert
			for _, a := range alerts {
				if a.ActiveAt.After(cutoff) {
					filtered = append(filtered, a)
				}
			}

			// 2. Build topology provider from CMDB
			topo := &cmdbTopologyAdapter{repo: repo}

			// 3. Run correlation analysis
			engine := alertcorrelation.NewEngine(topo, 5, 0.7)
			result := engine.Analyze(filtered)

			return marshalCorrelateOutput(CorrelateAlertsOutput{
				Success: true,
				Result:  &result,
				Message: result.Summary,
			})
		})
	if err != nil {
		return nil
	}
	return t
}

// cmdbTopologyAdapter adapts CMDB ServiceRepository to alertcorrelation.TopologyProvider.
type cmdbTopologyAdapter struct {
	repo cmdb.ServiceRepository
}

func (a *cmdbTopologyAdapter) GetUpstream(service string) []string {
	svc, ok := a.repo.GetService(service)
	if !ok {
		return nil
	}
	return svc.Dependencies
}

func (a *cmdbTopologyAdapter) GetDownstream(service string) []string {
	return a.repo.GetDependents(service)
}

func (a *cmdbTopologyAdapter) GetAllServices() []string {
	services := a.repo.ListAll()
	names := make([]string, len(services))
	for i, s := range services {
		names[i] = s.Name
	}
	return names
}

func fetchPrometheusAlerts(ctx context.Context) ([]alertcorrelation.SimplifiedAlert, error) {
	baseURL := ""
	if v, err := g.Cfg().Get(ctx, "prometheus.address"); err == nil {
		baseURL = normalizeOptionalURL(v.String())
	}
	if baseURL == "" {
		baseURL = normalizeOptionalURL(os.Getenv("PROMETHEUS_ADDRESS"))
	}
	if baseURL == "" {
		return nil, fmt.Errorf("prometheus.address not configured")
	}

	apiURL := fmt.Sprintf("%s/api/v1/alerts", baseURL)
	timeout := 5 * time.Second
	if v, err := g.Cfg().Get(ctx, "multi_agent.metrics_query_timeout_ms"); err == nil && v.Int64() > 0 {
		timeout = time.Duration(v.Int64()) * time.Millisecond
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("prometheus returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, err
	}

	var promResp struct {
		Status string `json:"status"`
		Data   struct {
			Alerts []struct {
				Labels      map[string]string `json:"labels"`
				Annotations map[string]string `json:"annotations"`
				State       string            `json:"state"`
				ActiveAt    string            `json:"activeAt"`
			} `json:"alerts"`
		} `json:"data"`
		Error     string `json:"error"`
		ErrorType string `json:"errorType"`
	}

	if err := json.Unmarshal(body, &promResp); err != nil {
		return nil, err
	}

	if promResp.Status != "success" {
		return nil, fmt.Errorf("prometheus error: %s", promResp.Error)
	}

	var alerts []alertcorrelation.SimplifiedAlert
	for _, a := range promResp.Data.Alerts {
		activeAt, _ := time.Parse(time.RFC3339Nano, a.ActiveAt)
		alerts = append(alerts, alertcorrelation.SimplifiedAlert{
			AlertName:   a.Labels["alertname"],
			Description: a.Annotations["description"],
			State:       a.State,
			ActiveAt:    activeAt,
			Labels:      a.Labels,
		})
	}

	return alerts, nil
}

func marshalCorrelateOutput(out CorrelateAlertsOutput) (string, error) {
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"success":false,"error":"marshal failed: %s"}`, err.Error()), nil
	}
	return string(b), nil
}
