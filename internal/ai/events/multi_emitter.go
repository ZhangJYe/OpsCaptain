package events

import "context"

// MultiEmitter 将事件同时发射到多个目标
type MultiEmitter struct {
	emitters []Emitter
}

// NewMultiEmitter 创建多目标发射器
func NewMultiEmitter(emitters ...Emitter) *MultiEmitter {
	return &MultiEmitter{emitters: emitters}
}

// Emit 同时发射到所有目标
func (m *MultiEmitter) Emit(ctx context.Context, event AgentEvent) {
	for _, e := range m.emitters {
		e.Emit(ctx, event)
	}
}
