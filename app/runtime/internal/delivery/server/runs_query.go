package server

import (
	"context"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// ListRuns returns the currently running runs as a cursor Page (API.md §7.3).
// The set and the session scope come from the durable admission record, not from
// this process's live registry: the registry sees only the segments it is
// streaming, so it lost every run whose process restarted and never held the
// ones a person is being asked to approve.
func (s *Server) ListRuns(ctx context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	page, err := s.queries.ListRunningRuns(ctx, in.SessionID, in.Cursor, in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	out := make([]protocol.RunRef, 0, len(page.Rows))
	for _, run := range page.Rows {
		out = append(out, presentRun(run))
	}
	return protocol.NewPageWithCursor(out, page.NextCursor), nil
}

// ListOpenInterrupts returns durable resumable interrupts as a Page
// (API.md §6.2).
func (s *Server) ListOpenInterrupts(ctx context.Context, in protocol.ListOpenInterruptsRequest) (*protocol.Page[protocol.OpenInterrupt], error) {
	page, err := s.queries.ListPendingInterruptPage(ctx, in.SessionID, in.Cursor, in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	out := make([]protocol.OpenInterrupt, 0, len(page.Rows))
	for _, pending := range page.Rows {
		out = append(out, protocol.OpenInterrupt{
			RunID:      pending.RunID,
			SessionID:  pending.SessionID,
			Interrupts: presentInterrupts(pending.Interrupts),
			CreatedAt:  pending.CreatedAt,
		})
	}
	return protocol.NewPageWithCursor(out, page.NextCursor), nil
}

// SubscribeRun opens a fresh event stream onto an actively-streaming root
// run (reconnect / crash recovery; subscribes the whole run tree, API.md
// §5.4 / §7.3). It attaches a new subscriber to the run's hub, replaying
// the durable backlog after the caller's Last-Event-Id (carried out-of-band
// via ctx, TRANSPORT §9.2) then tailing live. A run that isn't actively
// streaming (finished / parked / unknown) returns run_not_found — its tail
// is recovered through items.list, not here.
func (s *Server) SubscribeRun(ctx context.Context, runID string) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
	if runID == "" {
		return nil, nil, protocol.ErrRunNotFound
	}
	// The Journal replays after an opaque, prefix-free application cursor; strip
	// the evt_ wire framing off the client's Last-Event-Id (§11.2). TrimPrefix
	// leaves an empty / unframed id untouched, so replay-from-start still works.
	fromCursor := strings.TrimPrefix(transport.LastEventIDFrom(ctx), protocol.IDPrefixEvent)
	record, events, ok := s.coordinator.SubscribeLive(ctx, runID, fromCursor)
	if !ok {
		return nil, nil, protocol.ErrRunNotFound
	}
	return &protocol.StartRunResponse{RunID: runID, SegmentID: record.SegmentID}, mapRunEvents(ctx, events), nil
}
