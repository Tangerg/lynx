package server

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

// The run pump: one goroutine per run segment that subscribes to the
// turn's events, translates them to wire RunEvents, persists the
// durable side, and feeds the run's hub. Lifecycle bookkeeping
// (s.runs) starts in openSegment and ends in pumpRun's teardown.

const runCleanupTimeout = 5 * time.Second

func (s *Server) cancelTurnAfterAdmissionFailure(ctx context.Context, handle turn.TurnHandle) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	return s.rt.CancelTurn(cleanupCtx, handle)
}

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
	taskCtx, release, ok := s.tasks.Attach(reqCtx)
	if !ok {
		_ = s.cancelTurnAfterAdmissionFailure(reqCtx, handle)
		return nil, nil, errServerClosed
	}
	runCtx, cancel := context.WithCancel(taskCtx)
	inner, err := s.rt.TurnEvents(runCtx, handle)
	if err != nil {
		cancel()
		_ = s.rt.CancelTurn(taskCtx, handle)
		release()
		return nil, nil, err
	}
	// The hub owns the run's event stream for its whole lifetime,
	// independent of any client connection (streamable HTTP, TRANSPORT
	// §6.4/§9.2). The pump appends to it; this caller streams from a
	// fresh subscription; a later runs.subscribe attaches another.
	hub := runs.NewJournal[protocol.RunEvent]()
	// The canonical working tree this run mutates — the key the cwd-aware busy
	// guard (a file rollback) uses to find sibling sessions sharing the tree.
	// Resolved here so the guard never does a session lookup under the registry
	// lock.
	cwd := worktree.CanonicalCwd(s.sessionCwd(reqCtx, sessionID))
	live := &runHandle{cancel: cancel, owner: taskCtx, hub: hub}
	s.runs.Open(runs.Record{
		ID:          runID,
		SessionID:   sessionID,
		Cwd:         cwd,
		CreatedAt:   time.Now().UTC(),
		TurnID:      handle.TurnID,
		ParentRunID: parentRunID,
		Provider:    provider,
		Model:       model,
	}, live)
	events, unsubscribe := hub.Subscribe("")
	// Drop this caller's subscription when its request ends (client
	// disconnect or stream completion) — the run keeps running on runCtx
	// and stays resumable via runs.subscribe. runCtx is rooted on
	// WithoutCancel(reqCtx) (see above), so it outlives the request that
	// started it without losing the trace.
	context.AfterFunc(reqCtx, unsubscribe)
	go func() {
		defer release()
		s.pumpRun(runCtx, taskCtx, runID, parentRunID, handle, inner, live, userInput, resume, provider, model)
	}()
	return &protocol.StartRunResponse{RunID: runID}, events, nil
}

// pumpRun translates the turn's events to RunEvents under runID.
// A run segment ends one of two ways:
//
//   - interrupt: the turn parked for HITL approval. The pump records an
//     open interrupt (so runs.listOpenInterrupts finds it) and returns,
//     leaving the live turn parked for runs.resume.
//   - terminal: the turn finished. The pump tears the run down.
//
// Either way the wire run.finished event is the last thing on the
// channel before it closes.
func (s *Server) pumpRun(ctx, ownerCtx context.Context, runID, parentRunID string, handle turn.TurnHandle, inner iter.Seq[turn.Event], live *runHandle, userInput []protocol.ContentBlock, resume *resumeBinding, provider, model string) {
	hub := live.hub
	tr := newTranslator(handle.SessionID, runID, parentRunID, userInput, resume, provider, model)
	finished := false
	parked := false
	abortTurn := false
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
	emit := func(events []protocol.StreamEvent) bool {
		for _, se := range events {
			terminal := se.Type == protocol.StreamRunFinished
			interrupt := terminal && se.Outcome != nil && se.Outcome.Type == protocol.OutcomeInterrupt
			if terminal && !interrupt {
				finished = true
				if se.Outcome != nil && se.Outcome.Type == protocol.OutcomeCanceled {
					// Flow the runs.cancel reason to the outcome detail (S6)
					// so the client can tell user-canceled from other stops.
					if se.Outcome.Detail == "" {
						if r := live.reason(); r != "" {
							se.Outcome.Detail = r
						}
					}
				}
			}
			re := protocol.RunEvent{
				RunID:     runID,
				EventID:   s.nextEventID(),
				Timestamp: time.Now().UTC(),
				Event:     se,
			}
			side := s.sideEffectEvent(runID, handle.SessionID, parentRunID, cwd, se, provider, model)
			if interrupt {
				raw, err := json.Marshal(se.Outcome.Interrupts)
				if err != nil {
					tr.errMsg = fmt.Sprintf("persist interrupt payload: %v", err)
					abortTurn = true
					return false
				}
				side.Interrupt = &runsegment.Interrupt{
					RunID:        runID,
					Handle:       handle,
					Provider:     provider,
					Model:        model,
					Payload:      raw,
					DrainedTools: tr.parkDrained,
				}
				committed, err := live.commitInterrupt(func() error {
					if err := effects.BeforeLive(ownerCtx, side); err != nil {
						return err
					}
					finished = true
					parked = true
					hub.Append(re)
					return nil
				})
				if err != nil {
					if ctx.Err() == nil && ownerCtx.Err() == nil {
						tr.errMsg = err.Error()
					}
					abortTurn = true
					return false
				}
				if !committed {
					return false
				}
				effects.AfterLive(ownerCtx, side)
				continue
			}
			// Live first: deliver to subscribers + retain in the hub's in-memory
			// replay backlog. Interrupt terminals take the coordinated path above,
			// where their durable record is committed before publication.
			hub.Append(re)
			// ownerCtx survives runs.cancel but is canceled by Server.Close. This
			// preserves terminal history without allowing a stuck store to outlive
			// the component that owns the pump.
			effects.AfterLive(ownerCtx, side)
		}
		return true
	}

	// run.started leads every segment (root + continuation), independent of
	// any turn-level TurnStart — continuation runs (runs.resume) carry none,
	// so emitting here is what gives them a run boundary + parentRunId, and
	// closes any question item the parked run left open.
	if !emit(tr.open()) {
		return
	}

	defer func() {
		if ctx.Err() != nil || abortTurn {
			_ = s.rt.CancelTurn(ownerCtx, handle)
		}
		if !finished {
			// The turn ended without a run.finished (canceled mid-flight /
			// drained iterator) — synthesize the terminal so the stream ends
			// balanced.
			outcome := protocol.OutcomeCanceled
			if tr.errMsg != "" {
				outcome = protocol.OutcomeError
			}
			_ = emit(tr.finish(outcome))
		}
		hub.Close()
		if e, ok := s.runs.Close(runID); ok {
			// A parked run keeps its live turn alive for resume — only
			// cancel + forget on a true terminal.
			if !parked && e.Payload != nil {
				e.Payload.stop()
			}
		}
		// Terminal maintenance stays off the run.finished path: async +
		// best-effort, so a slow snapshot or title LLM call never holds up live
		// delivery or teardown. A parked run is resumable, not a boundary.
		effects.Finish(ownerCtx, runsegment.Finish{
			SessionID:       handle.SessionID,
			RunID:           runID,
			Parked:          parked,
			OpeningUserText: openingText,
		})
	}()

	for ev := range inner {
		if !emit(tr.translate(ev)) {
			return
		}
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
	}
}
