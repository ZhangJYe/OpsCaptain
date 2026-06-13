package memory

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type MemoryEvent struct {
	SessionID        string
	UserID           string
	ProjectID        string
	Query            string
	Answer           string
	TraceID          string
	ExistingMemories []*MemoryEntry
}

type MemoryActionOp string

const (
	MemoryActionSkip      MemoryActionOp = "skip"
	MemoryActionUpsert    MemoryActionOp = "upsert"
	MemoryActionSupersede MemoryActionOp = "supersede"
	MemoryActionPromote   MemoryActionOp = "promote"
)

type MemoryDecision struct {
	Actions []MemoryAction `json:"actions"`
}

type MemoryAction struct {
	Op            MemoryActionOp `json:"op"`
	TargetID      string         `json:"target_id,omitempty"`
	Type          MemoryType     `json:"type,omitempty"`
	Content       string         `json:"content,omitempty"`
	Scope         MemoryScope    `json:"scope,omitempty"`
	ScopeID       string         `json:"scope_id,omitempty"`
	Confidence    float64        `json:"confidence,omitempty"`
	ConflictGroup string         `json:"conflict_group,omitempty"`
	ExpiresAt     int64          `json:"expires_at,omitempty"`
	Reason        string         `json:"reason,omitempty"`
}

type MemoryAuditRecord struct {
	ID        string         `json:"id"`
	MemoryID  string         `json:"memory_id,omitempty"`
	Op        MemoryActionOp `json:"op"`
	Reason    string         `json:"reason,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	Actor     string         `json:"actor"`
	Before    *MemoryEntry   `json:"before,omitempty"`
	After     *MemoryEntry   `json:"after,omitempty"`
	CreatedAt int64          `json:"created_at"`
}

type MemoryAgent interface {
	Decide(ctx context.Context, event MemoryEvent) (*MemoryDecision, error)
}

type MemoryChatModel interface {
	Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
}