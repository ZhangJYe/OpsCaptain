package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

const defaultPrometheusQueryTimeout = 5 * time.Second

type PrometheusAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    string            `json:"activeAt"`
	Value       string            `json:"value"`
}

type PrometheusAlertsResult struct {
	Status string `json:"status"`
	Data   struct {
		Alerts []PrometheusAlert `json:"alerts"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

type SimplifiedAlert struct {
	AlertName   string            `json:"alert_name" jsonschema:"description=告警名称，从 Prometheus 告警的 labels.alertname 字段提取"`
	Labels      map[string]string `json:"labels,omitempty" jsonschema:"description=用于区分告警实例的 Prometheus 标签"`
	Description string            `json:"description" jsonschema:"description=告警描述信息，从 Prometheus 告警的 annotations.description 字段提取"`
	State       string            `json:"state" jsonschema:"description=告警状态，通常为 'firing'（触发中）或 'pending'（待触发）"`
	ActiveAt    string            `json:"active_at" jsonschema:"description=告警激活时间，RFC3339 格式的时间戳，例如 '2025-10-29T08:48:42.496134755Z'"`
	Duration    string            `json:"duration" jsonschema:"description=告警持续时间，从激活时间到当前时间的时长，格式如 '2h30m15s'、'30m15s' 或 '15s'"`
}

type PrometheusAlertsOutput struct {
	Success  bool              `json:"success" jsonschema:"description=查询是否成功"`
	Degraded bool              `json:"degraded,omitempty" jsonschema:"description=结果是否降级（查询失败时为 true）"`
	Alerts   []SimplifiedAlert `json:"alerts,omitempty" jsonschema:"description=活动告警列表，每个告警包含名称、标签、描述、状态、激活时间和持续时间。完整标签集合相同的告警只保留第一个"`
	Message  string            `json:"message,omitempty" jsonschema:"description=操作结果的状态消息"`
	Error    string            `json:"error,omitempty" jsonschema:"description=如果查询失败，包含错误信息"`
}

type PrometheusAlertsInput struct {
	Query string `json:"query,omitempty" jsonschema:"description=可选的服务名或 case_id，用于只返回与当前诊断对象匹配的告警"`
}

func queryPrometheusAlerts(ctx context.Context) (PrometheusAlertsResult, error) {
	var result PrometheusAlertsResult
	baseURL := ""
	if v, err := g.Cfg().Get(ctx, "prometheus.address"); err == nil {
		baseURL = normalizeOptionalURL(v.String())
	}
	if baseURL == "" {
		baseURL = normalizeOptionalURL(os.Getenv("PROMETHEUS_ADDRESS"))
	}
	if baseURL == "" {
		return result, fmt.Errorf("prometheus.address is not configured")
	}
	apiURL := fmt.Sprintf("%s/api/v1/alerts", baseURL)

	g.Log().Debugf(ctx, "querying Prometheus alerts: %s", apiURL)

	timeout := prometheusQueryTimeout(ctx)
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return result, fmt.Errorf("failed to build prometheus request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("failed to query Prometheus alerts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return result, fmt.Errorf("prometheus returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return result, fmt.Errorf("failed to read response: %w", err)
	}

	if err = json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return result, fmt.Errorf("prometheus query error: type=%s msg=%s", result.ErrorType, result.Error)
	}

	return result, nil
}

func prometheusQueryTimeout(ctx context.Context) time.Duration {
	v, err := g.Cfg().Get(ctx, "multi_agent.metrics_query_timeout_ms")
	if err == nil && v.Int64() > 0 {
		return time.Duration(v.Int64()) * time.Millisecond
	}
	return defaultPrometheusQueryTimeout
}

func calculateDuration(activeAtStr string) string {
	activeAt, err := time.Parse(time.RFC3339Nano, activeAtStr)
	if err != nil {
		return "unknown"
	}

	duration := time.Since(activeAt)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
	} else if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	} else {
		return fmt.Sprintf("%ds", seconds)
	}
}

func NewPrometheusAlertsQueryTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_prometheus_alerts",
		"Query active alerts from Prometheus alerting system. This tool retrieves all currently active/firing alerts including their labels, annotations, state, and values. Use this tool when you need to check what alerts are currently firing, investigate alert conditions, or monitor alert status.",
		func(ctx context.Context, input *PrometheusAlertsInput, opts ...tool.Option) (output string, err error) {
			g.Log().Infof(ctx, "querying Prometheus active alerts")

			result, err := queryPrometheusAlerts(ctx)
			if err != nil {
				alertsOut := PrometheusAlertsOutput{
					Success:  false,
					Degraded: true,
					Error:    err.Error(),
					Message:  "Failed to query Prometheus alerts. The service may not be configured or is unreachable.",
				}
				jsonBytes, marshalErr := json.MarshalIndent(alertsOut, "", "  ")
				if marshalErr != nil {
					return fmt.Sprintf(`{"success":false,"degraded":true,"error":"%s"}`, err.Error()), nil
				}
				return string(jsonBytes), nil
			}

			maxResults := g.Cfg().MustGet(ctx, "prometheus.alerts_max_results", 50).Int()
			query := ""
			if input != nil {
				query = input.Query
			}
			filteredAlerts := filterPrometheusAlerts(result.Data.Alerts, query)
			simplifiedAlerts := simplifyPrometheusAlerts(filteredAlerts, maxResults)

			alertsOut := PrometheusAlertsOutput{
				Success: true,
				Alerts:  simplifiedAlerts,
				Message: fmt.Sprintf("Successfully retrieved %d active alerts", len(simplifiedAlerts)),
			}

			jsonBytes, err := json.MarshalIndent(alertsOut, "", "  ")
			if err != nil {
				g.Log().Errorf(ctx, "error marshaling alerts result to JSON: %v", err)
				return "", err
			}

			g.Log().Infof(ctx, "Prometheus alerts query completed: %d alerts found", len(simplifiedAlerts))
			return string(jsonBytes), nil
		})
	if err != nil {
		g.Log().Warningf(context.Background(), "failed to create query_prometheus_alerts tool, will be unavailable: %v", err)
		return nil
	}
	return t
}

func filterPrometheusAlerts(alerts []PrometheusAlert, query string) []PrometheusAlert {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return alerts
	}
	out := make([]PrometheusAlert, 0, len(alerts))
	for _, alert := range alerts {
		candidates := []string{
			alert.Labels["service"],
			alert.Labels["case_id"],
			alert.Labels["namespace"],
			alert.Labels["pod"],
			alert.Annotations["summary"],
			alert.Annotations["description"],
		}
		matched := false
		for _, candidate := range candidates {
			candidate = strings.ToLower(strings.TrimSpace(candidate))
			if candidate != "" && (strings.Contains(query, candidate) || strings.Contains(candidate, query)) {
				matched = true
				break
			}
		}
		if matched {
			out = append(out, alert)
		}
	}
	return out
}

func simplifyPrometheusAlerts(alerts []PrometheusAlert, maxResults int) []SimplifiedAlert {
	if maxResults <= 0 {
		maxResults = 50
	}
	seen := make(map[string]struct{}, len(alerts))
	out := make([]SimplifiedAlert, 0, min(len(alerts), maxResults))
	for _, alert := range alerts {
		identity, _ := json.Marshal(alert.Labels)
		key := string(identity)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		labels := make(map[string]string, len(alert.Labels))
		for name, value := range alert.Labels {
			labels[name] = value
		}
		out = append(out, SimplifiedAlert{
			AlertName:   alert.Labels["alertname"],
			Labels:      labels,
			Description: alert.Annotations["description"],
			State:       alert.State,
			ActiveAt:    alert.ActiveAt,
			Duration:    calculateDuration(alert.ActiveAt),
		})
		if len(out) == maxResults {
			break
		}
	}
	return out
}
