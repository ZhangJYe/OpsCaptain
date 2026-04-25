package mem

import (
	"context"
	"time"

	"github.com/cloudwego/eino/schema"
)

type SessionMemoryStore interface {
	LoadMessages(ctx context.Context, sessionID string) ([]*schema.Message, string, error)
	SaveMessages(ctx context.Context, sessionID string, messages []*schema.Message, summary string) error
	Touch(ctx context.Context, sessionID string) error
	Delete(ctx context.Context, sessionID string) error
	Exists(ctx context.Context, sessionID string) (bool, error)
}

type LongTermMemoryBackend interface {
	StoreEntry(ctx context.Context, entry *MemoryEntry) error
	LoadEntries(ctx context.Context, sessionID string) ([]*MemoryEntry, error)
	UpdateEntry(ctx context.Context, entry *MemoryEntry) error
	DeleteEntry(ctx context.Context, id string) error
	CountBySession(ctx context.Context, sessionID string) (int, error)
	CountAll(ctx context.Context) (int, error)
}

type serializedSession struct {
	Messages  []serializedMessage `json:"messages"`
	Summary   string              `json:"summary"`
	TurnCount int                 `json:"turn_count"`
	CreatedAt int64               `json:"created_at"`
	Topics    []TopicSegment      `json:"topics,omitempty"`
}

type serializedMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func serializeMessages(msgs []*schema.Message) []serializedMessage {
	out := make([]serializedMessage, 0, len(msgs))
	for _, m := range msgs {
		if m == nil {
			continue
		}
		out = append(out, serializedMessage{
			Role:    string(m.Role),
			Content: m.Content,
		})
	}
	return out
}

func deserializeMessages(msgs []serializedMessage) []*schema.Message {
	out := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, &schema.Message{
			Role:    schema.RoleType(m.Role),
			Content: m.Content,
		})
	}
	return out
}

type serializedMemoryEntry struct {
	ID        string  `json:"id"`
	SessionID string  `json:"session_id"`
	Type      string  `json:"type"`
	Content   string  `json:"content"`
	Source    string  `json:"source"`
	Relevance float64 `json:"relevance"`
	AccessCnt int     `json:"access_count"`
	CreatedAt int64   `json:"created_at"`
	UpdatedAt int64   `json:"updated_at"`
	LastUsed  int64   `json:"last_used"`
	Decay     float64 `json:"decay"`
}

func serializeEntry(e *MemoryEntry) serializedMemoryEntry {
	return serializedMemoryEntry{
		ID:        e.ID,
		SessionID: e.SessionID,
		Type:      string(e.Type),
		Content:   e.Content,
		Source:    e.Source,
		Relevance: e.Relevance,
		AccessCnt: e.AccessCnt,
		CreatedAt: e.CreatedAt.UnixMilli(),
		UpdatedAt: e.UpdatedAt.UnixMilli(),
		LastUsed:  e.LastUsed.UnixMilli(),
		Decay:     e.Decay,
	}
}

func deserializeEntry(s serializedMemoryEntry) *MemoryEntry {
	return &MemoryEntry{
		ID:        s.ID,
		SessionID: s.SessionID,
		Type:      MemoryType(s.Type),
		Content:   s.Content,
		Source:    s.Source,
		Relevance: s.Relevance,
		AccessCnt: s.AccessCnt,
		CreatedAt: time.UnixMilli(s.CreatedAt),
		UpdatedAt: time.UnixMilli(s.UpdatedAt),
		LastUsed:  time.UnixMilli(s.LastUsed),
		Decay:     s.Decay,
	}
}
