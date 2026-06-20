package alertcorrelation

import "time"

// SimplifiedAlert is a minimal alert representation for correlation.
type SimplifiedAlert struct {
	AlertName   string            `json:"alert_name"`
	Description string            `json:"description"`
	State       string            `json:"state"`
	ActiveAt    time.Time         `json:"active_at"`
	Duration    string            `json:"duration"`
	Labels      map[string]string `json:"labels"`
}

// AlertGroup groups alerts that fired within the same time window.
type AlertGroup struct {
	WindowStart time.Time         `json:"window_start"`
	WindowEnd   time.Time         `json:"window_end"`
	Alerts      []SimplifiedAlert `json:"alerts"`
	Services    []string          `json:"services"`
}

// PropagationChain represents a suspected fault propagation path.
type PropagationChain struct {
	Path       []string `json:"path"`
	Direction  string   `json:"direction"`
	Confidence float64  `json:"confidence"`
	Evidence   string   `json:"evidence"`
}

// RootCauseCandidate is a suspected root cause.
type RootCauseCandidate struct {
	Service    string  `json:"service"`
	AlertName  string  `json:"alert_name"`
	ActiveAt   string  `json:"active_at"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// CorrelationResult is the output of alert correlation analysis.
type CorrelationResult struct {
	Success        bool                 `json:"success"`
	AlertGroups    []AlertGroup         `json:"alert_groups"`
	Propagation    []PropagationChain   `json:"propagation"`
	RootCandidates []RootCauseCandidate `json:"root_candidates"`
	Summary        string               `json:"summary"`
	TotalAlerts    int                  `json:"total_alerts"`
	TimeRange      string               `json:"time_range"`
}

// TopologyProvider supplies service dependency data for correlation.
type TopologyProvider interface {
	// GetUpstream returns services that the given service depends on (upstream).
	GetUpstream(service string) []string
	// GetDownstream returns services that depend on the given service (downstream).
	GetDownstream(service string) []string
	// GetAllServices returns all known service names.
	GetAllServices() []string
}
