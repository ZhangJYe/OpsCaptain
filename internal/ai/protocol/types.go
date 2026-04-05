package protocol

import (
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusSucceeded TaskStatus = "succeeded"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type ResultStatus string

const (
	ResultStatusSucceeded ResultStatus = "succeeded"
	ResultStatusFailed    ResultStatus = "failed"
	ResultStatusDegraded  ResultStatus = "degraded"
)

type MemoryRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type ArtifactRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	URI  string `json:"uri,omitempty"`
}

type Artifact struct {
	Ref       ArtifactRef    `json:"ref"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

type TaskEnvelope struct {
	TaskID       string         `json:"task_id"`
	ParentTaskID string         `json:"parent_task_id,omitempty"`
	SessionID    string         `json:"session_id"`
	TraceID      string         `json:"trace_id"`
	Goal         string         `json:"goal"`
	Assignee     string         `json:"assignee"`
	Creator      string         `json:"creator"`
	Intent       string         `json:"intent,omitempty"`
	Priority     string         `json:"priority,omitempty"`
	Status       TaskStatus     `json:"status"`
	Input        map[string]any `json:"input,omitempty"`
	Constraints  map[string]any `json:"constraints,omitempty"`
	MemoryRefs   []MemoryRef    `json:"memory_refs,omitempty"`
	ArtifactRefs []ArtifactRef  `json:"artifact_refs,omitempty"`
	CreatedAt    int64          `json:"created_at"`
	UpdatedAt    int64          `json:"updated_at"`
	DeadlineAt   int64          `json:"deadline_at,omitempty"`
}

type TaskError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type EvidenceItem struct {
	SourceType string  `json:"source_type"`
	SourceID   string  `json:"source_id"`
	Title      string  `json:"title"`
	Snippet    string  `json:"snippet"`
	Score      float64 `json:"score"`
	URI        string  `json:"uri,omitempty"`
}

type TaskResult struct {
	TaskID       string         `json:"task_id"`
	Agent        string         `json:"agent"`
	Status       ResultStatus   `json:"status"`
	Summary      string         `json:"summary"`
	Confidence   float64        `json:"confidence"`
	Evidence     []EvidenceItem `json:"evidence,omitempty"`
	ArtifactRefs []ArtifactRef  `json:"artifact_refs,omitempty"`
	NextActions  []string       `json:"next_actions,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Error        *TaskError     `json:"error,omitempty"`
	StartedAt    int64          `json:"started_at"`
	FinishedAt   int64          `json:"finished_at"`
}

type TaskEvent struct {
	EventID   string         `json:"event_id"`
	TaskID    string         `json:"task_id"`
	TraceID   string         `json:"trace_id"`
	Type      string         `json:"type"`
	Agent     string         `json:"agent"`
	Message   string         `json:"message,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

func NewRootTask(sessionID, goal, assignee string) *TaskEnvelope {
	now := time.Now().UnixMilli()
	traceID := uuid.NewString()
	return &TaskEnvelope{
		TaskID:    uuid.NewString(),
		SessionID: sessionID,
		TraceID:   traceID,
		Goal:      goal,
		Assignee:  assignee,
		Creator:   "controller",
		Status:    TaskStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func NewChildTask(parent *TaskEnvelope, assignee, goal string, input map[string]any) *TaskEnvelope {
	now := time.Now().UnixMilli()
	return &TaskEnvelope{
		TaskID:       uuid.NewString(),
		ParentTaskID: parent.TaskID,
		SessionID:    parent.SessionID,
		TraceID:      parent.TraceID,
		Goal:         goal,
		Assignee:     assignee,
		Creator:      parent.Assignee,
		Status:       TaskStatusPending,
		Input:        input,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}
