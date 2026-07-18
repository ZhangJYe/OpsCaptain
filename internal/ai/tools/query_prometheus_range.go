package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultPrometheusRangeTimeout       = 5 * time.Second
	defaultPrometheusRangeWindow        = 30 * time.Minute
	defaultPrometheusRangeMaxWindow     = 6 * time.Hour
	defaultPrometheusRangeStep          = 60 * time.Second
	defaultPrometheusRangeMaxSeries     = 20
	defaultPrometheusRangeMaxPoints     = 1000
	defaultPrometheusRangeReadBodyLimit = 10 * 1024 * 1024
)

type PrometheusRangeInput struct {
	Query string `json:"query" jsonschema:"description=PromQL range query, for example rate(http_requests_total{service=\"checkout\"}[5m])"`
	Start string `json:"start,omitempty" jsonschema:"description=Optional range start time, RFC3339 or Unix seconds. Defaults to end minus configured default window."`
	End   string `json:"end,omitempty" jsonschema:"description=Optional range end time, RFC3339 or Unix seconds. Defaults to now."`
	Step  string `json:"step,omitempty" jsonschema:"description=Optional query step, for example 30s, 1m, or 60. Defaults to configured step."`
}

type PrometheusRangeOutput struct {
	Success  bool                      `json:"success"`
	Degraded bool                      `json:"degraded,omitempty"`
	Query    string                    `json:"query,omitempty"`
	Start    string                    `json:"start,omitempty"`
	End      string                    `json:"end,omitempty"`
	Step     string                    `json:"step,omitempty"`
	Summary  *PrometheusRangeSummary   `json:"summary,omitempty"`
	Series   []PrometheusRangeSeries   `json:"series,omitempty"`
	Evidence []PrometheusRangeEvidence `json:"evidence,omitempty"`
	Message  string                    `json:"message,omitempty"`
	Error    string                    `json:"error,omitempty"`
}

type PrometheusRangeSummary struct {
	SeriesCount int     `json:"series_count"`
	PointsCount int     `json:"points_count"`
	Min         float64 `json:"min,omitempty"`
	Max         float64 `json:"max,omitempty"`
	Avg         float64 `json:"avg,omitempty"`
	Last        float64 `json:"last,omitempty"`
	Trend       string  `json:"trend,omitempty"`
	Truncated   bool    `json:"truncated,omitempty"`
}

type PrometheusRangeSeries struct {
	Labels map[string]string `json:"labels"`
	Points []PrometheusPoint `json:"points"`
}

type PrometheusPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

type PrometheusRangeEvidence struct {
	Labels map[string]string `json:"labels"`
	First  float64           `json:"first"`
	Last   float64           `json:"last"`
	Min    float64           `json:"min"`
	Max    float64           `json:"max"`
	Avg    float64           `json:"avg"`
	Trend  string            `json:"trend"`
}

type prometheusRangeAPIResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string                    `json:"resultType"`
		Result     []prometheusRangeAPISerie `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

type prometheusRangeAPISerie struct {
	Metric map[string]string `json:"metric"`
	Values [][]any           `json:"values"`
}

type prometheusRangePolicy struct {
	timeout       time.Duration
	defaultWindow time.Duration
	maxWindow     time.Duration
	step          time.Duration
	maxSeries     int
	maxPoints     int
}

func NewPrometheusRangeQueryTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_prometheus_range",
		"Query Prometheus range data for metric trends, such as latency, error rate, throughput, CPU, or memory over time. Use this when you need time-series evidence, not just active alerts.",
		func(ctx context.Context, input *PrometheusRangeInput, opts ...tool.Option) (string, error) {
			out, err := queryPrometheusRange(ctx, input)
			if err != nil {
				out = PrometheusRangeOutput{
					Success:  false,
					Degraded: true,
					Query:    inputQuery(input),
					Message:  "Prometheus range query failed. Continue with available alerts, logs, knowledge, and user context, and clearly mark missing metric trend evidence.",
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
		g.Log().Warningf(context.Background(), "failed to create query_prometheus_range tool, will be unavailable: %v", err)
		return nil
	}
	return t
}

func queryPrometheusRange(ctx context.Context, input *PrometheusRangeInput) (PrometheusRangeOutput, error) {
	ctx = ctxOrBackground(ctx)
	if input == nil {
		input = &PrometheusRangeInput{}
	}
	policy := loadPrometheusRangePolicy(ctx)
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return PrometheusRangeOutput{}, fmt.Errorf("query is required")
	}
	start, end, step, err := resolvePrometheusRange(input, policy)
	if err != nil {
		return PrometheusRangeOutput{}, err
	}
	baseURL := prometheusBaseURL(ctx)
	if baseURL == "" {
		return PrometheusRangeOutput{}, fmt.Errorf("prometheus.address is not configured")
	}

	values := url.Values{}
	values.Set("query", query)
	values.Set("start", formatPrometheusTime(start))
	values.Set("end", formatPrometheusTime(end))
	values.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))
	values.Set("limit", strconv.Itoa(policy.maxSeries))
	apiURL := fmt.Sprintf("%s/api/v1/query_range?%s", baseURL, values.Encode())

	reqCtx, cancel := context.WithTimeout(ctx, policy.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return PrometheusRangeOutput{}, fmt.Errorf("failed to build prometheus range request: %w", err)
	}

	client := &http.Client{Timeout: policy.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return PrometheusRangeOutput{}, fmt.Errorf("failed to query prometheus range: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, defaultPrometheusRangeReadBodyLimit))
	if readErr != nil {
		return PrometheusRangeOutput{}, fmt.Errorf("failed to read prometheus range response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PrometheusRangeOutput{}, fmt.Errorf("prometheus returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var apiResp prometheusRangeAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return PrometheusRangeOutput{}, fmt.Errorf("failed to parse prometheus range response: %w", err)
	}
	if apiResp.Status != "success" {
		return PrometheusRangeOutput{}, fmt.Errorf("prometheus range query error: type=%s msg=%s", apiResp.ErrorType, apiResp.Error)
	}

	series, evidence, summary := buildPrometheusRangeOutput(apiResp.Data.Result, policy)
	return PrometheusRangeOutput{
		Success:  true,
		Query:    query,
		Start:    start.Format(time.RFC3339),
		End:      end.Format(time.RFC3339),
		Step:     step.String(),
		Summary:  &summary,
		Series:   series,
		Evidence: evidence,
		Message:  fmt.Sprintf("Successfully queried %d series and %d points", summary.SeriesCount, summary.PointsCount),
	}, nil
}

func prometheusBaseURL(ctx context.Context) string {
	ctx = ctxOrBackground(ctx)
	if value := normalizeOptionalURL(os.Getenv("PROMETHEUS_ADDRESS")); value != "" {
		return value
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.address"); err == nil {
		if value := normalizeOptionalURL(v.String()); value != "" {
			return value
		}
	}
	return ""
}

func loadPrometheusRangePolicy(ctx context.Context) prometheusRangePolicy {
	ctx = ctxOrBackground(ctx)
	policy := prometheusRangePolicy{
		timeout:       defaultPrometheusRangeTimeout,
		defaultWindow: defaultPrometheusRangeWindow,
		maxWindow:     defaultPrometheusRangeMaxWindow,
		step:          defaultPrometheusRangeStep,
		maxSeries:     defaultPrometheusRangeMaxSeries,
		maxPoints:     defaultPrometheusRangeMaxPoints,
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.range_query_timeout_ms"); err == nil && v.Int64() > 0 {
		policy.timeout = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.range_default_window_minutes"); err == nil && v.Int64() > 0 {
		policy.defaultWindow = time.Duration(v.Int64()) * time.Minute
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.range_max_window_minutes"); err == nil && v.Int64() > 0 {
		policy.maxWindow = time.Duration(v.Int64()) * time.Minute
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.range_default_step_seconds"); err == nil && v.Int64() > 0 {
		policy.step = time.Duration(v.Int64()) * time.Second
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.range_max_series"); err == nil && v.Int() > 0 {
		policy.maxSeries = v.Int()
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.range_max_points"); err == nil && v.Int() > 0 {
		policy.maxPoints = v.Int()
	}
	return policy
}

func resolvePrometheusRange(input *PrometheusRangeInput, policy prometheusRangePolicy) (time.Time, time.Time, time.Duration, error) {
	end := time.Now()
	var err error
	if strings.TrimSpace(input.End) != "" {
		end, err = parsePrometheusTime(input.End)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid end: %w", err)
		}
	}

	start := end.Add(-policy.defaultWindow)
	if strings.TrimSpace(input.Start) != "" {
		start, err = parsePrometheusTime(input.Start)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid start: %w", err)
		}
	}

	step := policy.step
	if strings.TrimSpace(input.Step) != "" {
		step, err = parsePrometheusStep(input.Step)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid step: %w", err)
		}
	}
	if !end.After(start) {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("end must be after start")
	}
	if end.Sub(start) > policy.maxWindow {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("time range %s exceeds max window %s", end.Sub(start), policy.maxWindow)
	}
	if step <= 0 {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("step must be positive")
	}
	estimatedPoints := int(math.Ceil(end.Sub(start).Seconds()/step.Seconds())) + 1
	if estimatedPoints > policy.maxPoints {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("estimated points per series %d exceeds max points %d; increase step or narrow time range", estimatedPoints, policy.maxPoints)
	}
	return start, end, step, nil
}

func parsePrometheusTime(raw string) (time.Time, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, fmt.Errorf("time is empty")
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t, nil
	}
	seconds, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("expected RFC3339 or Unix seconds")
	}
	sec, frac := math.Modf(seconds)
	return time.Unix(int64(sec), int64(frac*1e9)).UTC(), nil
}

func parsePrometheusStep(raw string) (time.Duration, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, fmt.Errorf("step is empty")
	}
	if d, err := time.ParseDuration(trimmed); err == nil {
		return d, nil
	}
	seconds, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, fmt.Errorf("expected duration or seconds")
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func formatPrometheusTime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', -1, 64)
}

func buildPrometheusRangeOutput(raw []prometheusRangeAPISerie, policy prometheusRangePolicy) ([]PrometheusRangeSeries, []PrometheusRangeEvidence, PrometheusRangeSummary) {
	limitSeries := policy.maxSeries
	if limitSeries <= 0 || limitSeries > len(raw) {
		limitSeries = len(raw)
	}
	remainingPoints := policy.maxPoints
	series := make([]PrometheusRangeSeries, 0, limitSeries)
	evidence := make([]PrometheusRangeEvidence, 0, limitSeries)
	summary := PrometheusRangeSummary{SeriesCount: limitSeries}
	var all []float64
	var firstValues []float64
	var lastValues []float64
	truncated := len(raw) > limitSeries

	for _, item := range raw[:limitSeries] {
		if remainingPoints <= 0 {
			truncated = true
			break
		}
		points, values := parsePrometheusPoints(item.Values, remainingPoints)
		if len(item.Values) > len(points) {
			truncated = true
		}
		remainingPoints -= len(points)
		if len(points) == 0 {
			series = append(series, PrometheusRangeSeries{Labels: item.Metric})
			continue
		}
		stats := summarizeValues(values)
		evidence = append(evidence, PrometheusRangeEvidence{
			Labels: item.Metric,
			First:  values[0],
			Last:   values[len(values)-1],
			Min:    stats.Min,
			Max:    stats.Max,
			Avg:    stats.Avg,
			Trend:  classifyPrometheusTrend(values[0], values[len(values)-1]),
		})
		series = append(series, PrometheusRangeSeries{Labels: item.Metric, Points: points})
		all = append(all, values...)
		firstValues = append(firstValues, values[0])
		lastValues = append(lastValues, values[len(values)-1])
	}

	if len(all) > 0 {
		stats := summarizeValues(all)
		summary.SeriesCount = len(series)
		summary.PointsCount = len(all)
		summary.Min = stats.Min
		summary.Max = stats.Max
		summary.Avg = stats.Avg
		summary.Last = avgFloat64(lastValues)
		summary.Trend = classifyPrometheusTrend(avgFloat64(firstValues), avgFloat64(lastValues))
	}
	if len(all) == 0 {
		summary.SeriesCount = len(series)
	}
	summary.Truncated = truncated
	return series, evidence, summary
}

func parsePrometheusPoints(raw [][]any, limit int) ([]PrometheusPoint, []float64) {
	points := make([]PrometheusPoint, 0)
	values := make([]float64, 0)
	for _, pair := range raw {
		if len(points) >= limit {
			break
		}
		if len(pair) < 2 {
			continue
		}
		ts, ok := numberFromAny(pair[0])
		if !ok {
			continue
		}
		value, ok := valueFromAny(pair[1])
		if !ok {
			continue
		}
		sec, frac := math.Modf(ts)
		points = append(points, PrometheusPoint{
			Timestamp: time.Unix(int64(sec), int64(frac*1e9)).UTC().Format(time.RFC3339),
			Value:     value,
		})
		values = append(values, value)
	}
	return points, values
}

func numberFromAny(v any) (float64, bool) {
	switch typed := v.(type) {
	case float64:
		return typed, true
	case json.Number:
		out, err := typed.Float64()
		return out, err == nil
	case string:
		out, err := strconv.ParseFloat(typed, 64)
		return out, err == nil
	default:
		return 0, false
	}
}

func valueFromAny(v any) (float64, bool) {
	switch typed := v.(type) {
	case string:
		out, err := strconv.ParseFloat(typed, 64)
		if err != nil || math.IsNaN(out) || math.IsInf(out, 0) {
			return 0, false
		}
		return out, true
	default:
		out, ok := numberFromAny(v)
		if !ok || math.IsNaN(out) || math.IsInf(out, 0) {
			return 0, false
		}
		return out, true
	}
}

type prometheusValueStats struct {
	Min float64
	Max float64
	Avg float64
}

func summarizeValues(values []float64) prometheusValueStats {
	if len(values) == 0 {
		return prometheusValueStats{}
	}
	minV := values[0]
	maxV := values[0]
	sum := 0.0
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		sum += v
	}
	return prometheusValueStats{Min: minV, Max: maxV, Avg: sum / float64(len(values))}
}

func classifyPrometheusTrend(first, last float64) string {
	diff := last - first
	threshold := math.Max(math.Abs(first)*0.05, 1e-9)
	if math.Abs(diff) <= threshold {
		return "flat"
	}
	if diff > 0 {
		return "up"
	}
	return "down"
}

func avgFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func inputQuery(input *PrometheusRangeInput) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(input.Query)
}
