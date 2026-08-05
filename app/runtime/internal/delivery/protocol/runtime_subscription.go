package protocol

import (
	"context"
	"iter"
)

// RuntimeSubscription is the runtime-wide change notification surface. It is
// separate from workspace scope because sessions, runs, goals, and interrupts may
// change without a filesystem workspace changing.
type RuntimeSubscription interface {
	SubscribeRuntime(ctx context.Context, in RuntimeSubscribeRequest) (*RuntimeSubscribeResponse, iter.Seq[RuntimeEvent], error)
}
