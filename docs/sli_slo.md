# OpsCaptionAI SLI/SLO

## Scope

This document defines the baseline service level indicators and objectives for the production OpsCaptionAI deployment.

## SLI and SLO Targets

| SLI | SLO | Measurement |
| --- | --- | --- |
| Request availability | 99.5% | `sum(rate(opscaptionai_http_requests_total{status=~"2.."}[5m])) / sum(rate(opscaptionai_http_requests_total[5m]))` |
| Chat P95 latency | < 5s | `histogram_quantile(0.95, sum(rate(opscaptionai_http_request_duration_seconds_bucket{path="/api/chat"}[5m])) by (le))` |
| AI Ops P95 latency | < 30s | `histogram_quantile(0.95, sum(rate(opscaptionai_http_request_duration_seconds_bucket{path="/api/ai_ops"}[5m])) by (le))` |
| LLM success rate | > 95% | `sum(rate(opscaptionai_llm_calls_total{status="success"}[15m])) / sum(rate(opscaptionai_llm_calls_total[15m]))` |
| Degraded response rate | < 10% | `sum(rate(opscaptionai_agent_dispatch_total{status="degraded"}[15m])) / sum(rate(opscaptionai_agent_dispatch_total[15m]))` |

## Alerting Guidance

- Page on sustained availability violations over 10 minutes.
- Page on AI Ops P95 latency violations over 15 minutes.
- Warn on degraded rate above 5%; page above 10%.
- Warn on LLM success rate below 97%; page below 95%.

## Review Cadence

- Review SLO compliance weekly during on-call handoff.
- Recalibrate objectives after major routing, model, or traffic changes.
