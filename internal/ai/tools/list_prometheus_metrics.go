package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultPrometheusDiscoveryTimeout        = 5 * time.Second
	defaultPrometheusDiscoveryWindow         = 60 * time.Minute
	defaultPrometheusDiscoveryMaxWindow      = 6 * time.Hour
	defaultPrometheusDiscoveryMaxMetrics     = 50
	defaultPrometheusDiscoveryMaxSeries      = 50
	defaultPrometheusDiscoveryMaxLabelValues = 20
	defaultPrometheusDiscoveryReadBodyLimit  = 10 * 1024 * 1024
)

type PrometheusDiscoveryInput struct {
	Keyword string `json:"keyword,omitempty" jsonschema:"description=Optional case-insensitive keyword used to filter metric names or series labels, for example http, latency, cpu, memory, mysql, redis."`
	Match   string `json:"match,omitempty" jsonschema:"description=Optional Prometheus series selector, for example up, http_requests_total, or http_requests_total{service=\"checkout\"}. When set, the tool returns label keys and label values for matching series."`
	Start   string `json:"start,omitempty" jsonschema:"description=Optional discovery start time, RFC3339 or Unix seconds. Defaults to end minus configured discovery window."`
	End     string `json:"end,omitempty" jsonschema:"description=Optional discovery end time, RFC3339 or Unix seconds. Defaults to now."`
	Limit   int    `json:"limit,omitempty" jsonschema:"description=Optional result limit. Capped by configured prometheus.discovery_max_metrics or prometheus.discovery_max_series."`
}

type PrometheusDiscoveryOutput struct {
	Success     bool                          `json:"success"`
	Degraded    bool                          `json:"degraded,omitempty"`
	Keyword     string                        `json:"keyword,omitempty"`
	Match       string                        `json:"match,omitempty"`
	Start       string                        `json:"start,omitempty"`
	End         string                        `json:"end,omitempty"`
	Metrics     []string                      `json:"metrics,omitempty"`
	Series      []PrometheusDiscoveredSeries  `json:"series,omitempty"`
	LabelKeys   []string                      `json:"label_keys,omitempty"`
	LabelValues map[string][]string           `json:"label_values,omitempty"`
	Summary     *PrometheusDiscoverySummary   `json:"summary,omitempty"`
	Evidence    []PrometheusDiscoveryEvidence `json:"evidence,omitempty"`
	Message     string                        `json:"message,omitempty"`
	Error       string                        `json:"error,omitempty"`
}

type PrometheusDiscoveredSeries struct {
	Labels map[string]string `json:"labels"`
}

type PrometheusDiscoverySummary struct {
	MetricCount   int  `json:"metric_count"`
	SeriesCount   int  `json:"series_count"`
	LabelKeyCount int  `json:"label_key_count"`
	Truncated     bool `json:"truncated,omitempty"`
}

type PrometheusDiscoveryEvidence struct {
	Metric      string              `json:"metric,omitempty"`
	LabelKeys   []string            `json:"label_keys,omitempty"`
	LabelValues map[string][]string `json:"label_values,omitempty"`
}

type prometheusLabelValuesAPIResponse struct {
	Status    string   `json:"status"`
	Data      []string `json:"data"`
	Error     string   `json:"error,omitempty"`
	ErrorType string   `json:"errorType,omitempty"`
}

type prometheusSeriesAPIResponse struct {
	Status    string              `json:"status"`
	Data      []map[string]string `json:"data"`
	Error     string              `json:"error,omitempty"`
	ErrorType string              `json:"errorType,omitempty"`
}

type prometheusDiscoveryPolicy struct {
	timeout        time.Duration
	defaultWindow  time.Duration
	maxWindow      time.Duration
	maxMetrics     int
	maxSeries      int
	maxLabelValues int
}

func NewPrometheusMetricsDiscoveryTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"list_prometheus_metrics",
		"Discover Prometheus metric names and labels before writing PromQL. Use this when the metric name, service label, job label, status label, or available series is uncertain.",
		func(ctx context.Context, input *PrometheusDiscoveryInput, opts ...tool.Option) (string, error) {
			out, err := discoverPrometheusMetrics(ctx, input)
			if err != nil {
				out = PrometheusDiscoveryOutput{
					Success:  false,
					Degraded: true,
					Keyword:  inputDiscoveryKeyword(input),
					Match:    inputDiscoveryMatch(input),
					Message:  "Prometheus metric discovery failed. Continue with exact metrics only when already known, and clearly mark missing metric discovery evidence.",
					Error:    err.Error(),
				}
			}
			data, marshalErr := json.Marshal(out)
			if marshalErr != nil {
				return fmt.Sprintf(`{"success":false,"degraded":true,"error":%q}`, marshalErr.Error()), nil
			}
			return string(data), nil
		},
	)
	if err != nil {
		g.Log().Warningf(context.Background(), "failed to create list_prometheus_metrics tool, will be unavailable: %v", err)
		return nil
	}
	return t
}

func discoverPrometheusMetrics(ctx context.Context, input *PrometheusDiscoveryInput) (PrometheusDiscoveryOutput, error) {
	ctx = ctxOrBackground(ctx)
	if input == nil {
		input = &PrometheusDiscoveryInput{}
	}
	policy := loadPrometheusDiscoveryPolicy(ctx)
	start, end, err := resolvePrometheusDiscoveryRange(input, policy)
	if err != nil {
		return PrometheusDiscoveryOutput{}, err
	}
	baseURL := prometheusBaseURL(ctx)
	if baseURL == "" {
		return PrometheusDiscoveryOutput{}, fmt.Errorf("prometheus.address is not configured")
	}
	keyword := strings.TrimSpace(input.Keyword)
	match := strings.TrimSpace(input.Match)
	if match != "" {
		return discoverPrometheusSeries(ctx, baseURL, input, policy, start, end, keyword, match)
	}
	return discoverPrometheusMetricNames(ctx, baseURL, input, policy, start, end, keyword)
}

func discoverPrometheusMetricNames(ctx context.Context, baseURL string, input *PrometheusDiscoveryInput, policy prometheusDiscoveryPolicy, start time.Time, end time.Time, keyword string) (PrometheusDiscoveryOutput, error) {
	values := url.Values{}
	values.Set("start", formatPrometheusTime(start))
	values.Set("end", formatPrometheusTime(end))
	apiURL := fmt.Sprintf("%s/api/v1/label/__name__/values?%s", baseURL, values.Encode())

	var apiResp prometheusLabelValuesAPIResponse
	if err := getPrometheusJSON(ctx, apiURL, policy.timeout, &apiResp); err != nil {
		return PrometheusDiscoveryOutput{}, err
	}
	if apiResp.Status != "success" {
		return PrometheusDiscoveryOutput{}, fmt.Errorf("prometheus metric discovery error: type=%s msg=%s", apiResp.ErrorType, apiResp.Error)
	}
	limit := resolvePrometheusDiscoveryLimit(input.Limit, policy.maxMetrics)
	metrics, truncated := filterPrometheusMetricNames(apiResp.Data, keyword, limit)
	evidence := make([]PrometheusDiscoveryEvidence, 0, len(metrics))
	for _, metric := range metrics {
		evidence = append(evidence, PrometheusDiscoveryEvidence{Metric: metric})
	}
	return PrometheusDiscoveryOutput{
		Success:  true,
		Keyword:  keyword,
		Start:    start.Format(time.RFC3339),
		End:      end.Format(time.RFC3339),
		Metrics:  metrics,
		Summary:  &PrometheusDiscoverySummary{MetricCount: len(metrics), Truncated: truncated},
		Evidence: evidence,
		Message:  fmt.Sprintf("Successfully discovered %d Prometheus metrics", len(metrics)),
	}, nil
}

func discoverPrometheusSeries(ctx context.Context, baseURL string, input *PrometheusDiscoveryInput, policy prometheusDiscoveryPolicy, start time.Time, end time.Time, keyword string, match string) (PrometheusDiscoveryOutput, error) {
	values := url.Values{}
	values.Add("match[]", match)
	values.Set("start", formatPrometheusTime(start))
	values.Set("end", formatPrometheusTime(end))
	apiURL := fmt.Sprintf("%s/api/v1/series?%s", baseURL, values.Encode())

	var apiResp prometheusSeriesAPIResponse
	if err := getPrometheusJSON(ctx, apiURL, policy.timeout, &apiResp); err != nil {
		return PrometheusDiscoveryOutput{}, err
	}
	if apiResp.Status != "success" {
		return PrometheusDiscoveryOutput{}, fmt.Errorf("prometheus series discovery error: type=%s msg=%s", apiResp.ErrorType, apiResp.Error)
	}
	limit := resolvePrometheusDiscoveryLimit(input.Limit, policy.maxSeries)
	series, metrics, labelKeys, labelValues, evidence, truncated := buildPrometheusSeriesDiscovery(apiResp.Data, keyword, limit, policy.maxMetrics, policy.maxLabelValues)
	return PrometheusDiscoveryOutput{
		Success:     true,
		Keyword:     keyword,
		Match:       match,
		Start:       start.Format(time.RFC3339),
		End:         end.Format(time.RFC3339),
		Metrics:     metrics,
		Series:      series,
		LabelKeys:   labelKeys,
		LabelValues: labelValues,
		Summary:     &PrometheusDiscoverySummary{MetricCount: len(metrics), SeriesCount: len(series), LabelKeyCount: len(labelKeys), Truncated: truncated},
		Evidence:    evidence,
		Message:     fmt.Sprintf("Successfully discovered %d Prometheus series", len(series)),
	}, nil
}

func getPrometheusJSON(ctx context.Context, apiURL string, timeout time.Duration, out any) error {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build prometheus discovery request: %w", err)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to query prometheus discovery: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, defaultPrometheusDiscoveryReadBodyLimit))
	if readErr != nil {
		return fmt.Errorf("failed to read prometheus discovery response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("prometheus returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("failed to parse prometheus discovery response: %w", err)
	}
	return nil
}

func loadPrometheusDiscoveryPolicy(ctx context.Context) prometheusDiscoveryPolicy {
	ctx = ctxOrBackground(ctx)
	policy := prometheusDiscoveryPolicy{
		timeout:        defaultPrometheusDiscoveryTimeout,
		defaultWindow:  defaultPrometheusDiscoveryWindow,
		maxWindow:      defaultPrometheusDiscoveryMaxWindow,
		maxMetrics:     defaultPrometheusDiscoveryMaxMetrics,
		maxSeries:      defaultPrometheusDiscoveryMaxSeries,
		maxLabelValues: defaultPrometheusDiscoveryMaxLabelValues,
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.discovery_timeout_ms"); err == nil && v.Int64() > 0 {
		policy.timeout = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.discovery_default_window_minutes"); err == nil && v.Int64() > 0 {
		policy.defaultWindow = time.Duration(v.Int64()) * time.Minute
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.discovery_max_window_minutes"); err == nil && v.Int64() > 0 {
		policy.maxWindow = time.Duration(v.Int64()) * time.Minute
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.discovery_max_metrics"); err == nil && v.Int() > 0 {
		policy.maxMetrics = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.discovery_max_series"); err == nil && v.Int() > 0 {
		policy.maxSeries = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.discovery_max_label_values"); err == nil && v.Int() > 0 {
		policy.maxLabelValues = v.Int()
	}
	return policy
}

func resolvePrometheusDiscoveryRange(input *PrometheusDiscoveryInput, policy prometheusDiscoveryPolicy) (time.Time, time.Time, error) {
	end := time.Now()
	var err error
	if strings.TrimSpace(input.End) != "" {
		end, err = parsePrometheusTime(input.End)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end: %w", err)
		}
	}
	start := end.Add(-policy.defaultWindow)
	if strings.TrimSpace(input.Start) != "" {
		start, err = parsePrometheusTime(input.Start)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start: %w", err)
		}
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("end must be after start")
	}
	if end.Sub(start) > policy.maxWindow {
		return time.Time{}, time.Time{}, fmt.Errorf("time range %s exceeds max window %s", end.Sub(start), policy.maxWindow)
	}
	return start, end, nil
}

func resolvePrometheusDiscoveryLimit(inputLimit int, maxLimit int) int {
	if maxLimit <= 0 {
		return inputLimit
	}
	if inputLimit <= 0 || inputLimit > maxLimit {
		return maxLimit
	}
	return inputLimit
}

func filterPrometheusMetricNames(raw []string, keyword string, limit int) ([]string, bool) {
	sort.Strings(raw)
	metrics := make([]string, 0)
	lowerKeyword := strings.ToLower(keyword)
	truncated := false
	for _, name := range raw {
		if lowerKeyword != "" && !strings.Contains(strings.ToLower(name), lowerKeyword) {
			continue
		}
		if limit > 0 && len(metrics) >= limit {
			truncated = true
			break
		}
		metrics = append(metrics, name)
	}
	return metrics, truncated
}

func buildPrometheusSeriesDiscovery(raw []map[string]string, keyword string, limit int, maxMetrics int, maxLabelValues int) ([]PrometheusDiscoveredSeries, []string, []string, map[string][]string, []PrometheusDiscoveryEvidence, bool) {
	lowerKeyword := strings.ToLower(keyword)
	series := make([]PrometheusDiscoveredSeries, 0)
	labelKeysSet := map[string]struct{}{}
	labelValuesSet := map[string]map[string]struct{}{}
	metricLabels := map[string]map[string]map[string]struct{}{}
	metricsSet := map[string]struct{}{}
	truncated := false

	for _, labels := range raw {
		if lowerKeyword != "" && !prometheusSeriesContains(labels, lowerKeyword) {
			continue
		}
		if limit > 0 && len(series) >= limit {
			truncated = true
			break
		}
		copied := copyStringMap(labels)
		series = append(series, PrometheusDiscoveredSeries{Labels: copied})
		metric := copied["__name__"]
		if metric != "" {
			metricsSet[metric] = struct{}{}
		}
		for key, value := range copied {
			labelKeysSet[key] = struct{}{}
			if _, ok := labelValuesSet[key]; !ok {
				labelValuesSet[key] = map[string]struct{}{}
			}
			labelValuesSet[key][value] = struct{}{}
			if metric != "" {
				if _, ok := metricLabels[metric]; !ok {
					metricLabels[metric] = map[string]map[string]struct{}{}
				}
				if _, ok := metricLabels[metric][key]; !ok {
					metricLabels[metric][key] = map[string]struct{}{}
				}
				metricLabels[metric][key][value] = struct{}{}
			}
		}
	}

	metrics := sortedKeys(metricsSet, maxMetrics)
	labelKeys := sortedKeys(labelKeysSet, 0)
	labelValues := sortedLimitedStringSets(labelValuesSet, maxLabelValues)
	evidence := buildPrometheusDiscoveryEvidence(metrics, metricLabels, maxLabelValues)
	return series, metrics, labelKeys, labelValues, evidence, truncated
}

func prometheusSeriesContains(labels map[string]string, lowerKeyword string) bool {
	for key, value := range labels {
		if strings.Contains(strings.ToLower(key), lowerKeyword) || strings.Contains(strings.ToLower(value), lowerKeyword) {
			return true
		}
	}
	return false
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func sortedKeys(in map[string]struct{}, limit int) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		return out[:limit]
	}
	return out
}

func sortedLimitedStringSets(in map[string]map[string]struct{}, limit int) map[string][]string {
	out := make(map[string][]string, len(in))
	for key, valuesSet := range in {
		values := sortedKeys(valuesSet, limit)
		out[key] = values
	}
	return out
}

func buildPrometheusDiscoveryEvidence(metrics []string, metricLabels map[string]map[string]map[string]struct{}, maxLabelValues int) []PrometheusDiscoveryEvidence {
	evidence := make([]PrometheusDiscoveryEvidence, 0, len(metrics))
	for _, metric := range metrics {
		rawLabelValues := metricLabels[metric]
		labelValues := sortedLimitedStringSets(rawLabelValues, maxLabelValues)
		labelKeysSet := make(map[string]struct{}, len(rawLabelValues))
		for key := range rawLabelValues {
			labelKeysSet[key] = struct{}{}
		}
		evidence = append(evidence, PrometheusDiscoveryEvidence{
			Metric:      metric,
			LabelKeys:   sortedKeys(labelKeysSet, 0),
			LabelValues: labelValues,
		})
	}
	return evidence
}

func inputDiscoveryKeyword(input *PrometheusDiscoveryInput) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(input.Keyword)
}

func inputDiscoveryMatch(input *PrometheusDiscoveryInput) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(input.Match)
}
