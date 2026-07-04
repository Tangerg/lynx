package server

import (
	"context"
	"encoding/json"
	"iter"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	runstate "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/fspath"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// The run pump: one goroutine per run segment that subscribes to the
// turn's chat events, translates them to wire RunEvents, persists the
// durable side, and feeds the run's hub. Lifecycle bookkeeping
// (s.runs) starts in openSegment and ends in pumpRun's teardown.

// openSegment subscribes to the turn's event stream and starts the wire
// pump for one run segment. parentRunID is empty for a root run
// (runs.start) and set for a continuation (runs.resume) — it rides onto
// the RunRef and the active run record so the continuation links back to its
// parent.
func (s *Server) openSegment(reqCtx context.Context, runID, parentRunID string, handle turn.TurnHandle, sessionID string, userInput []protocol.ContentBlock, resume *resumeBinding, provider, model string) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
	// Detach the run from the request's cancellation (it must outlive the
	// request) WITHOUT losing the request's trace context: WithoutCancel
	// keeps ctx values — including the entry span — so the run's spans are
	// children of the same trace (full-link), while our own cancel drives
	// CancelRun. Rooting on context.Background() here would sever the trace.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(reqCtx))
	inner, err := s.rt.Chat().Events(runCtx, handle)
	if err != nil {
		cancel()
		_ = s.rt.Chat().Cancel(context.WithoutCancel(reqCtx), handle)
		return nil, nil, err
	}
	// The hub owns the run's event stream for its whole lifetime,
	// independent of any client connection (streamable HTTP, TRANSPORT
	// §6.4/§9.2). The pump appends to it; this caller streams from a
	// fresh subscription; a later runs.subscribe attaches another.
	hub := newRunHub()
	// The canonical working tree this run mutates — the key the cwd-aware busy
	// guard (a file rollback) uses to find sibling sessions sharing the tree.
	// Resolved here so the guard never does a session lookup under the registry
	// lock.
	cwd := fspath.Canonical(s.sessionCwd(reqCtx, sessionID))
	s.runs.Open(runstate.Record{
		ID:          runID,
		SessionID:   sessionID,
		Cwd:         cwd,
		CreatedAt:   time.Now().UTC(),
		TurnID:      handle.TurnID,
		ParentRunID: parentRunID,
		Provider:    provider,
		Model:       model,
	}, &runHandle{cancel: cancel, hub: hub})
	events, unsubscribe := hub.Subscribe("")
	// Drop this caller's subscription when its request ends (client
	// disconnect or stream completion) — the run keeps running on runCtx
	// and stays resumable via runs.subscribe. runCtx is rooted on
	// WithoutCancel(reqCtx) (see above), so it outlives the request that
	// started it without losing the trace.
	context.AfterFunc(reqCtx, unsubscribe)
	go s.pumpRun(runCtx, runID, parentRunID, handle, inner, hub, userInput, resume, provider, model)
	return &protocol.StartRunResponse{RunID: runID}, events, nil
}

// pumpRun translates the turn's chat events to RunEvents under runID.
// A run segment ends one of two ways:
//
//   - interrupt: the turn parked for HITL approval. The pump records an
//     open interrupt (so runs.listOpenInterrupts finds it) and returns,
//     leaving the live turn parked for runs.resume.
//   - terminal: the turn finished. The pump tears the run down.
//
// Either way the wire run.finished event is the last thing on the
// channel before it closes.
func (s *Server) pumpRun(ctx context.Context, runID, parentRunID string, handle turn.TurnHandle, inner iter.Seq[turn.Event], hub *runHub, userInput []protocol.ContentBlock, resume *resumeBinding, provider, model string) {
	tr := newTranslator(handle.SessionID, runID, parentRunID, userInput, resume, provider, model)
	finished := false
	parked := false
	// The session's cwd, resolved once: tool-derived files.changed events carry
	// it so a workspace subscriber can scope the (cwd-relative) paths.
	cwd := s.sessionCwd(ctx, handle.SessionID)
	effects := s.runSegmentEffects()
	openingText := ""
	if parentRunID == "" {
		openingText = userMessageText(userInput)
	}

	// emit assigns each StreamEvent its eventId, appends it to the hub (live
	// delivery + in-memory replay backlog), then hands the durable / workspace
	// side effects to kernel/runsegment. Append is non-blocking (it drops on a
	// slow subscriber, never stalls the run), and it runs BEFORE the durable
	// transcript write so a slow DB write (SQLite single-writer contention —
	// e.g. a concurrent compaction holding the connection) can't delay the
	// terminal run.finished reaching live subscribers or the pump teardown. Live
	// reconnect replays from the hub backlog, not the durable store; the store
	// feeds items.list + cross-restart only. Exception: the interrupt record is
	// written before the append because a client can resume the instant it sees
	// run.finished{interrupt}.
	emit := func(events []protocol.StreamEvent) {
		for _, se := range events {
			re := protocol.RunEvent{
				RunID:     runID,
				EventID:   s.nextEventID(),
				Timestamp: time.Now().UTC(),
				Event:     se,
			}
			if se.Type == protocol.StreamRunFinished {
				finished = true
				if se.Outcome != nil {
					switch se.Outcome.Type {
					case protocol.OutcomeInterrupt:
						parked = true
					case protocol.OutcomeCanceled:
						// Flow the runs.cancel reason to the outcome detail (S6)
						// so the client can tell user-canceled from other stops.
						if se.Outcome.Detail == "" {
							if r := s.cancelReasonFor(runID); r != "" {
								se.Outcome.Detail = r
							}
						}
					}
				}
			}
			side := s.sideEffectEvent(runID, handle.SessionID, parentRunID, cwd, se, provider, model)
			if se.Type == protocol.StreamRunFinished && se.Outcome != nil && se.Outcome.Type == protocol.OutcomeInterrupt {
				if raw, err := json.Marshal(se.Outcome.Interrupts); err == nil {
					side.Interrupt = &runsegment.Interrupt{
						RunID:        runID,
						Handle:       handle,
						Provider:     provider,
						Model:        model,
						Payload:      raw,
						DrainedTools: tr.parkDrained,
					}
				}
			}
			effects.BeforeLive(ctx, side)
			// Live first: deliver to subscribers + retain in the hub's in-memory
			// replay backlog. BeforeLive already ran, so a resume triggered by an
			// interrupt terminal event finds its record.
			hub.Append(re)
			// Persist off a cancel-decoupled ctx so the durable history (incl.
			// the terminal run.finished synthesized on a canceled run) lands
			// regardless of run-ctx cancellation — WithoutCancel keeps the
			// trace span (full-link), unlike context.Background(). After the
			// append so the DB never gates live delivery (see emit's doc).
			effects.AfterLive(context.WithoutCancel(ctx), side)
		}
	}

	// run.started leads every segment (root + continuation), independent of
	// any chat-level TurnStart — continuation runs (runs.resume) carry none,
	// so emitting here is what gives them a run boundary + parentRunId, and
	// closes any question item the parked run left open.
	emit(tr.open())

	defer func() {
		if !finished {
			// The turn ended without a run.finished (canceled mid-flight /
			// drained iterator) — synthesize the terminal so the stream ends
			// balanced.
			outcome := protocol.OutcomeCanceled
			if tr.errMsg != "" {
				outcome = protocol.OutcomeError
			}
			emit(tr.finish(outcome))
		}
		hub.Close()
		if e, ok := s.runs.Close(runID); ok {
			// A parked run keeps its live turn alive for resume — only
			// cancel + forget on a true terminal.
			if !parked && e.Payload != nil && e.Payload.cancel != nil {
				e.Payload.cancel()
			}
		}
		// Terminal maintenance stays off the run.finished path: async +
		// best-effort, so a slow snapshot or title LLM call never holds up live
		// delivery or teardown. A parked run is resumable, not a boundary.
		effects.Finish(ctx, runsegment.Finish{
			SessionID:       handle.SessionID,
			RunID:           runID,
			Parked:          parked,
			OpeningUserText: openingText,
		})
	}()

	for ev := range inner {
		emit(tr.translate(ev))
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
	}
}
