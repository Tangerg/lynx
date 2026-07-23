package dispatch

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/component/idempotency"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// Dispatcher routes inbound JSON-RPC messages to typed Runtime methods and
// coordinates replay-protected mutations. Request-scoped metadata is carried
// on ctx; durable replay records live in store.
type Dispatcher struct {
	api         protocol.Runtime
	store       idempotency.Store
	replayLocks [64]sync.Mutex
	pendingMu   sync.Mutex
	pending     map[string]idempotency.Record
}

type Option func(*Dispatcher)

// WithIdempotencyStore replaces the default process-local replay store.
func WithIdempotencyStore(store idempotency.Store) Option {
	return func(dispatcher *Dispatcher) {
		if store != nil {
			dispatcher.store = store
		}
	}
}

// New builds a Dispatcher bound to the given Runtime. The returned
// Dispatcher is safe for parallel Handle calls.
func New(api protocol.Runtime, options ...Option) *Dispatcher {
	dispatcher := &Dispatcher{
		api: api, store: newMemoryIdempotencyStore(), pending: make(map[string]idempotency.Record),
	}
	for _, option := range options {
		option(dispatcher)
	}
	return dispatcher
}

// HandleResult holds what the dispatcher returns after processing one
// inbound message.
type HandleResult struct {
	// Response is the synchronous JSON-RPC reply. nil when the input
	// was a notification (no id, no response on the wire).
	Response *transport.Response

	// EventStream is the channel of stream frames for a streaming method,
	// closed when the stream ends. Frames are domain-agnostic (run events,
	// workspace events): the dispatch encodes each domain event into a
	// StreamFrame (method + params + optional SSE id) so the transport stays
	// dumb — it just writes frames. Under streamable HTTP the transport drains
	// it straight into the call's own text/event-stream response.
	EventStream <-chan StreamFrame
}

// Handle is the entry point — every inbound transport.Message goes through here.
func (d *Dispatcher) Handle(ctx context.Context, msg transport.Message) HandleResult {
	req, ok := msg.(*transport.Request)
	if !ok || req == nil {
		return responseError(transport.ID{}, badEnvelope("expected a JSON-RPC request"))
	}

	// API.md §2.2: all ids are strings. Reject non-string ids at the
	// boundary. (Absent ids — Notifications — are fine.)
	if req.ID.IsValid() {
		if _, ok := req.ID.Raw().(string); !ok {
			return responseError(req.ID, badEnvelope(
				fmt.Sprintf("id must be a JSON string, got %T", req.ID.Raw())))
		}
	}
	// Metadata stripping rewrites Params for typed decoding. Work on a shallow
	// request copy so an in-process caller can safely retain or reuse its message;
	// Params bytes themselves are read-only and replaced, never mutated in place.
	requestCopy := *req
	req = &requestCopy

	var metaErr *transport.Error
	ctx, metaErr = bindRequestMeta(ctx, req)
	if metaErr != nil {
		if !req.IsCall() {
			return HandleResult{}
		}
		return responseError(req.ID, metaErr)
	}

	// Notifications: no response. We still dispatch so cancel-style
	// notifications take effect.
	if !req.IsCall() {
		d.handleNotification(ctx, req)
		return HandleResult{}
	}

	return d.dispatchReplayProtected(ctx, req)
}
