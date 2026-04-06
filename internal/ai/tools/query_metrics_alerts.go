package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	AlertName   string `json:"alert_name" jsonschema:"description=告警名称，从 Prometheus 告警的 labels.alertname 字段提取"`
	Description string `json:"description" jsonschema:"description=告警描述信息，从 Prometheus 告警的 annotations.description 字段提取"`
	State       string `json:"state" jsonschema:"description=告警状态，通常为 'firing'（触发中）或 'pending'（待触发）"`
	ActiveAt    string `json:"active_at" jsonschema:"description=告警激活时间，RFC3339 格式的时间戳，例如 '2025-10-29T08:48:42.496134755Z'"`
	Duration    string `json:"duration" jsonschema:"description=告警持续时间，从激活时间到当前时间的时长，格式如 '2h30m15s'、'30m15s' 或 '15s'"`
}

type PrometheusAlertsOutput struct {
	Success bool              `json:"success" jsonschema:"description=查询是否成功"`
	Alerts  []SimplifiedAlert `json:"alerts,omitempty" jsonschema:"description=活动告警列表，每个告警包含名称、描述、状态、激活时间和持续时间。相同 alertname 的告警只保留第一个"`
	Message string            `json:"message,omitempty" jsonschema:"description=操作结果的状态消息"`
	Error   string            `json:"error,omitempty" jsonschema:"description=如果查询失败，包含错误信息"`
}

func queryPrometheusAlerts(ctx context.Context) (PrometheusAlertsResult, error) {
	baseURLVal, err := g.Cfg().Get(ctx, "prometheus.address")
	var result PrometheusAlertsResult
	if err != nil || baseURLVal.String() == "" {
		return result, fmt.Errorf("prometheus.address is not configured")
	}
	apiURL := fmt.Sprintf("%s/api/v1/alerts", baseURLVal.String())

	g.Log().Debugf(ctx, "querying Prometheus alerts: %s", apiURL)

	timeout := prometheusQueryTimeout(ctx)
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := &http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return result, fmt.Errorf("failed to build prometheus request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return result, fmt.Errorf("failed to query Prometheus alerts: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return result, fmt.Errorf("failed to read response: %v", err)
	}

	if err = json.Unmarshal(body, &result); err != nil {
		return result, fmt.Errorf("failed to parse response: %v", err)
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
		func(ctx context.Context, input *struct{}, opts ...tool.Option) (output string, err error) {
			g.Log().Infof(ctx, "querying Prometheus active alerts")

			result, err := queryPrometheusAlerts(ctx)
			if err != nil {
				alertsOut := PrometheusAlertsOutput{
					Success: false,
					Error:   err.Error(),
					Message: "Failed to query Prometheus alerts. The service may not be configured or is unreachable.",
				}
				jsonBytes, _ := json.MarshalIndent(alertsOut, "", "  ")
				return string(jsonBytes), nil
			}

			seenAlertNames := make(map[string]bool)
			simplifiedAlerts := make([]SimplifiedAlert, 0)
			for _, alert := range result.Data.Alerts {
				alertName := alert.Labels["alertname"]

				if seenAlertNames[alertName] {
					continue
				}

				seenAlertNames[alertName] = true

				simplified := SimplifiedAlert{
					AlertName:   alertName,
					Description: alert.Annotations["description"],
					State:       alert.State,
					ActiveAt:    alert.ActiveAt,
					Duration:    calculateDuration(alert.ActiveAt),
				}
				simplifiedAlerts = append(simplifiedAlerts, simplified)
			}

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
		panic(fmt.Sprintf("failed to create query_prometheus_alerts tool: %v", err))
	}
	return t
}
