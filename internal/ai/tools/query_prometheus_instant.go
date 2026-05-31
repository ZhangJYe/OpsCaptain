package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	defaultPrometheusInstantTimeout       = 5 * time.Second
	defaultPrometheusInstantMaxSeries     = 20
	defaultPrometheusInstantReadBodyLimit = 10 * 1024 * 1024
)

type PrometheusInstantInput struct {
	Query string `json:"query" jsonschema:"description=PromQL instant query, for example up, sum(rate(http_requests_total[5m])), or histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le))"`
	Time  string `json:"time,omitempty" jsonschema:"description=Optional evaluation time, RFC3339 or Unix seconds. Defaults to now."`
}

type PrometheusInstantOutput struct {
	Success    bool                      `json:"success"`
	Degraded   bool                      `json:"degraded,omitempty"`
	Query      string                    `json:"query,omitempty"`
	Time       string                    `json:"time,omitempty"`
	ResultType string                    `json:"result_type,omitempty"`
	Summary    *PrometheusInstantSummary `json:"summary,omitempty"`
	Samples    []PrometheusInstantSample `json:"samples,omitempty"`
	Scalar     *PrometheusInstantSample  `json:"scalar,omitempty"`
	Evidence   []PrometheusInstantSample `json:"evidence,omitempty"`
	Message    string                    `json:"message,omitempty"`
	Error      string                    `json:"error,omitempty"`
}

type PrometheusInstantSummary struct {
	SeriesCount  int     `json:"series_count"`
	NumericCount int     `json:"numeric_count"`
	Min          float64 `json:"min,omitempty"`
	Max          float64 `json:"max,omitempty"`
	Avg          float64 `json:"avg,omitempty"`
	Truncated    bool    `json:"truncated,omitempty"`
}

type PrometheusInstantSample struct {
	Labels    map[string]string `json:"labels,omitempty"`
	Timestamp string            `json:"timestamp,omitempty"`
	Value     *float64          `json:"value,omitempty"`
	ValueText string            `json:"value_text,omitempty"`
}

type prometheusInstantAPIResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string          `json:"resultType"`
		Result     json.RawMessage `json:"result"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

type prometheusInstantVectorSerie struct {
	Metric map[string]string `json:"metric"`
	Value  []any             `json:"value"`
}

type prometheusInstantPolicy struct {
	timeout   time.Duration
	maxSeries int
}

func NewPrometheusInstantQueryTool() tool.InvokableTool {
	t, err := utils.InferOptionableTool(
		"query_prometheus_instant",
		"Query Prometheus instant data for current metric values, such as service up status, current QPS, error rate, latency percentile, CPU, or memory. Use this when you need a point-in-time metric snapshot.",
		func(ctx context.Context, input *PrometheusInstantInput, opts ...tool.Option) (string, error) {
			out, err := queryPrometheusInstant(ctx, input)
			if err != nil {
				out = PrometheusInstantOutput{
					Success:  false,
					Degraded: true,
					Query:    inputInstantQuery(input),
					Message:  "Prometheus instant query failed. Continue with available alerts, metric trends, logs, knowledge, and user context, and clearly mark missing current metric evidence.",
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
		g.Log().Warningf(context.Background(), "failed to create query_prometheus_instant tool, will be unavailable: %v", err)
		return nil
	}
	return t
}

func queryPrometheusInstant(ctx context.Context, input *PrometheusInstantInput) (PrometheusInstantOutput, error) {
	ctx = ctxOrBackground(ctx)
	if input == nil {
		input = &PrometheusInstantInput{}
	}
	policy := loadPrometheusInstantPolicy(ctx)
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return PrometheusInstantOutput{}, fmt.Errorf("query is required")
	}
	evalTime := time.Now()
	if strings.TrimSpace(input.Time) != "" {
		parsed, err := parsePrometheusTime(input.Time)
		if err != nil {
			return PrometheusInstantOutput{}, fmt.Errorf("invalid time: %w", err)
		}
		evalTime = parsed
	}
	baseURL := prometheusBaseURL(ctx)
	if baseURL == "" {
		return PrometheusInstantOutput{}, fmt.Errorf("prometheus.address is not configured")
	}

	values := url.Values{}
	values.Set("query", query)
	values.Set("time", formatPrometheusTime(evalTime))
	values.Set("limit", strconv.Itoa(policy.maxSeries))
	apiURL := fmt.Sprintf("%s/api/v1/query?%s", baseURL, values.Encode())

	reqCtx, cancel := context.WithTimeout(ctx, policy.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return PrometheusInstantOutput{}, fmt.Errorf("failed to build prometheus instant request: %w", err)
	}

	client := &http.Client{Timeout: policy.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return PrometheusInstantOutput{}, fmt.Errorf("failed to query prometheus instant: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, defaultPrometheusInstantReadBodyLimit))
	if readErr != nil {
		return PrometheusInstantOutput{}, fmt.Errorf("failed to read prometheus instant response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PrometheusInstantOutput{}, fmt.Errorf("prometheus returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var apiResp prometheusInstantAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return PrometheusInstantOutput{}, fmt.Errorf("failed to parse prometheus instant response: %w", err)
	}
	if apiResp.Status != "success" {
		return PrometheusInstantOutput{}, fmt.Errorf("prometheus instant query error: type=%s msg=%s", apiResp.ErrorType, apiResp.Error)
	}

	samples, scalar, summary, err := buildPrometheusInstantOutput(apiResp, policy)
	if err != nil {
		return PrometheusInstantOutput{}, err
	}
	return PrometheusInstantOutput{
		Success:    true,
		Query:      query,
		Time:       evalTime.Format(time.RFC3339),
		ResultType: apiResp.Data.ResultType,
		Summary:    &summary,
		Samples:    samples,
		Scalar:     scalar,
		Evidence:   prometheusInstantEvidence(samples, scalar),
		Message:    fmt.Sprintf("Successfully queried %d current metric samples", summary.SeriesCount),
	}, nil
}

func loadPrometheusInstantPolicy(ctx context.Context) prometheusInstantPolicy {
	ctx = ctxOrBackground(ctx)
	policy := prometheusInstantPolicy{
		timeout:   defaultPrometheusInstantTimeout,
		maxSeries: defaultPrometheusInstantMaxSeries,
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.instant_query_timeout_ms"); err == nil && v.Int64() > 0 {
		policy.timeout = time.Duration(v.Int64()) * time.Millisecond
	}
	if v, err := g.Cfg().Get(ctx, "prometheus.instant_max_series"); err == nil && v.Int() > 0 {
		policy.maxSeries = v.Int()
	}
	return policy
}

func buildPrometheusInstantOutput(apiResp prometheusInstantAPIResponse, policy prometheusInstantPolicy) ([]PrometheusInstantSample, *PrometheusInstantSample, PrometheusInstantSummary, error) {
	switch apiResp.Data.ResultType {
	case "vector":
		var raw []prometheusInstantVectorSerie
		if err := json.Unmarshal(apiResp.Data.Result, &raw); err != nil {
			return nil, nil, PrometheusInstantSummary{}, fmt.Errorf("failed to parse prometheus vector result: %w", err)
		}
		limit := policy.maxSeries
		if limit <= 0 || limit > len(raw) {
			limit = len(raw)
		}
		samples := make([]PrometheusInstantSample, 0, limit)
		values := make([]float64, 0, limit)
		for _, item := range raw[:limit] {
			sample, ok := parsePrometheusInstantSample(item.Metric, item.Value)
			if !ok {
				continue
			}
			if sample.Value != nil {
				values = append(values, *sample.Value)
			}
			samples = append(samples, sample)
		}
		return samples, nil, summarizePrometheusInstant(samples, values, len(raw) > limit), nil
	case "scalar", "string":
		var raw []any
		if err := json.Unmarshal(apiResp.Data.Result, &raw); err != nil {
			return nil, nil, PrometheusInstantSummary{}, fmt.Errorf("failed to parse prometheus %s result: %w", apiResp.Data.ResultType, err)
		}
		sample, ok := parsePrometheusInstantSample(map[string]string{"result_type": apiResp.Data.ResultType}, raw)
		if !ok {
			return nil, nil, PrometheusInstantSummary{}, fmt.Errorf("prometheus %s result is empty", apiResp.Data.ResultType)
		}
		values := []float64{}
		if sample.Value != nil {
			values = append(values, *sample.Value)
		}
		summary := summarizePrometheusInstant([]PrometheusInstantSample{sample}, values, false)
		return nil, &sample, summary, nil
	default:
		return nil, nil, PrometheusInstantSummary{}, fmt.Errorf("unsupported prometheus instant result type %q", apiResp.Data.ResultType)
	}
}

func parsePrometheusInstantSample(labels map[string]string, raw []any) (PrometheusInstantSample, bool) {
	if len(raw) < 2 {
		return PrometheusInstantSample{}, false
	}
	ts, ok := numberFromAny(raw[0])
	if !ok {
		return PrometheusInstantSample{}, false
	}
	sec, frac := math.Modf(ts)
	sample := PrometheusInstantSample{
		Labels:    labels,
		Timestamp: time.Unix(int64(sec), int64(frac*1e9)).UTC().Format(time.RFC3339),
	}
	if value, ok := valueFromAny(raw[1]); ok {
		sample.Value = float64Ptr(value)
		return sample, true
	}
	if value, ok := raw[1].(string); ok {
		sample.ValueText = value
		return sample, true
	}
	return sample, true
}

func summarizePrometheusInstant(samples []PrometheusInstantSample, values []float64, truncated bool) PrometheusInstantSummary {
	summary := PrometheusInstantSummary{
		SeriesCount:  len(samples),
		NumericCount: len(values),
		Truncated:    truncated,
	}
	if len(values) == 0 {
		return summary
	}
	stats := summarizeValues(values)
	summary.Min = stats.Min
	summary.Max = stats.Max
	summary.Avg = stats.Avg
	return summary
}

func prometheusInstantEvidence(samples []PrometheusInstantSample, scalar *PrometheusInstantSample) []PrometheusInstantSample {
	if scalar != nil {
		return []PrometheusInstantSample{*scalar}
	}
	return samples
}

func float64Ptr(v float64) *float64 {
	return &v
}

func inputInstantQuery(input *PrometheusInstantInput) string {
	if input == nil {
		return ""
	}
	return strings.TrimSpace(input.Query)
}
