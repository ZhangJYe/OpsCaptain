package changeevent

import (
	"sync"
	"time"
)

// AnalysisRecord 记录一次变更事件触发的 AIOps 分析。
type AnalysisRecord struct {
	TraceID   string
	TaskID    string
	Service   string
	EventType string
	Summary   string
	StartedAt time.Time
}

// AnalysisTracker 在 ProactiveAnalyzer 和 AnalysisResultNotifier 之间
// 传递 trace_id。handler 之间通过 deep copy 事件无法共享状态，
// 因此用一个独立的内存 store 做桥接。
type AnalysisTracker struct {
	mu      sync.RWMutex
	records map[string]*AnalysisRecord // event_id → record
	maxSize int
}

// NewAnalysisTracker 创建分析跟踪器。
func NewAnalysisTracker(maxSize int) *AnalysisTracker {
	if maxSize <= 0 {
		maxSize = 500
	}
	return &AnalysisTracker{
		records: make(map[string]*AnalysisRecord, maxSize),
		maxSize: maxSize,
	}
}

// Store 记录一次分析的 trace_id。
func (t *AnalysisTracker) Store(eventID string, record *AnalysisRecord) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.records) >= t.maxSize {
		t.evict()
	}
	t.records[eventID] = record
}

// Load 获取 eventID 对应的分析记录。
func (t *AnalysisTracker) Load(eventID string) (*AnalysisRecord, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	r, ok := t.records[eventID]
	return r, ok
}

// Delete 删除一条记录。
func (t *AnalysisTracker) Delete(eventID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.records, eventID)
}

// evict 清理最老的一半记录（调用方已持有写锁）。
func (t *AnalysisTracker) evict() {
	if len(t.records) <= t.maxSize/2 {
		return
	}
	// 找最早的一半删除
	type entry struct {
		id    string
		time  time.Time
	}
	entries := make([]entry, 0, len(t.records))
	for id, r := range t.records {
		entries = append(entries, entry{id: id, time: r.StartedAt})
	}
	// 简单按时间排序
	for i := 0; i < len(entries)/2; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].time.Before(entries[i].time) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	for i := 0; i < len(entries)/2; i++ {
		delete(t.records, entries[i].id)
	}
}
