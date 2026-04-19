package metrics

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var registerMetricsOnce sync.Once

var (
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_http_requests_total",
			Help: "Total number of HTTP requests handled by the service.",
		},
		[]string{"method", "path", "status"},
	)
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	llmCallsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_llm_calls_total",
			Help: "Total number of LLM calls.",
		},
		[]string{"agent", "model", "status"},
	)
	llmCallDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_llm_call_duration_seconds",
			Help:    "LLM call duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"agent", "model"},
	)
	llmTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_llm_tokens_total",
			Help: "Total number of LLM tokens consumed.",
		},
		[]string{"agent", "model", "type"},
	)
	agentDispatchTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_agent_dispatch_total",
			Help: "Total number of agent dispatches.",
		},
		[]string{"agent", "status"},
	)
	agentDispatchDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_agent_dispatch_duration_seconds",
			Help:    "Agent dispatch duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"agent"},
	)
	circuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "opscaptionai_circuit_breaker_state",
			Help: "Circuit breaker state encoded as closed=0, open=1, half_open=2.",
		},
		[]string{"name"},
	)
	cacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_cache_hits_total",
			Help: "Total number of cache hits.",
		},
		[]string{"type"},
	)
	cacheMissesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_cache_misses_total",
			Help: "Total number of cache misses.",
		},
		[]string{"type"},
	)
	sessionTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_session_tokens_total",
			Help: "Per-session token consumption for auditing and alerting.",
		},
		[]string{"user_id"},
	)
)

func Handler() http.Handler {
	ensureRegistered()
	return promhttp.Handler()
}

func ObserveHTTPRequest(method, path string, status int, duration time.Duration) {
	ensureRegistered()
	method = fallbackLabel(method, "UNKNOWN")
	path = fallbackLabel(path, "unknown")
	statusLabel := strconv.Itoa(status)
	httpRequestsTotal.WithLabelValues(method, path, statusLabel).Inc()
	httpRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

func ObserveLLMCall(agent, model, status string, duration time.Duration) {
	ensureRegistered()
	agent = fallbackLabel(agent, "unknown")
	model = fallbackLabel(model, "unknown")
	status = fallbackLabel(status, "unknown")
	llmCallsTotal.WithLabelValues(agent, model, status).Inc()
	llmCallDuration.WithLabelValues(agent, model).Observe(duration.Seconds())
}

func AddLLMTokens(agent, model, tokenType string, count int) {
	if count <= 0 {
		return
	}
	ensureRegistered()
	agent = fallbackLabel(agent, "unknown")
	model = fallbackLabel(model, "unknown")
	tokenType = fallbackLabel(tokenType, "unknown")
	llmTokensTotal.WithLabelValues(agent, model, tokenType).Add(float64(count))
}

func ObserveAgentDispatch(agent, status string, duration time.Duration) {
	ensureRegistered()
	agent = fallbackLabel(agent, "unknown")
	status = fallbackLabel(status, "unknown")
	agentDispatchTotal.WithLabelValues(agent, status).Inc()
	agentDispatchDuration.WithLabelValues(agent).Observe(duration.Seconds())
}

func SetCircuitBreakerState(name string, state float64) {
	ensureRegistered()
	circuitBreakerState.WithLabelValues(fallbackLabel(name, "unknown")).Set(state)
}

func IncCacheHit(cacheType string) {
	ensureRegistered()
	cacheHitsTotal.WithLabelValues(fallbackLabel(cacheType, "unknown")).Inc()
}

func IncCacheMiss(cacheType string) {
	ensureRegistered()
	cacheMissesTotal.WithLabelValues(fallbackLabel(cacheType, "unknown")).Inc()
}

func AddSessionTokens(_ string, userID string, count int) {
	if count <= 0 {
		return
	}
	ensureRegistered()
	sessionTokensTotal.WithLabelValues(
		fallbackLabel(userID, "anonymous"),
	).Add(float64(count))
}

func ensureRegistered() {
	registerMetricsOnce.Do(func() {
		prometheus.MustRegister(
			httpRequestsTotal,
			httpRequestDuration,
			llmCallsTotal,
			llmCallDuration,
			llmTokensTotal,
			agentDispatchTotal,
			agentDispatchDuration,
			circuitBreakerState,
			cacheHitsTotal,
			cacheMissesTotal,
			sessionTokensTotal,
		)
	})
}

func fallbackLabel(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
