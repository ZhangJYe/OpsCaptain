package alertcorrelation

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Engine performs alert correlation analysis.
type Engine struct {
	topology           TopologyProvider
	timeWindowMinutes  int
	propagationThreshold float64
}

// NewEngine creates a correlation engine.
func NewEngine(topo TopologyProvider, timeWindowMinutes int, propagationThreshold float64) *Engine {
	if timeWindowMinutes <= 0 {
		timeWindowMinutes = 5
	}
	if propagationThreshold <= 0 || propagationThreshold > 1 {
		propagationThreshold = 0.7
	}
	return &Engine{
		topology:             topo,
		timeWindowMinutes:    timeWindowMinutes,
		propagationThreshold: propagationThreshold,
	}
}

// Analyze performs full correlation analysis on a set of alerts.
func (e *Engine) Analyze(alerts []SimplifiedAlert) CorrelationResult {
	if len(alerts) == 0 {
		return CorrelationResult{
			Success:     true,
			Summary:     "当前没有活跃告警。",
			TotalAlerts: 0,
		}
	}

	// Sort by active time
	sorted := make([]SimplifiedAlert, len(alerts))
	copy(sorted, alerts)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ActiveAt.Before(sorted[j].ActiveAt)
	})

	// Group by time window
	groups := e.groupByTimeWindow(sorted)

	// Extract services from alerts
	alertServices := e.extractServices(sorted)

	// Detect propagation chains
	propagation := e.detectPropagation(sorted, alertServices)

	// Identify root cause candidates
	rootCandidates := e.identifyRootCauses(sorted, propagation, alertServices)

	// Build summary
	summary := e.buildSummary(groups, propagation, rootCandidates, sorted)

	timeRange := fmt.Sprintf("%s ~ %s",
		sorted[0].ActiveAt.Format("15:04:05"),
		sorted[len(sorted)-1].ActiveAt.Format("15:04:05"))

	return CorrelationResult{
		Success:        true,
		AlertGroups:    groups,
		Propagation:    propagation,
		RootCandidates: rootCandidates,
		Summary:        summary,
		TotalAlerts:    len(alerts),
		TimeRange:      timeRange,
	}
}

// groupByTimeWindow groups alerts into time windows.
func (e *Engine) groupByTimeWindow(alerts []SimplifiedAlert) []AlertGroup {
	if len(alerts) == 0 {
		return nil
	}

	window := time.Duration(e.timeWindowMinutes) * time.Minute
	var groups []AlertGroup

	currentGroup := AlertGroup{
		WindowStart: alerts[0].ActiveAt,
		WindowEnd:   alerts[0].ActiveAt.Add(window),
		Alerts:      []SimplifiedAlert{alerts[0]},
	}

	for i := 1; i < len(alerts); i++ {
		alert := alerts[i]
		if alert.ActiveAt.Before(currentGroup.WindowEnd.Add(window)) {
			// Same or adjacent window
			currentGroup.Alerts = append(currentGroup.Alerts, alert)
			if alert.ActiveAt.After(currentGroup.WindowEnd) {
				currentGroup.WindowEnd = alert.ActiveAt
			}
		} else {
			// New window
			currentGroup.Services = e.extractServicesFromGroup(currentGroup.Alerts)
			groups = append(groups, currentGroup)
			currentGroup = AlertGroup{
				WindowStart: alert.ActiveAt,
				WindowEnd:   alert.ActiveAt.Add(window),
				Alerts:      []SimplifiedAlert{alert},
			}
		}
	}
	currentGroup.Services = e.extractServicesFromGroup(currentGroup.Alerts)
	groups = append(groups, currentGroup)

	return groups
}

func (e *Engine) extractServicesFromGroup(alerts []SimplifiedAlert) []string {
	seen := make(map[string]bool)
	var services []string
	for _, a := range alerts {
		svc := e.inferService(a)
		if svc != "" && !seen[svc] {
			seen[svc] = true
			services = append(services, svc)
		}
	}
	return services
}

// extractServices gets all unique service names from alerts.
func (e *Engine) extractServices(alerts []SimplifiedAlert) []string {
	seen := make(map[string]bool)
	var services []string
	for _, a := range alerts {
		svc := e.inferService(a)
		if svc != "" && !seen[svc] {
			seen[svc] = true
			services = append(services, svc)
		}
	}
	return services
}

// inferService tries to determine which service an alert belongs to.
func (e *Engine) inferService(alert SimplifiedAlert) string {
	// Try common label names
	for _, key := range []string{"service", "instance", "job", "exported_service"} {
		if v, ok := alert.Labels[key]; ok && v != "" {
			// Strip port suffix if present
			if idx := strings.LastIndex(v, ":"); idx > 0 {
				v = v[:idx]
			}
			return v
		}
	}

	// Try to extract from alert name (e.g., "HighErrorRate_paymentservice")
	name := alert.AlertName
	if e.topology != nil {
		for _, svc := range e.topology.GetAllServices() {
			if strings.Contains(strings.ToLower(name), strings.ToLower(svc)) {
				return svc
			}
			if strings.Contains(strings.ToLower(alert.Description), strings.ToLower(svc)) {
				return svc
			}
		}
	}

	return ""
}

// detectPropagation checks if alerts follow the service dependency graph.
func (e *Engine) detectPropagation(alerts []SimplifiedAlert, services []string) []PropagationChain {
	if e.topology == nil || len(services) < 2 {
		return nil
	}

	var chains []PropagationChain
	seen := make(map[string]bool)

	// Build service -> earliest alert time map
	serviceFirstAlert := make(map[string]time.Time)
	for _, a := range alerts {
		svc := e.inferService(a)
		if svc == "" {
			continue
		}
		if t, ok := serviceFirstAlert[svc]; !ok || a.ActiveAt.Before(t) {
			serviceFirstAlert[svc] = a.ActiveAt
		}
	}

	// Check each pair for propagation
	for _, upstream := range services {
		downstreams := e.topology.GetDownstream(upstream)
		for _, downstream := range downstreams {
			upTime, upOk := serviceFirstAlert[upstream]
			downTime, downOk := serviceFirstAlert[downstream]

			if !upOk || !downOk {
				continue
			}

			// Upstream should alert before downstream
			if upTime.Before(downTime) || upTime.Equal(downTime) {
				key := upstream + "->" + downstream
				if seen[key] {
					continue
				}
				seen[key] = true

				delay := downTime.Sub(upTime)
				confidence := e.calculatePropagationConfidence(delay, upstream, downstream)

				if confidence >= e.propagationThreshold {
					chains = append(chains, PropagationChain{
						Path:       []string{upstream, downstream},
						Direction:  "upstream_to_downstream",
						Confidence: confidence,
						Evidence:   fmt.Sprintf("%s 先告警，%s %v 后告警", upstream, downstream, delay.Round(time.Second)),
					})
				}
			}
		}
	}

	// Also check downstream -> upstream (reverse propagation)
	for _, downstream := range services {
		upstreams := e.topology.GetUpstream(downstream)
		for _, upstream := range upstreams {
			downTime, downOk := serviceFirstAlert[downstream]
			upTime, upOk := serviceFirstAlert[upstream]

			if !downOk || !upOk {
				continue
			}

			// Downstream alerts before upstream — unusual, might indicate independent issue
			if downTime.Before(upTime) {
				key := downstream + "->" + upstream + "_reverse"
				if seen[key] {
					continue
				}
				seen[key] = true

				delay := upTime.Sub(downTime)
				chains = append(chains, PropagationChain{
					Path:       []string{downstream, upstream},
					Direction:  "downstream_to_upstream_unusual",
					Confidence: 0.5,
					Evidence:   fmt.Sprintf("%s 先于上游 %s 告警（%v），可能是独立故障", downstream, upstream, delay.Round(time.Second)),
				})
			}
		}
	}

	return chains
}

func (e *Engine) calculatePropagationConfidence(delay time.Duration, upstream, downstream string) float64 {
	// Higher confidence for shorter delays (likely cascading)
	// Lower confidence for long delays (might be coincidental)
	switch {
	case delay < 1*time.Minute:
		return 0.95
	case delay < 5*time.Minute:
		return 0.85
	case delay < 15*time.Minute:
		return 0.75
	case delay < 30*time.Minute:
		return 0.65
	default:
		return 0.5
	}
}

// identifyRootCauses finds the most likely root cause services.
func (e *Engine) identifyRootCauses(alerts []SimplifiedAlert, chains []PropagationChain, services []string) []RootCauseCandidate {
	if len(alerts) == 0 {
		return nil
	}

	// Find services that are upstream in propagation chains (they caused downstream alerts)
	upstreamCauses := make(map[string]bool)
	for _, chain := range chains {
		if chain.Direction == "upstream_to_downstream" && len(chain.Path) > 0 {
			upstreamCauses[chain.Path[0]] = true
		}
	}

	// Find the earliest alert for each service
	serviceAlerts := make(map[string]SimplifiedAlert)
	for _, a := range alerts {
		svc := e.inferService(a)
		if svc == "" {
			continue
		}
		if existing, ok := serviceAlerts[svc]; !ok || a.ActiveAt.Before(existing.ActiveAt) {
			serviceAlerts[svc] = a
		}
	}

	// Score each service
	type candidate struct {
		service string
		alert   SimplifiedAlert
		score   float64
		reason  string
	}

	var candidates []candidate
	for svc, alert := range serviceAlerts {
		score := 0.0
		reasons := []string{}

		// Earliest alert gets base score
		if alert.ActiveAt.Equal(alerts[0].ActiveAt) {
			score += 0.3
			reasons = append(reasons, "最早告警")
		}

		// Upstream in propagation chain gets high score
		if upstreamCauses[svc] {
			score += 0.5
			reasons = append(reasons, "传播链上游")
		}

		// Service with no upstream alerts is likely root
		upstreams := e.topology.GetUpstream(svc)
		hasUpstreamAlert := false
		for _, up := range upstreams {
			if _, ok := serviceAlerts[up]; ok {
				hasUpstreamAlert = true
				break
			}
		}
		if !hasUpstreamAlert && len(upstreams) > 0 {
			score += 0.2
			reasons = append(reasons, "上游无告警")
		}

		if score > 0 {
			candidates = append(candidates, candidate{
				service: svc,
				alert:   alert,
				score:   score,
				reason:  strings.Join(reasons, "；"),
			})
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Return top 3
	var result []RootCauseCandidate
	limit := 3
	if len(candidates) < limit {
		limit = len(candidates)
	}
	for i := 0; i < limit; i++ {
		c := candidates[i]
		result = append(result, RootCauseCandidate{
			Service:    c.service,
			AlertName:  c.alert.AlertName,
			ActiveAt:   c.alert.ActiveAt.Format(time.RFC3339),
			Reason:     c.reason,
			Confidence: c.score,
		})
	}

	return result
}

// buildSummary generates a human-readable summary.
func (e *Engine) buildSummary(groups []AlertGroup, chains []PropagationChain, roots []RootCauseCandidate, alerts []SimplifiedAlert) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("共 %d 个告警", len(alerts)))

	if len(groups) > 1 {
		sb.WriteString(fmt.Sprintf("，分布在 %d 个时间窗口", len(groups)))
	}
	sb.WriteString("。")

	if len(chains) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n检测到 %d 条传播链：", len(chains)))
		for i, chain := range chains {
			sb.WriteString(fmt.Sprintf("\n  %d. %s → %s（置信度 %.0f%%，%s）",
				i+1, chain.Path[0], chain.Path[len(chain.Path)-1],
				chain.Confidence*100, chain.Evidence))
		}
	}

	if len(roots) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n根因候选（按可能性排序）："))
		for i, root := range roots {
			sb.WriteString(fmt.Sprintf("\n  %d. %s — %s（置信度 %.0f%%，%s）",
				i+1, root.Service, root.AlertName, root.Confidence*100, root.Reason))
		}
	} else {
		sb.WriteString("\n\n未能确定根因候选，告警可能是独立事件。")
	}

	return sb.String()
}
