package runtime

import (
	"context"

	"SuperBizAgent/internal/ai/protocol"
)

type Agent interface {
	Name() string
	Capabilities() []string
	Handle(ctx context.Context, task *protocol.TaskEnvelope) (*protocol.TaskResult, error)
}
