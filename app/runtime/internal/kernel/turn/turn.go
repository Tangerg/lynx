package turn

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
)

// runTurn starts the turn's agent process and drives its first run
// segment to a suspension point — a HITL interrupt (park) or a terminal
// state. Later segments are driven by [inMemory.Resume] through the
// shared [drive] loop. st.ctx (the turn's own lifetime) bounds the run.
func (s *inMemory) runTurn(req StartTurnRequest, st *turnState) {
	st.maxBudget = req.MaxBudget
	st.maxCostUSD = req.MaxCostUSD
	st.maxSteps = req.MaxSteps
	s.emit(st, TurnStart{Model: st.model})

	// Resolve a per-turn client when the run picked a provider+model and a
	// resolver is wired; no selection / no resolver runs on the platform's
	// default client.
	var client core.ChatClient
	if req.Provider != "" && req.Model != "" && s.resolver != nil {
		c, err := s.resolver.ResolveClient(st.ctx, req.Provider, req.Model)
		if err != nil {
			s.emit(st, ErrorEvent{Message: err.Error(), Code: "MODEL_UNAVAILABLE"})
			s.finishTurn(st, TurnEndErrored)
			return
		}
		client = c
	}

	observer := &turnObserver{svc: s, st: st}
	st.lifecycle = &turnLifecycle{sessionID: st.handle.SessionID, cwd: st.cwd, hooks: st.hooks}
	proc := s.engine.StartTurn(st.ctx, kernel.TurnRequest{
		SessionID:     req.SessionID,
		Message:       req.Message,
		Provider:      req.Provider,
		Media:         req.Media,
		Cwd:           req.Cwd,
		MaxBudget:     req.MaxBudget,
		MaxCostUSD:    req.MaxCostUSD,
		MaxSteps:      req.MaxSteps,
		Options:       req.Options.Clone(),
		ChatClient:    client,
		Observer:      observer,
		EventListener: st.lifecycle.listener(st.handle.TurnID),
		// Mid-run steering: drained before each continuation round (with the
		// next-turn flushSteering as the after-last-round fallback).
		Steer: s.steerSource(st),
	})
	// Record the root process id so the lifecycle gate keeps subtask
	// terminals (which fire first) from being mistaken for the turn's end.
	st.lifecycle.setRoot(proc.ID())
	st.setProc(proc)

	s.drive(st, proc.Done())
}

// drive consumes one run segment's completion. When the process parks
// on a HITL interrupt (StatusWaiting) it surfaces a [TurnInterrupted]
// and leaves the turn registered (events channel open) for
// [inMemory.Resume]. On a terminal state it drains steering, runs
// post-turn maintenance on a clean finish, emits [TurnEnd], and tears
// the turn down. doneCh is the segment's Done channel — the process's
// for the first segment, the resume continuation's thereafter.
func (s *inMemory) drive(st *turnState, doneCh <-chan error) {
	runErr := <-doneCh
	proc := st.process()

	if proc.Status() == core.StatusWaiting {
		s.handleWaiting(st, proc)
		return
	}

	// Drain steering into history BEFORE maintenance so the compactor /
	// extractor see it as part of the conversation they summarize.
	s.flushSteering(st.ctx, st, st.handle.SessionID)
	if runErr == nil && st.handle.SessionID != "" {
		s.postTurnMaintenance(st.ctx, st, st.handle.SessionID)
	}
	// MessageDelta events already streamed through the observer — no
	// need to re-emit the assembled reply here.
	s.emitTurnEnd(st, proc, st.lifecycle.get(), runErr, time.Since(st.startedAt), st.ctx.Err())
	s.endTurn(st)
}

// handleWaiting decides what to do when the process parks at
// StatusWaiting. If the pending interrupt's kind is one the client can
// answer (see [inMemory.canSurface]) it surfaces it via
// [inMemory.emitInterrupt] and the turn waits for [inMemory.Resume].
// Otherwise the client could never answer it, so rather than leave a
// deadlocked interrupt (API.md §6.2) the turn auto-denies (via the shared
// [inMemory.resumeAndDrive]) and the continuation runs to a real terminal.
func (s *inMemory) handleWaiting(st *turnState, proc kernel.TurnProcess) {
	// Canceled while the process was parking: Cancel cancels st.ctx but skips
	// killing a process that still read Running, so a turn that parks just
	// afterwards lands here with a dead ctx. Don't surface an interrupt nobody
	// will answer — terminate the suspended process and emit the terminal.
	if st.ctx.Err() != nil {
		_ = proc.Cancel()
		s.finishTurn(st, TurnEndCanceled)
		return
	}
	aw := proc.PendingAwaitable()
	if aw == nil || s.canSurface(interruptKind(aw)) {
		s.emitInterrupt(st, proc)
		return
	}
	// Client can't answer this kind — deliver a deny and drive the
	// continuation (resumeAndDrive streams the terminal on a resume error
	// and launches drive otherwise; the returned error is already surfaced
	// on the channel, so it's safe to drop here).
	_ = s.resumeAndDrive(st, interrupts.Resolution{Approved: false})
}

// emitInterrupt marks the turn parked and surfaces the pending HITL
// request as a [TurnInterrupted] event. The turn stays registered with
// its events channel open; [inMemory.Resume] drives the next segment.
func (s *inMemory) emitInterrupt(st *turnState, proc kernel.TurnProcess) {
	aw := proc.PendingAwaitable()
	if !st.parkIfLive() {
		// Canceled between handleWaiting's top ctx check and here: don't surface
		// an interrupt nobody will answer — terminate like the canceled path so
		// the turn can't linger parked on a dead ctx. (handleWaiting's top check
		// catches cancel-before-handleWaiting; this closes the cancel-during gap.)
		_ = proc.Cancel()
		s.finishTurn(st, TurnEndCanceled)
		return
	}
	if aw == nil {
		// Defensive: Waiting without a parked awaitable shouldn't happen;
		// surface an empty interrupt rather than silently dropping it.
		s.emit(st, TurnInterrupted{})
		return
	}
	kind := interruptKind(aw)
	recordInterruptMetric(st.ctx, kind)
	s.emit(st, TurnInterrupted{Interrupts: []Interrupt{{Kind: kind, Payload: aw.PromptAny()}}})
	// Notification hooks (observe-only): the turn is waiting on the user — fire
	// so a user script can route it (desktop / Slack / …). The kind ("approval"
	// | "question") rides as the reason.
	if !st.hooks.Empty() {
		_ = st.hooks.Run(st.ctx, hooks.Input{
			Event: hooks.Notification, SessionID: st.handle.SessionID, Cwd: st.cwd, Reason: kind,
		})
	}
}

// interruptKind classifies the pending awaitable into the wire interrupt
// kind (API.md §6: "approval" | "question" | "toolResult"). An
// [ApprovalPrompt] payload is a gated tool call ("approval"); anything
// else is a structured question (ask_user / exit_plan_mode), which surfaces
// as a "question". Returns "" for a nil awaitable (treated as surfaceable so
// the defensive empty-interrupt path in emitInterrupt still fires).
func interruptKind(aw core.Awaitable) string {
	if aw == nil {
		return ""
	}
	if _, ok := aw.PromptAny().(ApprovalPrompt); ok {
		return "approval"
	}
	return "question"
}

// postTurnMaintenance runs the compact + (conditional) extract pair
// after the turn's real LLM round completed cleanly. Errors at
// this stage surface through ErrorEvent but don't abort the turn —
// the user's reply is already on screen.
//
// Both maintenance actions are observable: a fired compaction emits
// [CompactBoundary] (before/after message counts) and a successful
// extraction emits [MemoryUpdated] (the facts saved). Surfacing them
// keeps the runtime's housekeeping visible to clients instead of
// silently mutating context behind the user's back — the SDK's
// SDKCompactBoundaryMessage / memory-event spirit, adapted.
//
// Fact extraction is gated on compaction firing: extraction is one
// extra LLM call, so we amortize it onto the moments where the
// runtime had to summarize anyway.
func (s *inMemory) postTurnMaintenance(ctx context.Context, st *turnState, sessionID string) {
	// PreCompact hooks fire from inside MaybeCompact — exactly when a compaction
	// is committed (after its triggers + guards), never on a turn that won't
	// compact. A hook may veto (Block) the compaction; observe-only otherwise.
	preCompact := func(hctx context.Context) bool {
		if st.hooks.Empty() {
			return true
		}
		dec := st.hooks.Run(hctx, hooks.Input{Event: hooks.PreCompact, SessionID: sessionID, Cwd: st.cwd})
		return !dec.Block
	}
	compaction, err := s.engine.MaybeCompact(ctx, sessionID, preCompact)
	if err != nil {
		s.emit(st, ErrorEvent{
			Message: "auto-compaction failed: " + err.Error(),
			Code:    "COMPACTION_ERROR",
		})
		return
	}
	if !compaction.Compacted {
		return
	}
	s.emit(st, CompactBoundary{
		MessagesBefore: compaction.MessagesBefore,
		MessagesAfter:  compaction.MessagesAfter,
	})

	extraction, err := s.engine.MaybeExtract(ctx, sessionID, st.cwd)
	if err != nil {
		s.emit(st, ErrorEvent{
			Message: "memory extraction failed: " + err.Error(),
			Code:    "EXTRACTION_ERROR",
		})
		return
	}
	if extraction.Extracted {
		s.emit(st, MemoryUpdated{Facts: extraction.Facts})
	}
}
