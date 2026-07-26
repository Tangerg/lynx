package turn

import (
	"context"
	"errors"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/chatclient"
)

// runTurn starts the turn's agent process and drives its first run
// segment to a suspension point — a HITL interrupt (park) or a terminal
// state. Later segments are driven by [memoryDispatcher.Resume] through the
// shared [drive] loop. st.ctx (the turn's own lifetime) bounds the run.
func (s *memoryDispatcher) runTurn(request runs.StartTurn, st *turnState) {
	// Resolve a per-turn client when the run picked a provider+model and a
	// resolver is wired; no selection / no resolver runs on the engine's
	// default client.
	var client *chatclient.Client
	if request.ModelSelection.Configured() && s.resolver != nil {
		c, err := s.resolver.ResolveClient(st.ctx, request.ModelSelection)
		if err != nil {
			s.finishExecutionError(st, problemFromError(err), err)
			return
		}
		client = c
	}

	observer := &turnObserver{dispatcher: s, st: st}
	subagents := newSubagentLifecycle(st.handle.SessionID, st.cwd, st.hooks)
	var eventListener core.Extension
	if subagents != nil {
		eventListener = subagents.listener(st.handle.TurnID)
	}
	process, err := s.engine.StartTurn(st.ctx, agentexec.TurnRequest{
		SessionID:      request.SessionID,
		Message:        request.Message,
		ModelSelection: request.ModelSelection,
		Media:          request.Media,
		Cwd:            request.Cwd,
		Isolated:       request.Isolated,
		GoalLeaseID:    request.GoalLeaseID,
		MaxBudget:      request.MaxBudget,
		MaxCostUSD:     request.MaxCostUSD,
		MaxSteps:       request.MaxSteps,
		Options:        request.Options,
		ChatClient:     client,
		Observer:       observer,
		EventListener:  eventListener,
		// Mid-run steering: drained before each continuation round (with the
		// next-turn flushSteering as the after-last-round fallback).
		Steer: s.steerSource(st),
	})
	if err != nil {
		s.finishExecutionError(st, internalRunProblem(), err)
		return
	}
	if process == nil {
		err := errors.New("turn: engine returned a nil process")
		s.finishExecutionError(st, internalRunProblem(), err)
		return
	}
	if subagents != nil {
		if err := subagents.confirmRoot(process.ID()); err != nil {
			st.setProcess(process)
			st.cancel()
			recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
			s.finishExecutionError(st, internalRunProblem(), err)
			return
		}
	}
	st.setProcess(process)

	s.drive(st)
}

// drive consumes one typed run-segment completion. When the process parks
// on a HITL interrupt (StatusWaiting) it surfaces a [TurnInterrupted]
// and leaves the turn registered (events channel open) for
// [memoryDispatcher.Resume]. On a terminal state it drains steering, runs
// post-turn maintenance on a clean completion, emits [TurnEnd], and tears the
// turn down.
func (s *memoryDispatcher) drive(st *turnState) {
	process := st.process()
	completion := process.Await()

	if completion.Status == core.StatusWaiting {
		s.handleWaiting(st, process)
		return
	}

	// Drain steering into history BEFORE maintenance so the compactor /
	// extractor see it as part of the conversation they summarize.
	s.flushSteering(st.ctx, st, st.handle.SessionID)
	if completion.Status == core.StatusCompleted && completion.Error() == nil && st.handle.SessionID != "" {
		s.postTurnMaintenance(st.ctx, st, st.handle.SessionID)
	}
	// MessageDelta events already streamed through the observer — no
	// need to re-emit the assembled reply here.
	s.completeTurn(st, func() {
		s.emitTurnEnd(st, completion, time.Since(st.startedAt))
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
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
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
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		s.finishTurn(st, execution.OutcomeCanceled)
		return
	}
	if suspension == nil {
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		s.finishFailedTurn(st, internalRunProblem(), errors.New("agent process is waiting without a suspension"))
		return
	}
	pending, ok := typedInterrupt(suspension)
	if !ok {
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		s.finishFailedTurn(st, internalRunProblem(), errors.New("agent process returned an unsupported interrupt payload"))
		return
	}
	recordInterruptMetric(st.ctx, string(pending.Kind))
	if !s.emit(st, runs.TurnInterrupted{Interrupts: []runs.Interrupt{pending}}) {
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
// application interrupt kind. Unknown or malformed payloads return "" and
// are rejected by emitInterrupt; there is no field-shape fallback.
func interruptKind(suspension *agent.Suspension) runs.InterruptKind {
	if suspension == nil {
		return ""
	}
	pending, ok := typedInterrupt(suspension)
	if !ok {
		return ""
	}
	return pending.Kind
}

func typedInterrupt(parked *agent.Suspension) (runs.Interrupt, bool) {
	if parked == nil {
		return runs.Interrupt{}, false
	}
	pending, err := suspension.DecodePrompt(parked.Prompt)
	if err != nil {
		return runs.Interrupt{}, false
	}
	return pending, true
}

// postTurnMaintenance runs turn-boundary housekeeping after the turn's real LLM
// round completed cleanly. Errors are observability facts, not execution facts:
// the user reply has already completed and its outcome must not be rewritten.
//
// The concrete maintenance suite owns worker ordering and conditional work. A
// fired compaction emits [CompactBoundary] with before/after message counts;
// other maintenance output stays internal. Failures are recorded on the active
// turn span and never alter the completed reply.
func (s *memoryDispatcher) postTurnMaintenance(ctx context.Context, st *turnState, sessionID string) {
	if s.maintenance == nil {
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
	result := s.maintenance.Maintain(ctx, BoundaryMaintenanceInput{
		SessionID:      sessionID,
		Cwd:            st.cwd,
		ModelSelection: st.modelSelection,
		ToolCalls:      st.toolCallCount(),
		PreCompact:     preCompact,
	})
	for _, err := range result.Errors {
		recordTurnMaintenanceError(st, err)
	}
	if !result.Compaction.Compacted {
		return
	}
	s.emit(st, runs.CompactBoundary{
		MessagesBefore: result.Compaction.MessagesBefore,
		MessagesAfter:  result.Compaction.MessagesAfter,
	})
}
