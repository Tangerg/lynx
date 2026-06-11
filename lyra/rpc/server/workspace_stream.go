package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// workspaceHub fans workspace events out to the live workspace.subscribe
// streams (AUX_API §3). It is the non-run, ephemeral counterpart to the
// per-run hubs: lossy (a slow subscriber drops the event rather than
// back-pressuring the publisher — workspace events are "changed → re-fetch",
// so a drop self-heals on the next change or a resync), connection-scoped (no
// durable replay), shared by the whole app.
type workspaceHub struct {
	mu   sync.Mutex
	subs map[chan protocol.WorkspaceEvent]struct{}
}

func newWorkspaceHub() *workspaceHub {
	return &workspaceHub{subs: map[chan protocol.WorkspaceEvent]struct{}{}}
}

// subscribe registers a new subscriber and returns its channel plus an
// idempotent unsubscribe (closes the channel, drops it from the fan-out).
func (h *workspaceHub) subscribe() (<-chan protocol.WorkspaceEvent, func()) {
	ch := make(chan protocol.WorkspaceEvent, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// publish fans ev to every subscriber, dropping it for any whose buffer is
// full (lossy by design — see the type doc).
func (h *workspaceHub) publish(ev protocol.WorkspaceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- ev:
		default: // full subscriber: drop (it re-fetches on the next change / resync)
		}
	}
}

// WorkspaceSubscribe opens the workspace event stream (AUX_API §3.1). The
// stream's lifetime is the subscription; it ends when the request ctx does
// (client disconnect). File watching (the watches param) is gated behind
// features.fileWatch — until that lands, a non-empty watches set is rejected
// rather than silently ignored; a watch-less subscribe (skills / mcp events)
// is always available.
func (s *Server) WorkspaceSubscribe(ctx context.Context, in protocol.WorkspaceSubscribeRequest) (*protocol.WorkspaceSubscribeResponse, <-chan protocol.WorkspaceEvent, error) {
	if len(in.Watches) > 0 {
		return nil, nil, fmt.Errorf("%w: file watching (watches) not available on this build", protocol.ErrCapabilityNotNeg)
	}
	events, unsubscribe := s.wsHub.subscribe()
	context.AfterFunc(ctx, unsubscribe)
	return &protocol.WorkspaceSubscribeResponse{}, events, nil
}

// PublishWorkspaceEvent fans one workspace event out to subscribers. The
// runtime / engine call this when a non-run state change happens (mcp
// serverChanged, skills.changed, files.changed). Safe to call with no
// subscribers (no-op).
func (s *Server) PublishWorkspaceEvent(ev protocol.WorkspaceEvent) {
	if s.wsHub != nil {
		s.wsHub.publish(ev)
	}
}
