package turn

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/models/catalog"
)

// runTurn starts the turn's agent process and drives its first run
// segment to a suspension point — a HITL interrupt (park) or a terminal
// state. Later segments are driven by [memoryDispatcher.Resume] through the
// shared [drive] loop. st.ctx (the turn's own lifetime) bounds the run.
func (s *memoryDispatcher) runTurn(request StartTurnRequest, st *turnState) {
	st.maxBudget = request.MaxBudget
	st.maxCostUSD = request.MaxCostUSD
	st.maxSteps = request.MaxSteps
	s.emit(st, TurnStart{Model: st.model})

	// Resolve a per-turn client when the run picked a provider+model and a
	// resolver is wired; no selection / no resolver runs on the engine's
	// default client.
	var client *chatclient.Client
	if request.Provider != "" && request.Model != "" && s.resolver != nil {
		c, err := s.resolver.ResolveClient(st.ctx, request.Provider, request.Model)
		if err != nil {
			s.emit(st, ErrorEvent{Message: err.Error(), Code: "MODEL_UNAVAILABLE"})
			s.finishTurn(st, execution.OutcomeError)
			return
		}
		client = c
	}

	observer := &turnObserver{dispatcher: s, st: st}
	st.lifecycle = &turnLifecycle{sessionID: st.handle.SessionID, cwd: st.cwd, hooks: st.hooks}
	process, err := s.engine.StartTurn(st.ctx, agentexec.TurnRequest{
		SessionID:     request.SessionID,
		Message:       request.Message,
		Provider:      request.Provider,
		Media:         request.Media,
		Cwd:           request.Cwd,
		MaxBudget:     request.MaxBudget,
		MaxCostUSD:    request.MaxCostUSD,
		MaxSteps:      request.MaxSteps,
		Options:       request.Options,
		ChatClient:    client,
		Observer:      observer,
		EventListener: st.lifecycle.listener(st.handle.TurnID),
		// Mid-run steering: drained before each continuation round (with the
		// next-turn flushSteering as the after-last-round fallback).
		Steer: s.steerSource(st),
	})
	if err != nil {
		s.emit(st, ErrorEvent{Message: err.Error(), Code: "ENGINE_ERROR"})
		s.finishTurn(st, execution.OutcomeError)
		return
	}
	// Record the root process id so the lifecycle gate keeps subtask
	// terminals (which fire first) from being mistaken for the turn's end.
	st.lifecycle.setRoot(process.ID())
	st.setProcess(process)

	s.drive(st, process.Done())
}

// drive consumes one run segment's completion. When the process parks
// on a HITL interrupt (StatusWaiting) it surfaces a [TurnInterrupted]
// and leaves the turn registered (events channel open) for
// [memoryDispatcher.Resume]. On a terminal state it drains steering, runs
// post-turn maintenance on a clean finish, emits [TurnEnd], and tears
// the turn down. doneCh is the segment's Done channel — the process's
// for the first segment, the resume continuation's thereafter.
func (s *memoryDispatcher) drive(st *turnState, doneCh <-chan error) {
	runErr := <-doneCh
	process := st.process()

	if process.Status() == core.StatusWaiting {
		s.handleWaiting(st, process)
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
	s.completeTurn(st, func() {
		s.emitTurnEnd(st, process, st.lifecycle.terminalEvent(), runErr, time.Since(st.startedAt), st.ctx.Err())
	})
}

// handleWaiting decides what to do when the process parks at StatusWaiting. If
// the pending interrupt's kind is one this turn's client can answer, it
// surfaces it via [memoryDispatcher.emitInterrupt] and the turn waits for
// [memoryDispatcher.Resume]. Otherwise the client could never answer it, so rather
// than leave a deadlocked interrupt (API.md §6.2) the turn auto-denies and the
// continuation runs to a real terminal.
func (s *memoryDispatcher) handleWaiting(st *turnState, process agentexec.TurnProcess) {
	// Canceled while the process was parking: Cancel cancels st.ctx but skips
	// killing a process that still read Running, so a turn that parks just
	// afterwards lands here with a dead ctx. Don't surface an interrupt nobody
	// will answer — terminate the suspended process and emit the terminal.
	if st.ctx.Err() != nil {
		recordTurnCleanupError(st, cancelTurnProcess(process))
		s.finishTurn(st, execution.OutcomeCanceled)
		return
	}
	suspension := process.Suspension()
	kind := interruptKind(suspension)
	if suspension == nil || kind == "" || st.canSurface(kind) {
		s.emitInterrupt(st, process)
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
// its events channel open; [memoryDispatcher.Resume] drives the next segment.
func (s *memoryDispatcher) emitInterrupt(st *turnState, process agentexec.TurnProcess) {
	suspension := process.Suspension()
	if !st.parkIfLive() {
		// Canceled between handleWaiting's top ctx check and here: don't surface
		// an interrupt nobody will answer — terminate like the canceled path so
		// the turn can't linger parked on a dead ctx. (handleWaiting's top check
		// catches cancel-before-handleWaiting; this closes the cancel-during gap.)
		recordTurnCleanupError(st, cancelTurnProcess(process))
		s.finishTurn(st, execution.OutcomeCanceled)
		return
	}
	if suspension == nil {
		recordTurnCleanupError(st, cancelTurnProcess(process))
		s.emit(st, ErrorEvent{Message: "agent process is waiting without a suspension", Code: "ENGINE_ERROR"})
		s.finishTurn(st, execution.OutcomeError)
		return
	}
	pending, ok := typedInterrupt(suspension)
	if !ok {
		recordTurnCleanupError(st, cancelTurnProcess(process))
		s.emit(st, ErrorEvent{Message: "agent process returned an unsupported interrupt payload", Code: "ENGINE_ERROR"})
		s.finishTurn(st, execution.OutcomeError)
		return
	}
	recordInterruptMetric(st.ctx, string(pending.Kind))
	if !s.emit(st, TurnInterrupted{Interrupts: []Interrupt{pending}}) {
		return
	}
	// Notification hooks (observe-only): the turn is waiting on the user — fire
	// so a user script can route it (desktop / Slack / …). The kind ("approval"
	// | "question") rides as the reason.
	if !st.hooks.Empty() {
		_ = st.hooks.Run(st.ctx, hooks.Input{
			Event: hooks.Notification, SessionID: st.handle.SessionID, Cwd: st.cwd, Reason: string(pending.Kind),
		})
	}
}

// interruptKind decodes the application-owned discriminated envelope into its
// wire interrupt kind (API.md §6). Unknown or malformed payloads return "" and
// are rejected by emitInterrupt; there is no field-shape fallback.
func interruptKind(suspension *agent.Suspension) string {
	if suspension == nil {
		return ""
	}
	pending, ok := typedInterrupt(suspension)
	if !ok {
		return ""
	}
	return string(pending.Kind)
}

func typedInterrupt(suspension *agent.Suspension) (Interrupt, bool) {
	if suspension == nil {
		return Interrupt{}, false
	}
	pending, err := runs.DecodeInterrupt(suspension.Prompt)
	if err != nil {
		return Interrupt{}, false
	}
	return pending, true
}

// postTurnMaintenance runs the compact + (conditional) extract pair
// after the turn's real LLM round completed cleanly. Errors at
// this stage surface through ErrorEvent but don't abort the turn —
// the user's reply is already on screen.
//
// A fired compaction emits [CompactBoundary] with before/after message counts.
// Memory extraction writes its durable ledger/curated state internally; it has
// no client event because no application projection consumes one. Maintenance
// failures remain visible through [ErrorEvent].
//
// Memory maintenance is gated on compaction firing: extraction and any due
// curation add model calls, so we amortize them onto the moments where the
// runtime had to summarize anyway.
func (s *memoryDispatcher) postTurnMaintenance(ctx context.Context, st *turnState, sessionID string) {
	if s.compactor == nil {
		return
	}
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
	// Resolve the run's model context window so the token trigger is relative to the
	// model this run actually pinned, not a process-wide default. An unknown model
	// (default selection / catalog miss) passes 0 and the compactor falls back.
	contextWindow := 0
	if info, ok := catalog.Lookup(st.provider, st.model); ok {
		contextWindow = int(info.Limits.ContextWindow)
	}
	compaction, err := s.compactor.MaybeCompact(ctx, sessionID, contextWindow, preCompact)
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

	if s.extractor == nil {
		return
	}
	if err := s.extractor.MaybeExtract(ctx, sessionID, st.cwd); err != nil {
		s.emit(st, ErrorEvent{
			Message: "memory maintenance failed: " + err.Error(),
			Code:    "MEMORY_MAINTENANCE_ERROR",
		})
	}
}
