package actionexecutor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ActionCategory groups related actions.
type ActionCategory string

const (
	CategoryRestart ActionCategory = "restart"
	CategoryScale   ActionCategory = "scale"
	CategoryQuery   ActionCategory = "query"
	CategoryRollback ActionCategory = "rollback"
)

// RiskLevel indicates how dangerous an action is.
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// ActionParam defines a parameter for an action.
type ActionParam struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// ActionDefinition defines a runnable action.
type ActionDefinition struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    ActionCategory    `json:"category"`
	RiskLevel   RiskLevel         `json:"risk_level"`
	Parameters  []ActionParam     `json:"parameters"`
	Executor    string            `json:"executor"`
	Config      map[string]string `json:"config"`
}

// ActionResult is the outcome of an action execution.
type ActionResult struct {
	Success    bool              `json:"success"`
	ActionID   string            `json:"action_id"`
	ActionName string            `json:"action_name"`
	Output     string            `json:"output"`
	Error      string            `json:"error,omitempty"`
	ExecutedAt int64             `json:"executed_at"`
	Duration   int64             `json:"duration_ms"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Executor executes actions using a specific protocol.
type Executor interface {
	Execute(ctx context.Context, action *ActionDefinition, params map[string]string) (*ActionResult, error)
}

// Registry holds all registered actions.
type Registry struct {
	actions   map[string]*ActionDefinition
	executors map[string]Executor
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	r := &Registry{
		actions:   make(map[string]*ActionDefinition),
		executors: make(map[string]Executor),
	}
	return r
}

// Register adds an action definition.
func (r *Registry) Register(action *ActionDefinition) {
	r.actions[action.ID] = action
}

// RegisterExecutor registers an executor for a protocol.
func (r *Registry) RegisterExecutor(name string, exec Executor) {
	r.executors[name] = exec
}

// Get retrieves an action by ID.
func (r *Registry) Get(id string) (*ActionDefinition, bool) {
	action, ok := r.actions[id]
	return action, ok
}

// ListByCategory returns all actions in a category.
func (r *Registry) ListByCategory(category ActionCategory) []*ActionDefinition {
	var result []*ActionDefinition
	for _, a := range r.actions {
		if a.Category == category {
			result = append(result, a)
		}
	}
	return result
}

// ListAll returns all registered actions.
func (r *Registry) ListAll() []*ActionDefinition {
	var result []*ActionDefinition
	for _, a := range r.actions {
		result = append(result, a)
	}
	return result
}

// Execute runs an action by ID with the given parameters.
func (r *Registry) Execute(ctx context.Context, actionID string, params map[string]string) (*ActionResult, error) {
	action, ok := r.actions[actionID]
	if !ok {
		return nil, fmt.Errorf("action %q not found", actionID)
	}

	if params == nil {
		params = make(map[string]string)
	}

	// Validate required parameters
	for _, p := range action.Parameters {
		if p.Required {
			if v, ok := params[p.Name]; !ok || strings.TrimSpace(v) == "" {
				if p.Default == "" {
					return nil, fmt.Errorf("required parameter %q is missing", p.Name)
				}
				params[p.Name] = p.Default
			}
		}
	}

	// Apply defaults
	for _, p := range action.Parameters {
		if v, ok := params[p.Name]; !ok || strings.TrimSpace(v) == "" {
			if p.Default != "" {
				params[p.Name] = p.Default
			}
		}
	}

	exec, ok := r.executors[action.Executor]
	if !ok {
		return nil, fmt.Errorf("executor %q not registered", action.Executor)
	}

	start := time.Now()
	result, err := exec.Execute(ctx, action, params)
	if err != nil {
		return &ActionResult{
			Success:    false,
			ActionID:   actionID,
			ActionName: action.Name,
			Error:      err.Error(),
			ExecutedAt: start.UnixMilli(),
			Duration:   time.Since(start).Milliseconds(),
		}, nil
	}

	result.ActionID = actionID
	result.ActionName = action.Name
	result.ExecutedAt = start.UnixMilli()
	result.Duration = time.Since(start).Milliseconds()
	return result, nil
}

// HTTPExecutor executes actions via HTTP requests.
type HTTPExecutor struct {
	client *http.Client
}

// NewHTTPExecutor creates an HTTP executor with default timeout.
func NewHTTPExecutor() *HTTPExecutor {
	return &HTTPExecutor{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewHTTPExecutorWithTimeout creates an HTTP executor with custom timeout.
func NewHTTPExecutorWithTimeout(timeout time.Duration) *HTTPExecutor {
	return &HTTPExecutor{
		client: &http.Client{Timeout: timeout},
	}
}

func (e *HTTPExecutor) Execute(ctx context.Context, action *ActionDefinition, params map[string]string) (*ActionResult, error) {
	method := strings.ToUpper(action.Config["method"])
	if method == "" {
		method = "GET"
	}

	url := action.Config["url"]
	if url == "" {
		return nil, fmt.Errorf("action %q has no URL configured", action.ID)
	}

	// Replace template variables in URL: {param_name} -> value
	for k, v := range params {
		url = strings.ReplaceAll(url, "{"+k+"}", v)
	}

	// Replace environment variable references: ${ENV_VAR}
	url = os.ExpandEnv(url)

	var body io.Reader
	if bodyStr, ok := params["body"]; ok && bodyStr != "" {
		body = bytes.NewBufferString(bodyStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Set headers from config
	if contentType := action.Config["content_type"]; contentType != "" {
		req.Header.Set("Content-Type", contentType)
	} else if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set auth header if configured
	if authHeader := action.Config["auth_header"]; authHeader != "" {
		req.Header.Set("Authorization", os.ExpandEnv(authHeader))
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return &ActionResult{
			Success: false,
			Error:   fmt.Sprintf("HTTP request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return &ActionResult{
			Success: false,
			Error:   fmt.Sprintf("failed to read response: %v", err),
		}, nil
	}

	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	output := string(respBody)

	// Try to pretty-print JSON
	if pretty, err := json.MarshalIndent(json.RawMessage(output), "", "  "); err == nil {
		output = string(pretty)
	}

	return &ActionResult{
		Success:  success,
		Output:   output,
		Metadata: map[string]string{"status_code": fmt.Sprintf("%d", resp.StatusCode)},
	}, nil
}
