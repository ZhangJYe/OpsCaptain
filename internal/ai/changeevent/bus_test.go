package changeevent

import (
	"SuperBizAgent/internal/ai/protocol"
	"context"
	"sync"
	"testing"
	"time"
)

// mockStore 是 ChangeEventStore 的测试替身。
type mockStore struct {
	events  map[string]*protocol.ChangeEvent
	dedupes map[string]string
}

func newMockStore() *mockStore {
	return &mockStore{
		events:  make(map[string]*protocol.ChangeEvent),
		dedupes: make(map[string]string),
	}
}

func (m *mockStore) Save(_ context.Context, event *protocol.ChangeEvent) error {
	m.events[event.EventID] = event
	if event.DedupeKey != "" {
		m.dedupes[event.DedupeKey] = event.EventID
	}
	return nil
}

func (m *mockStore) ReserveDedupeKey(_ context.Context, key string, eventID string) (bool, string, error) {
	if key == "" {
		return true, "", nil
	}
	if existingID := m.dedupes[key]; existingID != "" {
		return false, existingID, nil
	}
	m.dedupes[key] = eventID
	return true, "", nil
}

func (m *mockStore) ReleaseDedupeKey(_ context.Context, key string) error {
	delete(m.dedupes, key)
	return nil
}

func (m *mockStore) GetByID(_ context.Context, eventID string) (*protocol.ChangeEvent, error) {
	return m.events[eventID], nil
}

func (m *mockStore) ExistsByDedupeKey(_ context.Context, key string) (bool, error) {
	if key == "" {
		return false, nil
	}
	return m.dedupes[key] != "", nil
}

func (m *mockStore) Query(_ context.Context, filter protocol.ChangeEventFilter) ([]*protocol.ChangeEvent, error) {
	var result []*protocol.ChangeEvent
	for _, e := range m.events {
		if len(filter.Services) > 0 {
			found := false
			for _, svc := range filter.Services {
				if e.Service == svc {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		result = append(result, e)
	}
	return result, nil
}

func (m *mockStore) Delete(_ context.Context, eventID string) error {
	delete(m.events, eventID)
	return nil
}

func (m *mockStore) Cleanup(_ context.Context, before time.Time) (int, error) {
	return 0, nil
}

// mockHandler 记录处理的事件。
type mockHandler struct {
	mu     sync.Mutex
	name   string
	events []*protocol.ChangeEvent
}

func (h *mockHandler) Name() string { return h.name }
func (h *mockHandler) Handle(_ context.Context, event *protocol.ChangeEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	return nil
}

func (h *mockHandler) snapshot() []*protocol.ChangeEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*protocol.ChangeEvent, len(h.events))
	copy(out, h.events)
	return out
}

func TestChangeEventBus_Ingest_Basic(t *testing.T) {
	store := newMockStore()
	bus := NewChangeEventBus(store, 100)

	event := &protocol.ChangeEvent{
		Source:    "cicd",
		EventType: "deploy",
		Service:   "user-service",
		Env:       "prod",
		Summary:   "v1.2.3 → v1.2.4",
		StartedAt: time.Now(),
	}

	eventID, accepted, err := bus.Ingest(context.Background(), event)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if !accepted {
		t.Fatal("expected event to be accepted")
	}
	if eventID == "" {
		t.Fatal("expected event ID to be set")
	}

	// 验证存储
	stored, _ := store.GetByID(context.Background(), eventID)
	if stored == nil {
		t.Fatal("event not found in store")
	}
	if stored.Service != "user-service" {
		t.Fatalf("expected service user-service, got %s", stored.Service)
	}
	if stored.RiskLevel == "" {
		t.Fatal("expected risk level to be assessed")
	}
}

func TestChangeEventBus_Ingest_Dedupe(t *testing.T) {
	store := newMockStore()
	bus := NewChangeEventBus(store, 100)

	event := &protocol.ChangeEvent{
		Source:    "cicd",
		EventType: "deploy",
		Service:   "user-service",
		Summary:   "v1.2.3 → v1.2.4",
		StartedAt: time.Now(),
		DedupeKey: "test-dedupe-key",
	}

	// 第一次应该成功
	_, accepted1, err := bus.Ingest(context.Background(), event)
	if err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if !accepted1 {
		t.Fatal("first event should be accepted")
	}

	// 第二次应该被去重
	event2 := &protocol.ChangeEvent{
		Source:    "cicd",
		EventType: "deploy",
		Service:   "user-service",
		Summary:   "v1.2.3 → v1.2.4",
		StartedAt: time.Now(),
		DedupeKey: "test-dedupe-key",
	}
	_, accepted2, err := bus.Ingest(context.Background(), event2)
	if err != nil {
		t.Fatalf("second ingest failed: %v", err)
	}
	if accepted2 {
		t.Fatal("duplicate event should not be accepted")
	}
}

func TestChangeEventBus_Ingest_Normalize(t *testing.T) {
	store := newMockStore()
	bus := NewChangeEventBus(store, 100)

	event := &protocol.ChangeEvent{
		EventType: "deploy",
		Service:   "user-service",
		Summary:   "test",
	}

	eventID, accepted, err := bus.Ingest(context.Background(), event)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}
	if !accepted {
		t.Fatal("expected accepted")
	}

	stored, _ := store.GetByID(context.Background(), eventID)
	if stored.EventID == "" {
		t.Fatal("expected event_id to be generated")
	}
	if stored.Env != "unknown" {
		t.Fatalf("expected default env=unknown, got %s", stored.Env)
	}
	if stored.Source != "manual" {
		t.Fatalf("expected default source=manual, got %s", stored.Source)
	}
	if stored.StartedAt.IsZero() {
		t.Fatal("expected started_at to be set")
	}
}

func TestChangeEventBus_Ingest_Validation(t *testing.T) {
	store := newMockStore()
	bus := NewChangeEventBus(store, 100)

	// 缺少 service
	_, _, err := bus.Ingest(context.Background(), &protocol.ChangeEvent{
		EventType: "deploy",
		Summary:   "test",
	})
	if err == nil {
		t.Fatal("expected error for missing service")
	}

	// 缺少 event_type
	_, _, err = bus.Ingest(context.Background(), &protocol.ChangeEvent{
		Service: "user-service",
		Summary: "test",
	})
	if err == nil {
		t.Fatal("expected error for missing event_type")
	}
}

func TestChangeEventBus_HandlerFanout(t *testing.T) {
	store := newMockStore()
	bus := NewChangeEventBus(store, 100)

	handler := &mockHandler{name: "test"}
	bus.Register(handler)

	event := &protocol.ChangeEvent{
		Source:    "cicd",
		EventType: "deploy",
		Service:   "user-service",
		Summary:   "test",
		StartedAt: time.Now(),
	}

	_, _, err := bus.Ingest(context.Background(), event)
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	// 等待异步 handler 执行
	time.Sleep(100 * time.Millisecond)

	received := handler.snapshot()
	if len(received) != 1 {
		t.Fatalf("expected handler to receive 1 event, got %d", len(received))
	}
	if received[0].Service != "user-service" {
		t.Fatalf("expected handler event service=user-service, got %s", received[0].Service)
	}
}

func TestAssessRiskLevel(t *testing.T) {
	tests := []struct {
		name     string
		event    *protocol.ChangeEvent
		expected string
	}{
		{
			name:     "prod deploy → high",
			event:    &protocol.ChangeEvent{Env: "prod", EventType: "deploy"},
			expected: "high",
		},
		{
			name:     "prod rollback → high",
			event:    &protocol.ChangeEvent{Env: "prod", EventType: "rollback"},
			expected: "high",
		},
		{
			name:     "prod config_update → medium",
			event:    &protocol.ChangeEvent{Env: "prod", EventType: "config_update"},
			expected: "medium",
		},
		{
			name:     "prod scale → medium",
			event:    &protocol.ChangeEvent{Env: "prod", EventType: "scale"},
			expected: "medium",
		},
		{
			name:     "staging deploy → low",
			event:    &protocol.ChangeEvent{Env: "staging", EventType: "deploy"},
			expected: "low",
		},
		{
			name:     "dev restart → low",
			event:    &protocol.ChangeEvent{Env: "dev", EventType: "restart"},
			expected: "low",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := assessRiskLevel(tt.event)
			if result != tt.expected {
				t.Fatalf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestRingBuffer_RecentByService(t *testing.T) {
	rb := newRingBuffer(10)
	now := time.Now()

	rb.Add(&protocol.ChangeEvent{Service: "user-service", EventType: "deploy", StartedAt: now.Add(-1 * time.Hour)})
	rb.Add(&protocol.ChangeEvent{Service: "order-service", EventType: "scale", StartedAt: now.Add(-30 * time.Minute)})
	rb.Add(&protocol.ChangeEvent{Service: "user-service", EventType: "config_update", StartedAt: now.Add(-10 * time.Minute)})

	// 查询 user-service
	results := rb.RecentByService("user-service", now.Add(-2*time.Hour), 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 user-service events, got %d", len(results))
	}

	// 查询 order-service
	results = rb.RecentByService("order-service", now.Add(-2*time.Hour), 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 order-service event, got %d", len(results))
	}

	// 查询不存在的服务
	results = rb.RecentByService("nonexistent", now.Add(-2*time.Hour), 10)
	if len(results) != 0 {
		t.Fatalf("expected 0 events, got %d", len(results))
	}

	// 时间窗口过滤
	results = rb.RecentByService("user-service", now.Add(-20*time.Minute), 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 event in 20min window, got %d", len(results))
	}
}

func TestDebounceTracker(t *testing.T) {
	dt := NewDebounceTracker(1 * time.Second)

	if dt.IsDuplicate("user-service") {
		t.Fatal("first call should not be duplicate")
	}
	if !dt.IsDuplicate("user-service") {
		t.Fatal("second call within window should be duplicate")
	}
	if !dt.IsDuplicate("user-service") {
		t.Fatal("third call within window should be duplicate")
	}

	// 不同服务不受影响
	if dt.IsDuplicate("order-service") {
		t.Fatal("different service should not be duplicate")
	}
}
