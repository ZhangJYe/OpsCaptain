package runtime

import (
	"context"

	"SuperBizAgent/internal/ai/protocol"
)

type Bus interface {
	Publish(ctx context.Context, event *protocol.TaskEvent) error
}

type LedgerBus struct {
	ledger Ledger
}

func NewLedgerBus(ledger Ledger) *LedgerBus {
	return &LedgerBus{ledger: ledger}
}

func (b *LedgerBus) Publish(ctx context.Context, event *protocol.TaskEvent) error {
	return b.ledger.AppendEvent(ctx, event)
}
