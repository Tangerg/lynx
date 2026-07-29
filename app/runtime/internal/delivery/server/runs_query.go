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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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

// ListInterrupts pages the durable waiting sets — what a person still has to
// answer — longest wait first.
//
// The caller's declared capabilities are part of the read: a set belonging to a run
// that publishes more than this caller can follow is refused, never trimmed. A
// trimmed set would be answered, consumed as if complete, and leave the run waiting
// on interrupts the client believes it resolved.
func (s *Server) ListInterrupts(ctx context.Context, in protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	page, err := s.queries.ListPendingInterruptPage(ctx, in.SessionID, in.RootRunID, caller, in.Cursor, in.Limit)
	if err != nil {
		return nil, wireInterruptPageError(wirePageError(err))
	}
	out := make([]protocol.PendingInterruptSet, 0, len(page.Rows))
	for _, pending := range page.Rows {
		out = append(out, protocol.PendingInterruptSet{
			RootRunID:  pending.RootRunID,
			SessionID:  pending.SessionID,
			Interrupts: presentInterrupts(pending.Interrupts),
			CreatedAt:  pending.CreatedAt,
		})
	}
	return protocol.NewPageWithCursor(out, page.NextCursor), nil
}

// wireInterruptPageError maps the read's two refusals. Both name something the
// caller can act on: declare the capabilities, or ask under the root.
func wireInterruptPageError(err error) error {
	if uncovered, ok := errors.AsType[*execution.ProfileNotCovered](err); ok {
		return profileGap(uncovered.Gap)
	}
	switch {
	case errors.Is(err, transcript.ErrNotRoot):
		return fmt.Errorf("%w: %w", protocol.ErrRunNotRoot, err)
	default:
		return err
	}
}

// SubscribeRun opens a fresh event stream onto the root segment the request
// names (reconnect / crash recovery; subscribes the whole run tree, API.md
// §5.4 / §7.3).
//
// With a Last-Event-Id (carried out-of-band via ctx, TRANSPORT §9.2) it replays
// the retained events after that position and then tails live; without one it
// attaches at the current head and returns it, so a client can read the durable
// state afterwards and fold this stream on top without a gap. History is NOT
// replayed for a cursorless subscribe — that is what items.list answers.
func (s *Server) SubscribeRun(ctx context.Context, in protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error) {
	caller, err := s.negotiateCapabilities(ctx)
	if err != nil {
		return nil, nil, err
	}
	attached, err := s.coordinator.Subscribe(ctx, runs.SubscribeRequest{
		RunID:     in.RunID,
		SegmentID: in.SegmentID,
		// The application's cursor is prefix-free; the evt_ framing is this layer's
		// (§11.2). TrimPrefix leaves an absent id untouched, which is the tail-only case.
		Cursor: strings.TrimPrefix(transport.LastEventIDFrom(ctx), protocol.IDPrefixEvent),
		Caller: caller,
	})
	if err != nil {
		return nil, nil, wireLiveSegmentError(err)
	}
	head := ""
	if attached.HeadCursor != "" {
		head = protocol.IDPrefixEvent + attached.HeadCursor
	}
	return &protocol.SubscribeRunResponse{
		RunID: in.RunID, SegmentID: attached.Record.SegmentID, HeadEventID: head,
	}, mapRunEvents(ctx, attached.Events), nil
}

// wireLiveSegmentError maps the refusals of addressing a live segment. Each one
// names something the caller can do instead, which is why they are not collapsed
// into run_not_found: "the run is waiting", "the run finished", "you are holding
// the wrong segment" and "your cursor is too old" have four different remedies.
func wireLiveSegmentError(err error) error {
	if uncovered, ok := errors.AsType[*execution.ProfileNotCovered](err); ok {
		return profileGap(uncovered.Gap)
	}
	switch {
	case errors.Is(err, runs.ErrRunNotFound):
		return protocol.ErrRunNotFound
	case errors.Is(err, transcript.ErrNotRoot):
		return fmt.Errorf("%w: %w", protocol.ErrRunNotRoot, err)
	case errors.Is(err, runs.ErrRunWaiting):
		return fmt.Errorf("%w: %w", protocol.ErrRunWaiting, err)
	case errors.Is(err, runs.ErrRunFinished):
		return fmt.Errorf("%w: %w", protocol.ErrRunFinished, err)
	case errors.Is(err, runs.ErrStaleSegment):
		return fmt.Errorf("%w: %w", protocol.ErrStaleSegment, err)
	case errors.Is(err, runs.ErrReplayCursorInvalid):
		return fmt.Errorf("%w: %w", protocol.ErrReplayCursorInvalid, err)
	case errors.Is(err, runs.ErrReplayUnavailable):
		return fmt.Errorf("%w: %w", protocol.ErrReplayUnavailable, err)
	default:
		return err
	}
}
