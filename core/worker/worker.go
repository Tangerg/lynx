package worker

import (
	"context"
	"github.com/Tangerg/lynx/core/message"
)

type Worker interface {
	Work()
}

type BatchWorker interface {
	Worker
	Context(ctx context.Context)
	Done() <-chan struct{}
}

type StreamWorker interface {
	Work(ctx context.Context, msg *message.Msg) ([]*message.Msg, error)
	Sleep()
}
