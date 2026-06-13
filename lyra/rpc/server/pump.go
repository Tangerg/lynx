package server

import (
	"context"
	"encoding/json"
	"iter"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine/chat"
	"github.com/Tangerg/lynx/lyra/internal/domain/interrupts"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// The run pump: one goroutine per run segment that subscribes to the
// turn's chat events, translates them to wire RunEvents, persists the
// durable side, and feeds the run's hub. Lifecycle bookkeeping
// (s.runs + runMu) starts in openSegment and ends in pumpRun's
// teardown.

// openSegment subscribes to the turn's event stream and starts the wire
// pump for one run segment. parentRunID is empty for a root run
// (runs.start) and set for a continuation (runs.resume) — it rides onto
// the RunRef and the runEntry so the continuation links back to its parent.
func (s *Server) openSegment(reqCtx context.Context, runID, parentRunID string, handle chat.TurnHandle, sessionID string, userInput []protocol.ContentBlock, resume *resumeBinding) (*protocol.StartRunResponse, <-chan protocol.RunEvent, error) {
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
	s.runMu.Lock()
	s.runs[runID] = &runEntry{runID: runID, sessionID: sessionID, turnID: handle.TurnID, parentRunID: parentRunID, cancel: cancel, hub: hub}
	s.runMu.Unlock()
	events, unsubscribe := hub.Subscribe("")
	// Drop this caller's subscription when its request ends (client
	// disconnect or stream completion) — the run keeps running on runCtx
	// and stays resumable via runs.subscribe. runCtx is Background-rooted
	// on purpose: the run must outlive the request that started it.
	context.AfterFunc(reqCtx, unsubscribe)
	go s.pumpRun(runCtx, runID, parentRunID, handle, inner, hub, userInput, resume)
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
func (s *Server) pumpRun(ctx context.Context, runID, parentRunID string, handle chat.TurnHandle, inner iter.Seq[chat.Event], hub *runHub, userInput []protocol.ContentBlock, resume *resumeBinding) {
	tr := newTranslator(handle.SessionID, runID, parentRunID, userInput, resume)
	finished := false
	parked := false
	// The session's cwd, resolved once: tool-derived files.changed events carry
	// it so a workspace subscriber can scope the (cwd-relative) paths.
	cwd := s.sessionCwd(ctx, handle.SessionID)

	// emit assigns each StreamEvent its eventId, persists the durable
	// side to history, and appends to the hub. hub.Append is non-blocking
	// (it drops on a slow subscriber, never stalls the run), so unlike the
	// old per-connection channel there is no client-disconnect backpressure
	// here — the run streams to the hub regardless of who is listening.
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
			// Persist with a background ctx so the durable history (incl.
			// the terminal run.finished synthesized on a canceled run) lands
			// regardless of run-ctx cancellation.
			s.persistStreamEvent(context.Background(), runID, handle.SessionID, parentRunID, se)
			// Tell workspace subscribers a file changed when an agent file tool
			// completes — precise + fd-free, so the watcher needn't watch the tree.
			s.emitToolFileChange(cwd, se)
			hub.Append(re)
		}
	}

	// run.started leads every segment (root + continuation), independent of
	// any chat-level TurnStart — continuation runs (runs.resume) carry none,
	// so emitting here is what gives them a run boundary + parentRunId, and
	// closes any plan-review question the parked run left open.
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
		s.runMu.Lock()
		if e, ok := s.runs[runID]; ok {
			// A parked run keeps its live turn alive for resume — only
			// cancel + forget on a true terminal.
			if !parked {
				e.cancel()
			}
			delete(s.runs, runID)
		}
		s.runMu.Unlock()
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
func (s *Server) recordInterrupt(ctx context.Context, runID string, handle chat.TurnHandle, ints []protocol.Interrupt, drained []interrupts.DrainedTool) {
	raw, err := json.Marshal(ints)
	if err != nil {
		return
	}
	// Best-effort: persist the interrupt record so the run can be
	// resumed across restarts. If Put fails, same-process resume still
	// works (the process is parked in-memory), but cross-restart resume
	// will not find this interrupt.
	processID, _ := s.rt.Chat().ProcessID(ctx, handle)
	_ = s.rt.Interrupts().Put(ctx, interrupts.Pending{
		ParentRunID:  runID,
		SessionID:    handle.SessionID,
		TurnID:       handle.TurnID,
		ProcessID:    processID,
		Interrupts:   raw,
		DrainedTools: drained,
		CreatedAt:    time.Now().UTC(),
	})
}

// cancelReasonFor returns the runs.cancel reason recorded for a run, or ""
// when it wasn't canceled with one. Read under runMu (S6).
func (s *Server) cancelReasonFor(runID string) string {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if e, ok := s.runs[runID]; ok {
		return e.cancelReason
	}
	return ""
}
