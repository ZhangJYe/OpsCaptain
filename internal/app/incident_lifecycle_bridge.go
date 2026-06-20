package app

import (
	"SuperBizAgent/internal/ai/incidentlifecycle"
	"time"
)

// GenerateIncidentPostmortem creates a postmortem from an incident session.
func GenerateIncidentPostmortem(incident *IncidentSession) *incidentlifecycle.Postmortem {
	if incident == nil {
		return nil
	}

	// Build timeline from events
	var events []incidentlifecycle.RawEvent
	for _, e := range incident.Events {
		events = append(events, incidentlifecycle.RawEvent{
			Timestamp: e.CreatedAt,
			Type:      e.Type,
			Message:   e.Message,
			Agent:     e.Agent,
		})
	}
	timeline := incidentlifecycle.AssembleTimeline(events)

	// Infer severity from turn count and status
	severity := incidentlifecycle.SeverityP3
	if len(incident.Turns) >= 5 {
		severity = incidentlifecycle.SeverityP1
	} else if len(incident.Turns) >= 3 {
		severity = incidentlifecycle.SeverityP2
	}

	lc := incidentlifecycle.IncidentLifecycle{
		Severity:  severity,
		Status:    incidentlifecycle.LifecycleResolved,
		DetectedAt: incident.CreatedAt,
		ResolvedAt: incident.UpdatedAt,
		MTTR:      incident.UpdatedAt - incident.CreatedAt,
	}

	// Extract affected services from event messages
	var affectedServices []string
	seen := make(map[string]bool)
	for _, e := range incident.Events {
		if e.Type == "service_affected" {
			svc := ""
			if v, ok := e.Payload["service"]; ok {
				if s, ok := v.(string); ok {
					svc = s
				}
			}
			if svc != "" && !seen[svc] {
				seen[svc] = true
				affectedServices = append(affectedServices, svc)
			}
		}
	}

	impact := incident.LatestSummary
	if impact == "" {
		impact = "事件已恢复"
	}

	return incidentlifecycle.GeneratePostmortem(
		incident.Title,
		lc,
		timeline,
		affectedServices,
		impact,
	)
}

// FormatIncidentPostmortem formats a postmortem as Markdown.
func FormatIncidentPostmortem(pm *incidentlifecycle.Postmortem) string {
	return incidentlifecycle.FormatPostmortemMarkdown(pm)
}

// NowMs returns current time in milliseconds.
func NowMs() int64 {
	return time.Now().UnixMilli()
}
