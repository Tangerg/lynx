package server

import (
	"context"
	"errors"
	"iter"
	"strconv"
	"strings"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/chat"
	"github.com/Tangerg/lynx/lyra/internal/service/session"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// StartRun translates runs.start into the in-process chat.StartTurn
// path (API.md §7.3). It returns the runId synchronously; the run's
// events flow out via the returned channel as RunEvents, wrapped by the
// transport into notifications.run.event. The terminal run.finished
// event rides this same channel — there is no separate close signal.
func (i *Server) StartRun(ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	sessionID, err := i.resolveSession(ctx, in.SessionID)
	if err != nil {
		return nil, nil, err
	}

	userMsg := lastUserText(in.Input)
	if userMsg == "" {
		return nil, nil, errors.New("runs.start: input must contain a user text block")
	}

	handle, err := i.rt.Chat().StartTurn(ctx, chat.StartTurnRequest{
		SessionID:  sessionID,
		Message:    userMsg,
		MaxCostUSD: in.MaxBudgetUSD, // maxBudgetUsd → turn dollar cap (API.md §7.1)
	})
	if err != nil {
		return nil, nil, err
	}

	// runCtx outlives the StartRun request ctx — it drives the pump and
	// bounds the event iterator, canceled by runs.cancel or turn end.
	runCtx, cancel := context.WithCancel(context.Background())
	inner, err := i.rt.Chat().Events(runCtx, handle)
	if err != nil {
		cancel()
		return nil, nil, err
	}

	// runId on the wire == the turn id Lyra assigns internally.
	runID := handle.TurnID
	out := &protocol.StartRunResponse{RunID: runID}
	events := make(chan protocol.RunEvent, 32)

	i.runMu.Lock()
	i.runs[runID] = &runEntry{runID: runID, sessionID: sessionID, turnID: handle.TurnID, cancel: cancel}
	i.runMu.Unlock()

	go i.pumpRun(runCtx, handle, inner, events)
	return out, events, nil
}

// pumpRun translates internal chat events to RunEvents and pipes them
// to the consumer. Exits when the inner stream closes (turn end) or the
// run is canceled; guarantees a terminal run.finished even when the
// turn never delivered one (cancellation drained the iterator early).
func (i *Server) pumpRun(ctx context.Context, handle chat.TurnHandle, inner iter.Seq[chat.Event], out chan<- protocol.RunEvent) {
	tr := newTranslator(handle.SessionID, handle.TurnID)
	var evtSeq int
	finished := false

	send := func(events []protocol.StreamEvent) bool {
		for _, se := range events {
			evtSeq++
			re := protocol.RunEvent{
				RunID:     handle.TurnID,
				EventID:   protocol.IDPrefixEvent + handle.TurnID + "_" + strconv.Itoa(evtSeq),
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
				Durable:   durableFor(se.Type),
				Event:     se,
			}
			if se.Type == protocol.StreamRunFinished {
				finished = true
			}
			select {
			case out <- re:
			case <-ctx.Done():
				return false
			}
		}
		return true
	}

	defer func() {
		// Synthesize a terminal run.finished if the stream stopped before
		// the turn delivered TurnEnd (e.g. the run was canceled).
		if !finished {
			outcome := protocol.OutcomeCanceled
			if tr.errMsg != "" {
				outcome = protocol.OutcomeError
			}
			_ = sendTerminal(out, handle.TurnID, &evtSeq, tr.finish(outcome))
		}
		close(out)
		i.runMu.Lock()
		if e, ok := i.runs[handle.TurnID]; ok {
			e.cancel()
			delete(i.runs, handle.TurnID)
		}
		i.runMu.Unlock()
	}()

	for ev := range inner {
		if !send(tr.translate(ev)) {
			// ctx canceled mid-send: best-effort cancel the turn, then let
			// the deferred terminal close the stream.
			_ = i.rt.Chat().Cancel(context.Background(), handle)
			return
		}
	}
	if ctx.Err() != nil {
		_ = i.rt.Chat().Cancel(context.Background(), handle)
	}
}

// sendTerminal pushes the deferred terminal events without the ctx
// guard (ctx is already done in the cancellation path) so the stream
// always ends with run.finished. Best-effort: a full buffer drops it.
func sendTerminal(out chan<- protocol.RunEvent, runID string, evtSeq *int, events []protocol.StreamEvent) error {
	for _, se := range events {
		*evtSeq++
		re := protocol.RunEvent{
			RunID:     runID,
			EventID:   protocol.IDPrefixEvent + runID + "_" + strconv.Itoa(*evtSeq),
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Durable:   durableFor(se.Type),
			Event:     se,
		}
		select {
		case out <- re:
		default:
			return errors.New("run event buffer full")
		}
	}
	return nil
}

// durableFor classifies a stream event's durability (API.md §5.3):
// run.* / item.started / item.completed / state.snapshot are
// authoritative; item.delta / state.delta are ephemeral.
func durableFor(t protocol.StreamEventType) bool {
	switch t {
	case protocol.StreamItemDelta, protocol.StreamStateDelta:
		return false
	default:
		return true
	}
}

// CancelRun hard-stops a running run (outcome:canceled, API.md §7.3).
func (i *Server) CancelRun(_ context.Context, in protocol.CancelRunRequest) error {
	i.runMu.Lock()
	e, ok := i.runs[in.RunID]
	i.runMu.Unlock()
	if !ok {
		return protocol.ErrRunNotFound
	}
	e.cancel()
	return nil
}

// ListRuns returns the currently running runs (API.md §7.3).
func (i *Server) ListRuns(_ context.Context, in protocol.ListRunsRequest) ([]protocol.RunRef, error) {
	i.runMu.Lock()
	defer i.runMu.Unlock()
	out := make([]protocol.RunRef, 0, len(i.runs))
	for _, e := range i.runs {
		if in.SessionID != "" && e.sessionID != in.SessionID {
			continue
		}
		out = append(out, protocol.RunRef{
			ID:        e.runID,
			SessionID: e.sessionID,
			Status:    protocol.RunStatusRunning,
		})
	}
	return out, nil
}

// ResumeRun answers open interrupts via a continuation run (R model).
// HITL is not yet surfaced on the wire (no engine interrupt source);
// gated off in capabilities until the P→R refactor lands.
func (i *Server) ResumeRun(_ context.Context, _ protocol.ResumeRunRequest) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	return nil, nil, notImpl("runs.resume")
}

// SubscribeRun rebinds an existing run's stream to the caller. Stream
// fan-out / replay isn't wired yet (single-consumer in-process streams).
func (i *Server) SubscribeRun(_ context.Context, _ string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	return nil, nil, notImpl("runs.subscribe")
}

// ListOpenInterrupts returns durable resumable interrupts. None exist
// until the HITL R refactor persists them.
func (i *Server) ListOpenInterrupts(_ context.Context, _ protocol.ListOpenInterruptsRequest) ([]protocol.OpenInterrupt, error) {
	return []protocol.OpenInterrupt{}, nil
}

// ─── helpers ────────────────────────────────────────────────────────

// resolveSession verifies sessionID exists, or creates a fresh session
// when empty (zero-friction cold start for in-process callers).
func (i *Server) resolveSession(ctx context.Context, sessionID string) (string, error) {
	if sessionID == "" {
		sess, err := i.rt.Session().Create(ctx, "")
		if err != nil {
			return "", err
		}
		return sess.ID, nil
	}
	if _, err := i.rt.Session().Get(ctx, sessionID); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return "", protocol.ErrSessionNotFound
		}
		return "", err
	}
	return sessionID, nil
}

// lastUserText joins the text blocks of a run's input into the single
// user message the in-process chat.StartTurn API expects today. Image
// blocks (attachmentId) are ignored until multimodal lands.
func lastUserText(blocks []protocol.ContentBlock) string {
	var b []string
	for _, blk := range blocks {
		if blk.Type == "text" && blk.Text != "" {
			b = append(b, blk.Text)
		}
	}
	return strings.Join(b, "\n")
}
