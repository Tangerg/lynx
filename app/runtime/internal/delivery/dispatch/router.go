package dispatch

import (
	"context"
	"fmt"
	"iter"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
)

// Router routes inbound JSON-RPC messages to typed Runtime methods and
// coordinates replay-protected mutations. Request-scoped metadata is carried
// on ctx; durable replay records live in store.
type Router struct {
	api         protocol.Runtime
	store       idempotency.Store
	replayLocks [64]sync.Mutex
	pendingMu   sync.Mutex
	pending     map[string]idempotency.Record
}

// Config supplies optional Router dependencies. A nil IdempotencyStore
// selects the Runtime-instance-local replay store.
type Config struct {
	IdempotencyStore idempotency.Store
}

// New builds a Router bound to the given Runtime. The returned
// Router is safe for parallel Handle calls.
func New(api protocol.Runtime, config Config) *Router {
	store := config.IdempotencyStore
	if store == nil {
		store = newMemoryIdempotencyStore()
	}
	router := &Router{
		api: api, store: store, pending: make(map[string]idempotency.Record),
	}
	return router
}

// HandleResult holds what the router returns after processing one
// inbound message.
type HandleResult struct {
	// Response is the synchronous JSON-RPC reply. nil when the input
	// was a notification (no id, no response on the wire).
	Response *transport.Response

	// EventStream is the sequence of stream frames for a streaming method, ending
	// when the source is drained. Frames are domain-agnostic (run events,
	// workspace events): the dispatch encodes each domain event into a
	// StreamFrame (method + params + optional SSE id) so the transport stays
	// dumb — it just writes frames. The sequence is synchronous end to end
	// (Journal → presenter → adaptStream); the transport supplies the single
	// goroutine that ranges it (streamable HTTP selects it against a heartbeat;
	// in-process ranges it straight onto Recv).
	EventStream iter.Seq[StreamFrame]
}

// Handle is the entry point — every inbound transport.Message goes through here.
func (r *Router) Handle(ctx context.Context, message transport.Message) HandleResult {
	request, ok := message.(*transport.Request)
	if !ok || request == nil {
		return responseError(transport.ID{}, badEnvelope("expected a JSON-RPC request"))
	}

	// API.md §2.2: all ids are strings. Reject non-string ids at the
	// boundary. (Absent ids — Notifications — are fine.)
	if request.ID.IsValid() {
		if _, ok := request.ID.Raw().(string); !ok {
			return responseError(request.ID, badEnvelope(
				fmt.Sprintf("id must be a JSON string, got %T", request.ID.Raw())))
		}
	}
	// Metadata stripping rewrites Params for typed decoding. Work on a shallow
	// request copy so an in-process caller can safely retain or reuse its message;
	// Params bytes themselves are read-only and replaced, never mutated in place.
	requestCopy := *request
	request = &requestCopy

	var metaErr *transport.Error
	ctx, metaErr = bindRequestMeta(ctx, request)
	if metaErr != nil {
		if !request.IsCall() {
			return HandleResult{}
		}
		return responseError(request.ID, metaErr)
	}

	// Notifications: no response. We still dispatch so cancel-style
	// notifications take effect.
	if !request.IsCall() {
		r.handleNotification(ctx, request)
		return HandleResult{}
	}

	return r.dispatchReplayProtected(ctx, request)
}
