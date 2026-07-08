package server

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ListRuns returns the currently running runs as a Page (API.md §7.3).
// The set is in-process and bounded, so the page carries no cursor.
func (s *Server) ListRuns(_ context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	entries := s.runs.List()
	out := make([]protocol.RunRef, 0, len(entries))
	for _, e := range entries {
		r := e.Record
		if in.SessionID != "" && r.SessionID != in.SessionID {
			continue
		}
		out = append(out, protocol.RunRef{
			ID:          r.ID,
			SessionID:   r.SessionID,
			ParentRunID: r.ParentRunID,
			Provider:    r.Provider,
			Model:       r.Model,
			Status:      protocol.RunStatusRunning,
		})
	}
	return protocol.NewPage(out), nil
}

// ListOpenInterrupts returns durable resumable interrupts as a Page
// (API.md §6.2).
func (s *Server) ListOpenInterrupts(ctx context.Context, in protocol.ListOpenInterruptsRequest) (*protocol.Page[protocol.OpenInterrupt], error) {
	pending, err := s.rt.ListPendingInterrupts(ctx, in.SessionID)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.OpenInterrupt, 0, len(pending))
	for _, p := range pending {
		var ints []protocol.Interrupt
		if err := json.Unmarshal(p.Interrupts, &ints); err != nil {
			// Corrupted interrupt record — skip it rather than
			// surfacing a bogus entry with zero interrupts.
			continue
		}
		out = append(out, protocol.OpenInterrupt{
			ParentRunID: p.ParentRunID,
			SessionID:   p.SessionID,
			Interrupts:  ints,
			CreatedAt:   p.CreatedAt,
		})
	}
	return protocol.NewPage(out), nil
}

// SubscribeRun opens a fresh event stream onto an actively-streaming root
// run (reconnect / crash recovery; subscribes the whole run tree, API.md
// §5.4 / §7.3). It attaches a new subscriber to the run's hub, replaying
// the durable backlog after the caller's Last-Event-Id (carried out-of-band
// via ctx, TRANSPORT §9.2) then tailing live. A run that isn't actively
// streaming (finished / parked / unknown) returns run_not_found — its tail
// is recovered through items.list, not here.
func (s *Server) SubscribeRun(ctx context.Context, runID string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	if runID == "" {
		return nil, nil, protocol.ErrRunNotFound
	}
	e, live := s.runs.Get(runID)
	if !live || e.Payload == nil || e.Payload.hub == nil {
		return nil, nil, protocol.ErrRunNotFound
	}
	events, unsubscribe := e.Payload.hub.Subscribe(transport.LastEventIDFrom(ctx))
	// Drop the subscription when this request ends; the run continues.
	context.AfterFunc(ctx, unsubscribe)
	return &protocol.StartRunResponse{RunID: runID}, events, nil
}
