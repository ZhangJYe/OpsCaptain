package mem

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestGetSimpleMemory_CreateNew(t *testing.T) {
	sessionMu.Lock()
	sessionMap = make(map[string]*sessionEntry)
	sessionMu.Unlock()

	m := GetSimpleMemory("test-1")
	if m == nil {
		t.Fatal("expected non-nil SimpleMemory")
	}
	m2 := GetSimpleMemory("test-1")
	if m != m2 {
		t.Fatal("expected same SimpleMemory for same id")
	}
}

func TestGetSimpleMemory_MaxSessions(t *testing.T) {
	sessionMu.Lock()
	sessionMap = make(map[string]*sessionEntry)
	sessionMu.Unlock()

	for i := 0; i < maxSessions+10; i++ {
		GetSimpleMemory(time.Now().String() + string(rune(i)))
	}

	sessionMu.Lock()
	count := len(sessionMap)
	sessionMu.Unlock()

	if count > maxSessions {
		t.Fatalf("expected at most %d sessions, got %d", maxSessions, count)
	}
}

func TestSimpleMemory_SetAndGetMessages(t *testing.T) {
	m := &SimpleMemory{}
	msg1 := schema.UserMessage("hello")
	msg2 := schema.UserMessage("world")

	m.SetMessages(msg1)
	m.SetMessages(msg2)

	msgs := m.GetMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Fatalf("expected 'hello', got '%s'", msgs[0].Content)
	}
	if msgs[1].Content != "world" {
		t.Fatalf("expected 'world', got '%s'", msgs[1].Content)
	}
}

func TestSimpleMemory_GetMessages_ReturnsCopy(t *testing.T) {
	m := &SimpleMemory{}
	m.SetMessages(schema.UserMessage("original"))

	msgs := m.GetMessages()
	msgs[0] = schema.UserMessage("modified")

	msgsAgain := m.GetMessages()
	if msgsAgain[0].Content != "original" {
		t.Fatalf("GetMessages should return a copy, but original was modified")
	}
}

func TestSimpleMemory_ConcurrentAccess(t *testing.T) {
	m := &SimpleMemory{}
	var wg sync.WaitGroup
	count := 100

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			m.SetMessages(schema.UserMessage("msg"))
			_ = m.GetMessages()
		}(i)
	}

	wg.Wait()

	msgs := m.GetMessages()
	if len(msgs) == 0 {
		t.Fatal("expected non-zero messages after concurrent writes")
	}
	if len(msgs) > defaultMaxWindowSize {
		t.Fatalf("expected at most %d messages due to window, got %d", defaultMaxWindowSize, len(msgs))
	}
}

func TestSimpleMemory_SlidingWindow(t *testing.T) {
	m := &SimpleMemory{}

	for i := 0; i < defaultMaxWindowSize+10; i++ {
		m.SetMessages(schema.UserMessage("msg"))
	}

	msgs := m.GetMessages()
	if len(msgs) != defaultMaxWindowSize {
		t.Fatalf("expected %d messages due to sliding window, got %d", defaultMaxWindowSize, len(msgs))
	}
}

func TestGetSimpleMemory_UpdatesLastAccess(t *testing.T) {
	sessionMu.Lock()
	sessionMap = make(map[string]*sessionEntry)
	sessionMu.Unlock()

	GetSimpleMemory("access-test")
	sessionMu.Lock()
	t1 := sessionMap["access-test"].lastAccess
	sessionMu.Unlock()

	time.Sleep(10 * time.Millisecond)

	GetSimpleMemory("access-test")
	sessionMu.Lock()
	t2 := sessionMap["access-test"].lastAccess
	sessionMu.Unlock()

	if !t2.After(t1) {
		t.Fatal("expected lastAccess to be updated on re-access")
	}
}

func TestSimpleMemory_AddUserAssistantPair(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 20}
	m.AddUserAssistantPair("hello", "hi there")

	msgs := m.GetMessages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != schema.User || msgs[0].Content != "hello" {
		t.Fatalf("expected user message 'hello', got %s: '%s'", msgs[0].Role, msgs[0].Content)
	}
	if msgs[1].Role != schema.Assistant || msgs[1].Content != "hi there" {
		t.Fatalf("expected assistant message 'hi there', got %s: '%s'", msgs[1].Role, msgs[1].Content)
	}
	if m.TurnCount() != 1 {
		t.Fatalf("expected turnCount 1, got %d", m.TurnCount())
	}
}

func TestSimpleMemory_SummaryOnEviction(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 4}

	m.AddUserAssistantPair("question 1", "answer 1")
	m.AddUserAssistantPair("question 2", "answer 2")
	m.AddUserAssistantPair("question 3", "answer 3")

	summary := m.GetSummary()
	if summary == "" {
		t.Fatal("expected non-empty summary after eviction")
	}

	msgs := m.GetMessages()
	if len(msgs) > 4 {
		t.Fatalf("expected at most 4 messages, got %d", len(msgs))
	}
}

func TestSimpleMemory_GetContextMessages(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 4}

	m.AddUserAssistantPair("q1", "a1")
	m.AddUserAssistantPair("q2", "a2")
	m.AddUserAssistantPair("q3", "a3")

	ctxMsgs := m.GetContextMessages()

	if m.GetSummary() != "" {
		if ctxMsgs[0].Role != schema.User {
			t.Fatal("expected first context message to be user role (summary carrier)")
		}
		if !strings.Contains(ctxMsgs[0].Content, "[对话历史摘要]") {
			t.Fatal("expected summary marker in first message")
		}
		if ctxMsgs[1].Role != schema.Assistant {
			t.Fatal("expected second context message to be assistant acknowledgement")
		}
		windowMsgs := m.GetMessages()
		if len(ctxMsgs) != len(windowMsgs)+2 {
			t.Fatalf("expected context messages = 2 (summary pair) + %d (window), got %d", len(windowMsgs), len(ctxMsgs))
		}
	}
}

func TestSimpleMemory_SummaryContentTruncation(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 2}

	longContent := ""
	for i := 0; i < 50; i++ {
		longContent += fmt.Sprintf("word%d ", i)
	}
	m.AddUserAssistantPair(longContent, "short answer")

	m.AddUserAssistantPair("new question", "new answer")

	summary := m.GetSummary()
	if len(summary) == 0 {
		t.Fatal("expected non-empty summary")
	}
}

func TestValidateSessionID(t *testing.T) {
	tests := []struct {
		id      string
		wantErr bool
	}{
		{"", true},
		{"valid-session_123", false},
		{"abc", false},
		{"ABC-xyz_09", false},
		{"has space", true},
		{"has/slash", true},
		{"has.dot", true},
		{"has@at", true},
		{strings.Repeat("a", 128), false},
		{strings.Repeat("a", 129), true},
		{"injection'; DROP TABLE--", true},
	}
	for _, tt := range tests {
		err := ValidateSessionID(tt.id)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateSessionID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
		}
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := GenerateSessionID()
	id2 := GenerateSessionID()
	if id1 == id2 {
		t.Fatal("expected unique session IDs")
	}
	if len(id1) != 32 {
		t.Fatalf("expected 32 char hex string, got %d chars", len(id1))
	}
	if err := ValidateSessionID(id1); err != nil {
		t.Fatalf("generated ID should be valid: %v", err)
	}
}

func TestSimpleMemory_SummaryBoundedGrowth(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 2}

	for i := 0; i < 100; i++ {
		m.AddUserAssistantPair(
			fmt.Sprintf("long question %d with lots of detail about topic xyz", i),
			fmt.Sprintf("long answer %d with comprehensive explanation of results", i),
		)
	}

	summary := m.GetSummary()
	if len(summary) > maxSummaryLen+200 {
		t.Fatalf("summary too long: %d chars (max should be ~%d)", len(summary), maxSummaryLen)
	}
	if summary == "" {
		t.Fatal("expected non-empty summary after many turns")
	}
}

func TestSimpleMemory_Reset(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 20}
	m.AddUserAssistantPair("q1", "a1")
	m.AddUserAssistantPair("q2", "a2")

	if m.TurnCount() != 2 {
		t.Fatalf("expected 2 turns, got %d", m.TurnCount())
	}

	m.Reset()

	if m.TurnCount() != 0 {
		t.Fatalf("expected 0 turns after reset, got %d", m.TurnCount())
	}
	if len(m.GetMessages()) != 0 {
		t.Fatal("expected 0 messages after reset")
	}
	if m.GetSummary() != "" {
		t.Fatal("expected empty summary after reset")
	}
}

func TestClearSession(t *testing.T) {
	sessionMu.Lock()
	sessionMap = make(map[string]*sessionEntry)
	sessionMu.Unlock()

	GetSimpleMemory("sess-to-clear")
	if SessionCount() != 1 {
		t.Fatal("expected 1 session")
	}

	ClearSession("sess-to-clear")
	if SessionCount() != 0 {
		t.Fatal("expected 0 sessions after clear")
	}
}

func TestSimpleMemory_ContextMessagesNoSummary(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 20}
	m.AddUserAssistantPair("hello", "hi")

	ctxMsgs := m.GetContextMessages()
	if len(ctxMsgs) != 2 {
		t.Fatalf("expected 2 context messages (no summary), got %d", len(ctxMsgs))
	}
	if ctxMsgs[0].Role != schema.User || ctxMsgs[0].Content != "hello" {
		t.Fatalf("first message should be user 'hello'")
	}
}

func TestSimpleMemory_ContextMessagesSummaryAsPair(t *testing.T) {
	m := &SimpleMemory{MaxWindowSize: 2}

	m.AddUserAssistantPair("old-q", "old-a")
	m.AddUserAssistantPair("new-q", "new-a")

	ctxMsgs := m.GetContextMessages()

	var hasUserSummary, hasAssistantAck bool
	for _, msg := range ctxMsgs {
		if msg.Role == schema.User && strings.Contains(msg.Content, "[对话历史摘要]") {
			hasUserSummary = true
		}
		if msg.Role == schema.Assistant && strings.Contains(msg.Content, "了解") {
			hasAssistantAck = true
		}
	}

	if m.GetSummary() != "" {
		if !hasUserSummary || !hasAssistantAck {
			t.Fatal("summary should be injected as user+assistant pair to maintain role alternation")
		}
	}
}

func TestSimpleMemory_ConcurrentSessionAccess(t *testing.T) {
	sessionMu.Lock()
	sessionMap = make(map[string]*sessionEntry)
	sessionMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-sess-%d", idx%5)
			m := GetSimpleMemory(id)
			m.AddUserAssistantPair(
				fmt.Sprintf("q from goroutine %d", idx),
				fmt.Sprintf("a from goroutine %d", idx),
			)
			_ = m.GetContextMessages()
		}(i)
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("concurrent-sess-%d", i)
		m := GetSimpleMemory(id)
		msgs := m.GetMessages()
		for j := 0; j < len(msgs)-1; j++ {
			if msgs[j].Role == msgs[j+1].Role && msgs[j].Role != schema.System {
				t.Fatalf("session %s: consecutive messages with same role at index %d and %d: %s",
					id, j, j+1, msgs[j].Role)
			}
		}
	}
}
