package embedded

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// SubscribeRuntime observes Runtime-wide change topics and file watches.
func (r *Runtime) SubscribeRuntime(ctx context.Context, request protocol.RuntimeSubscribeRequest, options SubscriptionOptions) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	return invokeStream[protocol.RuntimeSubscribeRequest, *protocol.RuntimeSubscribeResponse, protocol.RuntimeEvent](ctx, r, "runtime.subscribe", request, subscriptionOptions(options))
}
