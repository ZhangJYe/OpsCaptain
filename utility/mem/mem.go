package mem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	maxSessions          = 500
	sessionTTL           = 2 * time.Hour
	cleanupInterval      = 10 * time.Minute
	defaultMaxWindowSize = 20
	maxSummaryLen        = 2000
	maxSummaryItemLen    = 200
)

type sessionEntry struct {
	memory     *SimpleMemory
	lastAccess time.Time
}

var (
	sessionMap  = make(map[string]*sessionEntry)
	sessionMu   sync.Mutex
	cleanupOnce sync.Once
)

func startCleanup() {
	cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(cleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				sessionMu.Lock()
				now := time.Now()
				for id, entry := range sessionMap {
					if now.Sub(entry.lastAccess) > sessionTTL {
						delete(sessionMap, id)
					}
				}
				sessionMu.Unlock()
			}
		}()
	})
}

func loadWindowSize() int {
	v, err := g.Cfg().Get(context.Background(), "memory.max_window_size")
	if err == nil && v.Int() > 0 {
		return v.Int()
	}
	return defaultMaxWindowSize
}

func ValidateSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("session ID cannot be empty")
	}
	if len(id) > 128 {
		return fmt.Errorf("session ID too long (max 128)")
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return fmt.Errorf("session ID contains invalid character: %c", c)
		}
	}
	return nil
}

func GenerateSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func GetSimpleMemory(id string) *SimpleMemory {
	startCleanup()
	sessionMu.Lock()
	defer sessionMu.Unlock()

	entry, ok := sessionMap[id]
	if !ok {
		if len(sessionMap) >= maxSessions {
			var oldestID string
			var oldestTime time.Time
			first := true
			for sid, e := range sessionMap {
				if first || e.lastAccess.Before(oldestTime) {
					oldestID = sid
					oldestTime = e.lastAccess
					first = false
				}
			}
			if oldestID != "" {
				delete(sessionMap, oldestID)
			}
		}
		entry = &sessionEntry{
			memory: &SimpleMemory{
				ID:            id,
				Messages:      []*schema.Message{},
				MaxWindowSize: loadWindowSize(),
				turnCount:     0,
				createdAt:     time.Now(),
			},
			lastAccess: time.Now(),
		}
		sessionMap[id] = entry
	} else {
		entry.lastAccess = time.Now()
	}
	return entry.memory
}

func ClearSession(id string) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	delete(sessionMap, id)
}

func SessionCount() int {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	return len(sessionMap)
}

func GetSessionStats() map[string]SessionStats {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	stats := make(map[string]SessionStats, len(sessionMap))
	for id, entry := range sessionMap {
		m := entry.memory
		m.mu.Lock()
		stats[id] = SessionStats{
			TurnCount:    m.turnCount,
			MessageCount: len(m.Messages),
			HasSummary:   m.Summary != "",
			SummaryLen:   len(m.Summary),
			CreatedAt:    m.createdAt,
			LastAccess:   entry.lastAccess,
			TopicCount:   len(m.topics),
		}
		m.mu.Unlock()
	}
	return stats
}

type SessionStats struct {
	TurnCount    int       `json:"turn_count"`
	MessageCount int       `json:"message_count"`
	HasSummary   bool      `json:"has_summary"`
	SummaryLen   int       `json:"summary_len"`
	CreatedAt    time.Time `json:"created_at"`
	LastAccess   time.Time `json:"last_access"`
	TopicCount   int       `json:"topic_count"`
}

type TopicSegment struct {
	Topic     string    `json:"topic"`
	Summary   string    `json:"summary"`
	StartTurn int       `json:"start_turn"`
	EndTurn   int       `json:"end_turn"`
	CreatedAt time.Time `json:"created_at"`
}

type SimpleMemory struct {
	ID            string            `json:"id"`
	Messages      []*schema.Message `json:"messages"`
	Summary       string            `json:"summary"`
	MaxWindowSize int
	turnCount     int
	createdAt     time.Time
	topics        []TopicSegment
	mu            sync.Mutex
}

func (c *SimpleMemory) TurnCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.turnCount
}

func (c *SimpleMemory) AddUserAssistantPair(userMsg string, assistantMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Messages = append(c.Messages, schema.UserMessage(userMsg), schema.AssistantMessage(assistantMsg, nil))
	c.turnCount++
	c.trimWindow()
}

func (c *SimpleMemory) SetMessages(msg *schema.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Messages = append(c.Messages, msg)
	c.trimWindow()
}

func (c *SimpleMemory) trimWindow() {
	windowSize := c.MaxWindowSize
	if windowSize <= 0 {
		windowSize = defaultMaxWindowSize
	}
	if len(c.Messages) > windowSize {
		excess := len(c.Messages) - windowSize
		if excess%2 != 0 {
			excess++
		}
		if excess < len(c.Messages) {
			newSummary := c.summarizeMessages(c.Messages[:excess])
			if newSummary != "" {
				if c.Summary == "" {
					c.Summary = newSummary
				} else {
					c.Summary = c.Summary + "\n" + newSummary
				}
				if len(c.Summary) > maxSummaryLen {
					c.Summary = c.Summary[len(c.Summary)-maxSummaryLen:]
					if idx := strings.Index(c.Summary, "\n"); idx >= 0 && idx < len(c.Summary)-1 {
						c.Summary = c.Summary[idx+1:]
					}
				}
			}
			c.Messages = c.Messages[excess:]
		}
	}
}

func (c *SimpleMemory) summarizeMessages(msgs []*schema.Message) string {
	var parts []string
	for _, m := range msgs {
		if m.Content == "" {
			continue
		}
		role := string(m.Role)
		content := m.Content
		if len(content) > maxSummaryItemLen {
			content = content[:maxSummaryItemLen] + "..."
		}
		parts = append(parts, fmt.Sprintf("[%s] %s", role, content))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

func (c *SimpleMemory) GetMessages() []*schema.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := make([]*schema.Message, len(c.Messages))
	copy(copied, c.Messages)
	return copied
}

func (c *SimpleMemory) GetSummary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Summary
}

func (c *SimpleMemory) GetContextMessages() []*schema.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	var result []*schema.Message
	if c.Summary != "" {
		result = append(result, &schema.Message{
			Role:    schema.User,
			Content: fmt.Sprintf("[对话历史摘要] %s", c.Summary),
		})
		result = append(result, schema.AssistantMessage("好的，我已了解之前的对话背景，请继续。", nil))
	}
	msgs := make([]*schema.Message, len(c.Messages))
	copy(msgs, c.Messages)
	result = append(result, msgs...)
	return result
}

func (c *SimpleMemory) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Messages = []*schema.Message{}
	c.Summary = ""
	c.turnCount = 0
	c.topics = nil
}

func (c *SimpleMemory) AddTopic(topic string, summary string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	seg := TopicSegment{
		Topic:     topic,
		Summary:   summary,
		StartTurn: c.turnCount,
		EndTurn:   c.turnCount,
		CreatedAt: time.Now(),
	}
	c.topics = append(c.topics, seg)
}

func (c *SimpleMemory) GetTopics() []TopicSegment {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := make([]TopicSegment, len(c.topics))
	copy(copied, c.topics)
	return copied
}
