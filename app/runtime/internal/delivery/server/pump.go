package server

import (
	"context"
	"encoding/json"
	"iter"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	runstate "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/fspath"
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

	// emit assigns each StreamEvent its eventId, appends it to the hub (live
	// delivery + in-memory replay backlog), then persists the durable side to
	// history. Append is non-blocking (it drops on a slow subscriber, never
	// stalls the run), and it runs BEFORE the durable persist so a slow DB write
	// (SQLite single-writer contention — e.g. a concurrent compaction holding the
	// connection) can't delay the terminal run.finished reaching live subscribers
	// or the pump teardown. Live reconnect replays from the hub backlog, not the
	// persist; the persist feeds items.list + cross-restart only. Exception: the
	// interrupt record is written before the append (see below) because a client
	// can resume the instant it sees run.finished{interrupt}.
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
						s.recordInterrupt(ctx, runID, handle, se.Outcome.Interrupts, tr.parkDrained)
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
			// Live first: deliver to subscribers + retain in the hub's in-memory
			// replay backlog. recordInterrupt (the interrupt branch above) already
			// ran, so a resume triggered by this event finds its record.
			hub.Append(re)
			// Persist off a cancel-decoupled ctx so the durable history (incl.
			// the terminal run.finished synthesized on a canceled run) lands
			// regardless of run-ctx cancellation — WithoutCancel keeps the
			// trace span (full-link), unlike context.Background(). After the
			// append so the DB never gates live delivery (see emit's doc).
			s.persistStreamEvent(context.WithoutCancel(ctx), runID, handle.SessionID, parentRunID, se, provider, model)
			// Tell workspace subscribers a file changed when an agent file tool
			// completes — precise + fd-free, so the watcher needn't watch the tree.
			s.emitToolFileChange(cwd, se)
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
		// Anchor the file checkpoint for this run boundary AFTER teardown and
		// OFF the run.finished path: async + best-effort, so a slow snapshot
		// (or none, for a non-git dir — gated in workspace.Snapshot) never
		// holds up the terminal event or the run's teardown. Only on a true
		// terminal (a parked run is resumable, not a boundary). WithoutCancel
		// detaches from the run ctx the cancel() above just fired; the Store
		// serializes per session so it can't race the next run's snapshot.
		if !parked {
			go s.snapshotCheckpoint(context.WithoutCancel(ctx), handle.SessionID, runID)
			// Auto-name an untitled session from its first user message — async +
			// best-effort off the terminal path, same discipline as the snapshot
			// above (an LLM call must never hold up the run's teardown).
			go s.maybeTitleSession(context.WithoutCancel(ctx), handle.SessionID, parentRunID, userInput)
		}
	}()

	for ev := range inner {
		emit(tr.translate(ev))
		if parked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
	}
}

// recordInterrupt persists the open interrupt so runs.listOpenInterrupts
// can discover it and runs.resume can map it back to the live turn.
// drained is the backend-private snapshot of tool items open at park
// time — see [interrupts.Pending.DrainedTools].
func (s *Server) recordInterrupt(ctx context.Context, runID string, handle turn.TurnHandle, ints []protocol.Interrupt, drained []interrupts.DrainedTool) {
	raw, err := json.Marshal(ints)
	if err != nil {
		return
	}
	// Best-effort: persist the interrupt record so the run can be
	// resumed across restarts. If Put fails, same-process resume still
	// works (the process is parked in-memory), but cross-restart resume
	// will not find this interrupt — so record both errors on the run's span
	// (ctx keeps it, full-link) rather than dropping them silently, mirroring
	// persistItem / persistRun. A missing ProcessID also makes a later
	// rehydrate fail far from here, so it's worth the breadcrumb too.
	processID, perr := s.rt.Chat().ProcessID(ctx, handle)
	if perr != nil {
		trace.SpanFromContext(ctx).RecordError(perr)
	}
	// Carry the run's per-run model selection onto the interrupt so a
	// cross-restart rehydrate rebuilds the SAME model client (the live process
	// holds it in memory; the persisted record is the only place it survives a
	// restart).
	var provider, model string
	if e, ok := s.runs.Get(runID); ok {
		provider, model = e.Record.Provider, e.Record.Model
	}
	if err := s.rt.Interrupts().Put(ctx, interrupts.Pending{
		ParentRunID:  runID,
		SessionID:    handle.SessionID,
		TurnID:       handle.TurnID,
		ProcessID:    processID,
		Provider:     provider,
		Model:        model,
		Interrupts:   raw,
		DrainedTools: drained,
		CreatedAt:    time.Now().UTC(),
	}); err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}
