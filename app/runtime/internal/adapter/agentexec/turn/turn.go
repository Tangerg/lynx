package turn

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/suspension"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/hooks"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/chatclient"
)

// runTurn starts the turn's agent process and drives its first run
// segment to a suspension point — a HITL interrupt (park) or a terminal
// state. Later segments are driven by [controller.Resume] through the
// shared [drive] loop. st.ctx (the turn's own lifetime) bounds the run.
func (s *controller) runTurn(request runs.RootExecutionStart, st *turnState) {
	// Resolve a per-turn client when the Run picked a provider+model. Preparation
	// has already rejected an explicit selection without a chat resolver.
	var client *chatclient.Client
	if request.ModelSelection.Configured() {
		c, err := s.chatResolver.ResolveChat(st.ctx, request.ModelSelection)
		if err != nil {
			recordTurnCleanupError(st, s.finishExecutionError(st, problemFromError(err), err))
			return
		}
		if c == nil {
			err := errors.New("turn: chat resolver returned nil for an explicit model selection")
			recordTurnCleanupError(st, s.finishExecutionError(st, internalRunProblem(), err))
			return
		}
		client = c
	}

	observer := &turnObserver{
		controller:       s,
		st:               st,
		projectChildRuns: request.ChildRunAdmissionEnabled,
	}
	var admitChild agentexec.AdmitChildFunc
	if request.ChildRunAdmissionEnabled {
		admitChild = observer.admitChild
	}
	subagents := newSubagentLifecycle(
		st.handle.SessionID,
		st.cwd,
		st.hooks,
		observer.childRun,
		s.engine.SubagentProjection,
	)
	var eventListener core.Extension
	if subagents != nil {
		eventListener = subagents.listener(st.handle.TurnID)
	}
	process, err := s.engine.StartTurn(st.ctx, agentexec.TurnRequest{
		SessionID:      request.SessionID,
		Message:        request.Message,
		ModelSelection: request.ModelSelection,
		Media:          request.Media,
		CWD:            request.CWD,
		WorkspaceCWD:   request.WorkspaceCWD,
		Isolated:       request.Isolated,
		GoalLeaseID:    request.GoalLeaseID,
		Limits:         request.Limits,
		Options:        request.Options,
		ChatClient:     client,
		Observer:       observer,
		EventListener:  eventListener,
		AdmitChild:     admitChild,
		// Mid-run steering: drained before each continuation round (with the
		// next-turn flushSteering as the after-last-round fallback).
		Steer: s.steerSource(st),
	})
	if err != nil {
		recordTurnCleanupError(st, s.finishExecutionError(st, internalRunProblem(), err))
		return
	}
	if process == nil {
		err := errors.New("turn: engine returned a nil process")
		recordTurnCleanupError(st, s.finishExecutionError(st, internalRunProblem(), err))
		return
	}
	if subagents != nil {
		if err := subagents.confirmRoot(process.ID()); err != nil {
			st.setProcess(process)
			st.cancel()
			recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
			recordTurnCleanupError(st, s.finishExecutionError(st, internalRunProblem(), err))
			return
		}
	}
	st.setProcess(process)

	s.drive(st)
}

// drive consumes one typed Agent-runtime completion. When the process parks
// on a HITL interrupt (StatusWaiting) it surfaces a [TurnInterrupted]
// and leaves the turn registered (events channel open) for
// [controller.Resume]. On a terminal state it drains steering, runs
// post-turn maintenance on a clean completion, emits [TurnEnd], and tears the
// turn down.
func (s *controller) drive(st *turnState) {
	process := st.process()
	completion := process.Await()

	if completion.Status == core.StatusWaiting {
		if err := completion.Err; err != nil {
			recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
			recordTurnCleanupError(st, s.finishFailedTurn(st, internalRunProblem(), err))
			return
		}
		s.handleWaiting(st, process)
		return
	}

	// Drain steering into history BEFORE maintenance so the compactor /
	// extractor see it as part of the conversation they summarize.
	s.flushSteering(st.ctx, st, st.handle.SessionID)
	if completion.Status == core.StatusCompleted && completion.Err == nil && st.handle.SessionID != "" {
		s.postMaintenance(st.ctx, st, st.handle.SessionID)
	}
	// MessageDelta events already streamed through the observer — no
	// need to re-emit the assembled reply here.
	recordTurnCleanupError(st, s.completeTurn(st, func() {
		s.emitTurnEnd(st, completion, st.segmentElapsed())
	}))
}

// handleWaiting decides what to do when the process parks at StatusWaiting. If
// the pending interrupt's kind is one this turn's client can answer, it
// surfaces it via [controller.emitInterrupt] and the turn waits for
// [controller.Resume]. Otherwise the client could never answer it, so rather
// than leave a deadlocked interrupt, the turn auto-denies and the
// continuation runs to a real terminal.
func (s *controller) handleWaiting(st *turnState, process agentexec.TurnProcess) {
	// Canceled while the process was parking: Cancel cancels st.ctx but skips
	// killing a process that still read Running, so a turn that parks just
	// afterwards lands here with a dead ctx. Don't surface an interrupt nobody
	// will answer — terminate the suspended process and emit the terminal.
	if st.ctx.Err() != nil {
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		recordTurnCleanupError(st, s.finishTurn(st, run.OutcomeCanceled))
		return
	}
	pending, err := process.PendingSuspensions(st.ctx)
	if err != nil {
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		recordTurnCleanupError(st, s.finishFailedTurn(st, internalRunProblem(), err))
		return
	}
	if len(pending) == 0 {
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		recordTurnCleanupError(st, s.finishFailedTurn(
			st,
			internalRunProblem(),
			errors.New("agent process tree is waiting without an unanswered suspension"),
		))
		return
	}
	canSurfaceAll := true
	for _, suspension := range pending {
		interrupt, ok := typedInterrupt(suspension.Prompt)
		if !ok || !st.canSurface(interrupt.Kind) {
			canSurfaceAll = false
			break
		}
	}
	if canSurfaceAll {
		s.emitInterrupt(st, process)
		return
	}
	answers := make([]agentexec.SuspensionAnswer, len(pending))
	for index, boundary := range pending {
		answers[index] = agentexec.SuspensionAnswer{
			ProcessID:    boundary.ProcessID,
			SuspensionID: boundary.SuspensionID,
			Resolution:   interrupt.Resolution{Approved: false},
		}
	}
	// Client can't answer every kind — deny the whole accepted set and drive the
	// continuations (resumeAndDrive streams the terminal on a resume error
	// and launches drive otherwise; the returned error is already surfaced
	// on the channel, so it's safe to drop here).
	_ = s.resumeAndDrive(st.ctx, st, answers)
}

// emitInterrupt marks the turn parked and surfaces the pending HITL
// request as a [TurnInterrupted] event. The turn stays registered with
// its events channel open; [controller.Resume] drives the next segment.
func (s *controller) emitInterrupt(
	st *turnState,
	process agentexec.TurnProcess,
) {
	checkpoint, err := process.CaptureWaitingCheckpoint(st.ctx)
	if err != nil {
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		recordTurnCleanupError(st, s.finishFailedTurn(st, internalRunProblem(), err))
		return
	}
	pending := checkpoint.Suspensions
	if !st.parkIfLive() {
		// Canceled between handleWaiting's top ctx check and here: don't surface
		// an interrupt nobody will answer — terminate like the canceled path so
		// the turn can't linger parked on a dead ctx. (handleWaiting's top check
		// catches cancel-before-handleWaiting; this closes the cancel-during gap.)
		recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
		recordTurnCleanupError(st, s.finishTurn(st, run.OutcomeCanceled))
		return
	}
	barrier := runs.TreeInterrupted{
		Checkpoint:    checkpoint.Checkpoint,
		Interruptions: make([]runs.MemberInterruption, len(pending)),
	}
	for index, suspension := range pending {
		interrupt, ok := typedInterrupt(suspension.Prompt)
		if !ok {
			recordTurnCleanupError(st, cancelTurnProcess(st.ctx, process))
			recordTurnCleanupError(st, s.finishFailedTurn(
				st,
				internalRunProblem(),
				fmt.Errorf(
					"agent process %q suspension %q has an unsupported interrupt payload",
					suspension.ProcessID,
					suspension.SuspensionID,
				),
			))
			return
		}
		recordInterruptMetric(st.ctx, interrupt.Kind.String())
		barrier.Interruptions[index] = runs.MemberInterruption{
			MemberID:  suspension.ProcessID,
			RequestID: suspension.SuspensionID,
			Interrupt: interrupt,
		}
	}
	if !s.emitProcessEvent(st, agentexec.ProcessRef{ID: process.ID()}, barrier) {
		return
	}
	// Notification hooks (observe-only): the turn is waiting on the user — fire
	// so external automation can route it. The kind ("approval"
	// | "question") rides as the reason.
	if !st.hooks.Empty() {
		_ = st.hooks.Run(st.ctx, hooks.Input{
			Event: hooks.Notification, SessionID: st.handle.SessionID, CWD: st.cwd, Reason: "interrupt",
		})
	}
}

func typedInterrupt(prompt []byte) (runs.Interrupt, bool) {
	if len(prompt) == 0 {
		return runs.Interrupt{}, false
	}
	pending, err := suspension.DecodePrompt(prompt)
	if err != nil {
		return runs.Interrupt{}, false
	}
	return pending, true
}

// postMaintenance runs Run-boundary housekeeping after the Run's real LLM
// round completed cleanly. Errors are observability facts, not execution facts:
// the user reply has already completed and its outcome must not be rewritten.
//
// The concrete maintenance pipeline owns worker ordering and conditional work. A
// fired compaction emits [CompactionBoundary] with before/after message counts;
// other maintenance output stays internal. Failures are recorded on the active
// execution span and never alter the completed reply.
func (s *controller) postMaintenance(ctx context.Context, st *turnState, sessionID string) {
	if s.maintenance == nil {
		return
	}
	// PreCompact hooks fire from inside MaybeCompact — exactly when a compaction
	// is committed (after its triggers + guards), never on a Run that won't
	// compact. A hook may veto (Block) the compaction; observe-only otherwise.
	preCompact := func(hctx context.Context) bool {
		if st.hooks.Empty() {
			return true
		}
		dec := st.hooks.Run(hctx, hooks.Input{Event: hooks.PreCompact, SessionID: sessionID, CWD: st.cwd})
		return !dec.Block
	}
	result := s.maintenance.Maintain(ctx, MaintenanceInput{
		SessionID:      sessionID,
		CWD:            st.cwd,
		ModelSelection: st.modelSelection,
		ToolCalls:      st.toolCallCount(),
		PreCompact:     preCompact,
	})
	for _, err := range result.Errors {
		recordMaintenanceError(st, err)
	}
	if !result.Compaction.Compacted {
		return
	}
	s.emitRootEvent(st, runs.CompactionBoundary{
		MessagesBefore: result.Compaction.MessagesBefore,
		MessagesAfter:  result.Compaction.MessagesAfter,
	})
}
