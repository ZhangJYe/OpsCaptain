package skills

import "time"

// Status constants for user-created tools and skills.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusRejected = "rejected"
	StatusDisabled = "disabled"
)

// Transport constants for MCP tool connections.
const (
	TransportSSE  = "sse"
	TransportHTTP = "http"
)

// Parser constants for skill output parsing.
const (
	ParserJSONArray  = "json_array"
	ParserJSONNested = "json_nested"
	ParserLogLines   = "log_lines"
	ParserRaw        = "raw"
)

// Domain constants for skill categorization.
const (
	DomainMetrics   = "metrics"
	DomainLogs      = "logs"
	DomainKnowledge = "knowledge"
	DomainCustom    = "custom"
)

// UserMCPTool represents a user-defined MCP tool configuration.
type UserMCPTool struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Transport   string         `json:"transport"`
	EndpointURL string         `json:"endpoint_url"`
	HTTPURL     string         `json:"http_url"`
	AuthToken   string         `json:"auth_token"`
	ToolName    string         `json:"tool_name"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	TimeoutMs   int            `json:"timeout_ms"`
	Status      string         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	CreatedBy   string         `json:"created_by"`
	ApprovedAt  *time.Time     `json:"approved_at,omitempty"`
	ApprovedBy  string         `json:"approved_by,omitempty"`
}

// UserSkill represents a user-defined skill that references a user MCP tool.
type UserSkill struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Domain       string   `json:"domain"`
	ToolRefID    string   `json:"tool_ref_id"`
	Focus        string   `json:"focus"`
	OutputParser string   `json:"output_parser"`
	JSONPath     string   `json:"json_path"`
	Keywords     []string `json:"keywords,omitempty"`
	Tier         int      `json:"tier"`
	Status       string   `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	CreatedBy    string    `json:"created_by"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	ApprovedBy   string    `json:"approved_by,omitempty"`
}

// UserRegistryData holds all user-created tools and skills.
type UserRegistryData struct {
	Tools  []UserMCPTool `json:"tools"`
	Skills []UserSkill   `json:"skills"`
}
