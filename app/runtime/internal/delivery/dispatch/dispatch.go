package dispatch

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// Dispatcher routes inbound JSON-RPC messages to typed Runtime
// methods. One instance per connection — it carries the per-conn
// handshake state (initialized) which the HTTP transport keys by
// request affinity and the InProcess transport sees as a single
// long-lived value.
type Dispatcher struct {
	api protocol.Runtime

	// initialized flips once runtime.initialize succeeds. Until then
	// every business method returns capability_not_negotiated. atomic
	// so concurrent dispatch goroutines see a consistent view.
	initialized atomic.Bool
}

// New builds a Dispatcher bound to the given Runtime. The returned
// Dispatcher is safe for parallel Handle calls; per-conn state lives on
// it, so use one Dispatcher per logical connection.
func New(api protocol.Runtime) *Dispatcher {
	return &Dispatcher{api: api}
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

// Handle is the entry point — every inbound transport.Message goes
// through here. ExpectedMethod, when non-empty, is the method the
// transport extracted from the URL path (POST /v2/rpc/{method}); the
// dispatcher cross-checks it against the body method.
func (d *Dispatcher) Handle(ctx context.Context, msg transport.Message, expectedMethod string) HandleResult {
	req, ok := msg.(*transport.Request)
	if !ok || req == nil {
		return responseError(transport.ID{}, badEnvelope("expected a JSON-RPC request"))
	}

	if expectedMethod != "" && expectedMethod != req.Method {
		return responseError(req.ID, badEnvelope(fmt.Sprintf(
			"url method %q does not match body method %q", expectedMethod, req.Method,
		)))
	}

	// API.md §2.2: all ids are strings. Reject non-string ids at the
	// boundary. (Absent ids — Notifications — are fine.)
	if req.ID.IsValid() {
		if _, ok := req.ID.Raw().(string); !ok {
			return responseError(req.ID, badEnvelope(
				fmt.Sprintf("id must be a JSON string, got %T", req.ID.Raw())))
		}
	}

	// Notifications: no response. We still dispatch so cancel-style
	// notifications take effect.
	if !req.IsCall() {
		d.handleNotification(ctx, req)
		return HandleResult{}
	}

	// Gate business methods behind initialize.
	if !d.initialized.Load() && req.Method != MethodInitialize && req.Method != MethodPing {
		return responseError(req.ID, notInitialized(
			"runtime.initialize must succeed before any business method"))
	}

	return d.dispatchRequest(ctx, req)
}
