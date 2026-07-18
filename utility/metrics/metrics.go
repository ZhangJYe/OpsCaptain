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
	memoryExtractionTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_memory_extraction_events_total",
			Help: "Total number of memory extraction events by mode and status.",
		},
		[]string{"mode", "status"},
	)
	chatTaskEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "opscaptionai_chat_task_events_total",
			Help: "Total number of asynchronous chat task events by status.",
		},
		[]string{"status"},
	)
	gosRunDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_gos_run_duration_seconds",
			Help:    "GoS run duration in seconds by result status.",
			Buckets: gosLatencyBuckets,
		},
		[]string{"status"},
	)
	gosPhaseDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_gos_phase_duration_seconds",
			Help:    "GoS phase duration in seconds.",
			Buckets: gosLatencyBuckets,
		},
		[]string{"phase"},
	)
	gosCallsPerRun = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_gos_calls_per_run",
			Help:    "LLM, tool, and RAG calls per GoS run.",
			Buckets: prometheus.LinearBuckets(0, 4, 17),
		},
		[]string{"type"},
	)
	gosGraphSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_gos_graph_size",
			Help:    "GoS graph and retained history size per run.",
			Buckets: prometheus.ExponentialBuckets(1, 2, 25),
		},
		[]string{"type"},
	)
	gosTransitionsPerRun = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "opscaptionai_gos_transitions_per_run",
			Help:    "GoS frontier, backtrack, and evidence changes per run.",
			Buckets: prometheus.LinearBuckets(0, 1, 16),
		},
		[]string{"type"},
	)
)

var gosLatencyBuckets = []float64{
	0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
	20, 30, 45, 60, 90, 120, 180, 300,
}

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

func ObserveMemoryExtraction(mode, status string) {
	ensureRegistered()
	mode = fallbackLabel(mode, "unknown")
	status = fallbackLabel(status, "unknown")
	memoryExtractionTotal.WithLabelValues(mode, status).Inc()
}

func ObserveChatTask(status string) {
	ensureRegistered()
	chatTaskEventsTotal.WithLabelValues(fallbackLabel(status, "unknown")).Inc()
}

func ObserveGoSRun(
	status string,
	duration time.Duration,
	phaseLatencyMs map[string]int64,
	calls map[string]int,
	graphSize map[string]int,
	transitions map[string]int,
) {
	ensureRegistered()
	gosRunDuration.WithLabelValues(fallbackLabel(status, "unknown")).Observe(duration.Seconds())
	for phase, latencyMs := range phaseLatencyMs {
		gosPhaseDuration.WithLabelValues(fallbackLabel(phase, "unknown")).Observe(float64(latencyMs) / 1000)
	}
	for callType, count := range calls {
		gosCallsPerRun.WithLabelValues(fallbackLabel(callType, "unknown")).Observe(float64(count))
	}
	for sizeType, size := range graphSize {
		gosGraphSize.WithLabelValues(fallbackLabel(sizeType, "unknown")).Observe(float64(size))
	}
	for transitionType, count := range transitions {
		gosTransitionsPerRun.WithLabelValues(fallbackLabel(transitionType, "unknown")).Observe(float64(count))
	}
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
			memoryExtractionTotal,
			chatTaskEventsTotal,
			gosRunDuration,
			gosPhaseDuration,
			gosCallsPerRun,
			gosGraphSize,
			gosTransitionsPerRun,
		)
	})
}

func fallbackLabel(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
