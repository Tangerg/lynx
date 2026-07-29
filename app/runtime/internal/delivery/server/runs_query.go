package server

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// GetRun returns one run by id, in whatever state the durable record has it. The
// caller supplies no session: a runId already identifies one run, and requiring
// the session would mean knowing where a run lives before being able to ask what
// it is.
func (s *Server) GetRun(ctx context.Context, in protocol.GetRunRequest) (*protocol.RunRef, error) {
	run, found, err := s.queries.Run(ctx, in.RunID)
	switch {
	case err != nil:
		return nil, err
	case !found:
		return nil, protocol.ErrRunNotFound
	}
	ref := presentRun(run)
	return &ref, nil
}

// ListRuns pages the durable run history as a cursor Page. The set and the scope
// come from the durable admission record, not from this process's live registry:
// the registry sees only the segments it is streaming, so it lost every run whose
// process restarted and never held the ones a person is being asked to approve.
// A request asking for descendants never reaches here: the method's capability
// rule refuses it while features.subagents is off, which is why this reads
// IncludeDescendants nowhere — re-checking it would be a second author of the
// registered rule.
func (s *Server) ListRuns(ctx context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
	statuses, err := runStatusesFromWire(in.Statuses)
	if err != nil {
		return nil, err
	}
	page, err := s.queries.ListRunPage(ctx, in.SessionID, statuses, in.Cursor, in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	out := make([]protocol.RunRef, 0, len(page.Rows))
	for _, run := range page.Rows {
		out = append(out, presentRun(run))
	}
	return protocol.NewPageWithCursor(out, page.NextCursor), nil
}

// runStatusesFromWire reads a status filter into the lifecycle positions the
// durable record is keyed by. It is total over what the wire can carry: Go's
// decoder puts any string into a named string type, so a value outside the enum
// arrives here and must be refused rather than dropped — a dropped filter value
// silently widens the page.
func runStatusesFromWire(statuses []protocol.RunStatus) ([]execution.RunStatus, error) {
	out := make([]execution.RunStatus, 0, len(statuses))
	for _, status := range statuses {
		switch status {
		case protocol.RunStatusRunning:
			out = append(out, execution.StatusRunning)
		case protocol.RunStatusWaiting:
			out = append(out, execution.StatusWaiting)
		case protocol.RunStatusFinished:
			out = append(out, execution.StatusFinished)
		default:
			return nil, fmt.Errorf("%w: unknown statuses value %q", protocol.ErrInvalidParams, status)
		}
	}
	return out, nil
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
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	record, events, err := s.coordinator.SubscribeLive(ctx, runID, fromCursor, caller)
	switch {
	case errors.Is(err, runs.ErrProfileNotCovered):
		return nil, nil, fmt.Errorf("%w: %w", protocol.ErrCapabilityNotNeg, err)
	case err != nil:
		return nil, nil, protocol.ErrRunNotFound
	}
	return &protocol.StartRunResponse{RunID: runID, SegmentID: record.SegmentID}, mapRunEvents(ctx, events), nil
}
