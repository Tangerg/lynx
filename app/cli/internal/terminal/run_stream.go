package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/runrecovery"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

const (
	animationInterval     = 100 * time.Millisecond
	runtimeControlTimeout = 5 * time.Second
)

type activeDurationClock struct {
	carried          time.Duration
	segmentStartedAt time.Time
}

// startRunCallError identifies an error returned by RunLifecycle.StartRun
// itself. Protocol validation failures after a successful call are deliberately
// excluded: the runtime already acknowledged the command, so replaying the
// mutation cannot repair its malformed receipt.
type startRunCallError struct{ err error }

func (e *startRunCallError) Error() string { return e.err.Error() }
func (e *startRunCallError) Unwrap() error { return e.err }

// resumeRunCallError has the same acknowledgement semantics as
// startRunCallError, but its recovery owner is the still-open HITL review.
type resumeRunCallError struct{ err error }

func (e *resumeRunCallError) Error() string { return e.err.Error() }
func (e *resumeRunCallError) Unwrap() error { return e.err }

func (clock *activeDurationClock) start(carried time.Duration, at time.Time) {
	clock.carried = carried
	clock.segmentStartedAt = at
}

func (clock *activeDurationClock) elapsed(at time.Time) time.Duration {
	if clock.segmentStartedAt.IsZero() {
		return clock.carried
	}
	current := at.Sub(clock.segmentStartedAt)
	if current < 0 {
		return clock.carried
	}
	return clock.carried + current
}

func (a *app) startRun(commandID agent.CommandID, message agent.Message, options agent.RunOptions, status string) bool {
	input := agent.StartRun{CommandID: commandID, SessionID: a.session.ID, Message: message.Clone(), Options: options.Clone()}
	if err := a.conversation.Starting(); err != nil {
		a.fail(err)
		return false
	}
	if a.workbench != nil {
		if err := a.workbench.MarkPendingRunDispatching(
			input.SessionID, input.CommandID, commandReplayGuard(a.runtimeProfile),
		); err != nil {
			rollbackErr := a.conversation.CancelStarting()
			a.message("run start blocked: save dispatching run: " + err.Error())
			if rollbackErr != nil {
				a.fail(errors.Join(err, rollbackErr))
			}
			return false
		}
	}
	a.transcript.Follow()
	a.activity.Reset()
	a.header.SetUsage(agent.Usage{})
	a.prompt.SetBusy(true)
	a.status.active(status)
	a.executionClock.start(0, time.Now())
	a.syncAnimation()
	a.followOpening(func(ctx context.Context) (agent.SegmentStream, error) {
		opened, err := openStartRun(ctx, a.runtime, input, reconnect.Policy{})
		if err != nil {
			if _, accepted := agent.AcceptedMutationReceipt(err); accepted {
				return agent.SegmentStream{}, err
			}
			return agent.SegmentStream{}, &startRunCallError{err: err}
		}
		if err := opened.ValidateStart(); err != nil {
			return agent.SegmentStream{}, agent.NewAcceptedMutationError(opened, fmt.Errorf("start run: %w", err))
		}
		return opened, nil
	}, streamOpeningObserver{
		persistent: true,
		accepted: func(opened agent.SegmentStream) streamOpeningDisposition {
			a.acceptStartedRun(input, opened)
			return followOpenedStream
		},
		rejected: func(err error) error {
			if receipt, accepted := agent.AcceptedMutationReceipt(err); accepted &&
				strings.TrimSpace(receipt.RunID) != "" {
				a.openingRunID = receipt.RunID
				a.cancelRuntimePreservingFailure(agent.CancelRun{
					RunID: receipt.RunID, Reason: "runtime returned an invalid start receipt",
				})
			}
			return errors.Join(err, a.requeueDefinitivelyRefusedStart(input, err))
		},
	})
	return true
}

func (a *app) acceptStartedRun(input agent.StartRun, opened agent.SegmentStream) {
	a.openingRunID = opened.RunID
	if a.workbench == nil {
		return
	}
	pending := a.workbench.PendingRuns(input.SessionID)
	if len(pending) == 0 || pending[0].Command.CommandID != input.CommandID {
		return
	}
	if pending[0].State != workbench.PendingRunCanceling {
		return
	}
	a.status.active("canceling")
	a.requestRuntimeCancellation(agent.CancelRun{
		CommandID: pending[0].CancelCommandID,
		RunID:     opened.RunID,
		Reason:    "canceled while start delivery was unconfirmed",
	}, applyRuntimeSettlement)
}

func (a *app) requeueDefinitivelyRefusedStart(input agent.StartRun, failure error) error {
	callFailure, refused := errors.AsType[*startRunCallError](failure)
	_, dispatchingPresent := a.queue.Dispatching(input.SessionID)
	if !refused || mutation.OutcomeUnknown(callFailure.err) || !dispatchingPresent {
		return nil
	}
	var replacement agent.CommandID
	var err error
	if a.workbench != nil {
		replacement, err = a.workbench.RequeuePendingRun(input.SessionID, input.CommandID)
		if err != nil {
			return fmt.Errorf("requeue refused run: %w", err)
		}
	} else {
		replacement, err = agent.NewCommandID()
		if err != nil {
			return fmt.Errorf("prepare refused run for retry: %w", err)
		}
	}
	if err := a.queue.RequeueDispatch(input.SessionID, input.CommandID, replacement); err != nil {
		return fmt.Errorf("reidentify refused run: %w", err)
	}
	return nil
}

func openStartRun(ctx context.Context, runtime agent.RunLifecycle, command agent.StartRun, policy reconnect.Policy) (agent.SegmentStream, error) {
	for attempt := 1; ; attempt++ {
		opened, err := runtime.StartRun(ctx, command)
		if err == nil {
			return opened, nil
		}
		delay, shouldRetry := policy.Next(attempt, err)
		if !shouldRetry {
			return agent.SegmentStream{}, err
		}
		if err := retry.Wait(ctx, delay); err != nil {
			return agent.SegmentStream{}, err
		}
	}
}

type streamOpeningDisposition uint8

const (
	rejectOpenedStream streamOpeningDisposition = iota
	followOpenedStream
)

type streamOpeningObserver struct {
	// accepted owns the linearization boundary between command acknowledgement
	// and stream consumption. It may reject a valid runtime stream when the local
	// projection cannot safely install the acknowledged state.
	accepted func(agent.SegmentStream) streamOpeningDisposition
	rejected func(error) error
	// persistent makes retryable opening failures wait for either an
	// acknowledgement or owner cancellation. It is reserved for idempotent
	// mutations whose delivery outcome is ambiguous after a disconnect.
	persistent bool
}

func (a *app) followOpening(
	open func(context.Context) (agent.SegmentStream, error),
	observer streamOpeningObserver,
) {
	a.dropStream()
	sequence := a.streamSeq
	a.following = true
	a.operations.Go(streamOperation, true, func(ctx context.Context, _ operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: a.loop.Dispatcher(), sequence: sequence,
			open: open, applyEvent: a.apply,
			policy: reconnect.New(a.settings.UI.ReconnectAttempts),
		}
		follower.opening = observer
		follower.run()
	})
}

// followRecoveredSession closes the read-then-subscribe gap when the terminal
// opens an already-running session. The background owner attaches first, takes
// a second authoritative read, atomically installs it on the UI thread, and
// only then starts consuming the attached tail.
func (a *app) followRecoveredSession() {
	a.dropStream()
	sequence := a.streamSeq
	a.following = true
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(streamOperation, true, func(ctx context.Context, _ operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: dispatcher, sequence: sequence,
			applyEvent: a.apply, policy: reconnect.New(a.settings.UI.ReconnectAttempts),
		}
		recovered, ok := follower.restoreAttachedSession(a.session.ID)
		if !ok {
			return
		}
		active := true
		var reconcileErr error
		postErr := post(ctx, dispatcher, func() {
			if a.streamSeq != sequence {
				active = false
				return
			}
			reconcileErr = a.reconcileRunSnapshot(recovered.Snapshot, recovered.Stream)
		})
		if postErr != nil || reconcileErr != nil {
			follower.postFailure("", errors.Join(postErr, reconcileErr))
			return
		}
		if !active || recovered.Run.Status != agent.RunStatusRunning {
			return
		}
		follower.checkpoint = recovered.Stream.HeadEventID
		follower.runStream(recovered.Stream)
	})
}

type streamFollower struct {
	app        *app
	ctx        context.Context
	dispatcher program.Dispatcher
	sequence   uint64
	open       func(context.Context) (agent.SegmentStream, error)
	applyEvent func(agent.RunEvent) error
	policy     reconnect.Policy
	opening    streamOpeningObserver
	failures   int
	checkpoint string
}

func (f *streamFollower) restoreAttachedSession(sessionID string) (runrecovery.State, bool) {
	for {
		recovered, err := runrecovery.AttachSession(f.ctx, f.app.runtime, sessionID)
		if err == nil {
			f.failures = 0
			return recovered, true
		}
		if !f.waitBeforeRetry("", fmt.Errorf("restore active session: %w", err)) {
			return runrecovery.State{}, false
		}
	}
}

// eventApplicationError marks a runtime event that reached the terminal but could
// not be folded into its conversation projection. Reconnecting cannot repair an
// invalid or conflicting event; only transport failures are eligible for replay.
type eventApplicationError struct{ err error }

func (e *eventApplicationError) Error() string { return e.err.Error() }
func (e *eventApplicationError) Unwrap() error { return e.err }

func (f *streamFollower) run() {
	var current agent.SegmentStream
	for {
		opened, err := f.open(f.ctx)
		if err == nil {
			current = opened
			f.failures = 0
			break
		}
		if !f.waitBeforeOpenRetry(err) {
			return
		}
	}
	if !f.postOpenAccepted(current) {
		return
	}
	f.runStream(current)
}

func (f *streamFollower) postOpenAccepted(opened agent.SegmentStream) bool {
	if f.opening.accepted == nil {
		return true
	}
	active := true
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			active = false
			return
		}
		active = f.opening.accepted(opened) == followOpenedStream && f.app.streamSeq == f.sequence
	})
	return err == nil && active
}

func (f *streamFollower) waitBeforeOpenRetry(cause error) bool {
	f.failures++
	if f.opening.persistent && mutation.AcknowledgementUncertain(cause) {
		delay := runtimeRecoveryBackoff.Delay(f.failures)
		return f.postRetryStatus(true) && retry.Wait(f.ctx, delay) == nil
	}
	delay, shouldRetry := f.policy.Next(f.failures, cause)
	if !shouldRetry {
		f.postOpenFailure(cause)
		return false
	}
	return f.postRetryStatus(false) && retry.Wait(f.ctx, delay) == nil
}

func (f *streamFollower) runStream(current agent.SegmentStream) {
	if err := current.Validate(); err != nil {
		f.postFailure(current.RunID, err)
		return
	}
	for {
		active, applied, streamErr := f.consume(current.Events)
		if !active {
			return
		}
		if applicationErr, ok := errors.AsType[*eventApplicationError](streamErr); ok {
			f.postFailure(current.RunID, applicationErr.err)
			return
		}
		snapshot, err := f.snapshot()
		if err != nil || !snapshot.active {
			return
		}
		f.checkpoint = snapshot.checkpoint
		if snapshot.phase != agent.ConversationRunning {
			f.finish()
			return
		}
		if streamErr == nil {
			streamErr = fmt.Errorf("%w: segment stream ended without a terminal event", agent.ErrDisconnected)
		}
		if context.Cause(f.ctx) != nil {
			return
		}
		if applied > 0 {
			f.failures = 0
		}
		rebound, ok := f.reconnect(current.RunID, current.SegmentID, streamErr)
		if !ok {
			return
		}
		current = rebound
	}
}

func (f *streamFollower) consume(stream agent.EventStream) (bool, int, error) {
	applied := 0
	for event, err := range stream {
		if err != nil {
			return true, applied, err
		}
		accepted, err := f.apply(event)
		if err != nil || !accepted {
			return accepted, applied, err
		}
		applied++
	}
	return true, applied, nil
}

func (f *streamFollower) apply(event agent.RunEvent) (bool, error) {
	active := true
	var applyErr error
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			active = false
			return
		}
		applyErr = f.applyEvent(event)
		f.checkpoint = f.app.conversation.Checkpoint()
	})
	if applyErr != nil {
		applyErr = &eventApplicationError{err: applyErr}
	}
	return active, errors.Join(err, applyErr)
}

type followSnapshot struct {
	active     bool
	checkpoint string
	phase      agent.ConversationPhase
}

type recoveryDisposition uint8

const (
	recoveryStopped recoveryDisposition = iota
	recoveryRetry
	recoveryAttached
)

type recoveryAttempt struct {
	disposition recoveryDisposition
	stream      agent.SegmentStream
	cause       error
}

func (f *streamFollower) snapshot() (followSnapshot, error) {
	snapshot := followSnapshot{active: true}
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			snapshot.active = false
			return
		}
		snapshot.checkpoint = f.app.conversation.Checkpoint()
		snapshot.phase = f.app.conversation.Phase()
	})
	return snapshot, err
}

func (f *streamFollower) reconnect(runID, segmentID string, cause error) (agent.SegmentStream, bool) {
	for {
		if !f.waitBeforeRetry(runID, cause) {
			return agent.SegmentStream{}, false
		}
		rebound, err := f.app.runtime.SubscribeRun(f.ctx, agent.SubscribeRun{
			RunID: runID, SegmentID: segmentID, AfterEventID: f.checkpoint,
		})
		if err == nil {
			return f.acceptRebound(runID, segmentID, rebound)
		}
		if !runrecovery.Required(err) {
			cause = err
			continue
		}
		recovery := f.recover(runID, cause)
		switch recovery.disposition {
		case recoveryRetry:
			cause = recovery.cause
		case recoveryAttached:
			return recovery.stream, true
		default:
			return agent.SegmentStream{}, false
		}
	}
}

func (f *streamFollower) waitBeforeRetry(runID string, cause error) bool {
	f.failures++
	delay, shouldRetry := f.policy.Next(f.failures, cause)
	if !shouldRetry {
		f.postFailure(runID, cause)
		return false
	}
	return f.postRetryStatus(false) && retry.Wait(f.ctx, delay) == nil
}

func (f *streamFollower) postRetryStatus(persistent bool) bool {
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq == f.sequence {
			label := fmt.Sprintf("reconnecting %d/%d", f.failures, f.policy.Attempts)
			if persistent {
				label = fmt.Sprintf("confirming delivery · attempt %d", f.failures)
			}
			f.app.status.note(label)
			f.app.syncAnimation()
		}
	})
	return err == nil
}

func (f *streamFollower) acceptRebound(runID, segmentID string, rebound agent.SegmentStream) (agent.SegmentStream, bool) {
	if err := rebound.ValidateSubscription(); err != nil {
		f.postFailure(runID, err)
		return agent.SegmentStream{}, false
	}
	if rebound.RunID != runID || rebound.SegmentID != segmentID {
		f.postFailure(runID, errors.New("runtime rebound a different run segment"))
		return agent.SegmentStream{}, false
	}
	return rebound, true
}

func (f *streamFollower) recover(runID string, cause error) recoveryAttempt {
	recovered, err := runrecovery.Recover(f.ctx, f.app.runtime, f.app.session.ID, runID)
	if err != nil {
		if runrecovery.Required(err) {
			return recoveryAttempt{disposition: recoveryRetry, cause: cause}
		}
		return recoveryAttempt{disposition: recoveryRetry, cause: err}
	}
	active := true
	var reconcileErr error
	postErr := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			active = false
			return
		}
		reconcileErr = f.app.reconcileRunSnapshot(recovered.Snapshot, recovered.Stream)
	})
	if postErr != nil || reconcileErr != nil {
		f.postFailure(runID, errors.Join(postErr, reconcileErr))
		return recoveryAttempt{disposition: recoveryStopped}
	}
	if !active || recovered.Run.Status != agent.RunStatusRunning {
		return recoveryAttempt{disposition: recoveryStopped}
	}
	f.checkpoint = recovered.Stream.HeadEventID
	return recoveryAttempt{disposition: recoveryAttached, stream: recovered.Stream}
}

func (f *streamFollower) finish() {
	_ = post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq == f.sequence {
			f.app.finishFollowing()
		}
	})
}

func (f *streamFollower) postFailure(runID string, err error) {
	if errors.Is(err, context.Canceled) || f.ctx.Err() != nil {
		return
	}
	_ = post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			return
		}
		f.app.fail(err)
		if runID != "" {
			f.app.cancelRuntimePreservingFailure(agent.CancelRun{RunID: runID, Reason: "terminal stream failed"})
		}
	})
}

func (f *streamFollower) postOpenFailure(err error) {
	if errors.Is(err, context.Canceled) || f.ctx.Err() != nil {
		return
	}
	_ = post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			return
		}
		if f.opening.rejected != nil {
			err = f.opening.rejected(err)
			if err == nil {
				return
			}
		}
		f.app.fail(err)
	})
}

func post(ctx context.Context, dispatcher program.Dispatcher, fn func()) error {
	finished := make(chan struct{})
	var claimed atomic.Bool
	dispatcher.Post(func() {
		if claimed.CompareAndSwap(false, true) {
			fn()
		}
		close(finished)
	})
	abort := func(err error) error {
		if claimed.CompareAndSwap(false, true) {
			return err
		}
		<-finished
		return err
	}
	select {
	case <-finished:
		return nil
	case <-ctx.Done():
		return abort(context.Cause(ctx))
	case <-dispatcher.Done():
		return abort(program.ErrStopped)
	}
}

func (a *app) apply(event agent.RunEvent) error {
	result, err := a.conversation.ApplyRunEvent(event)
	if err != nil {
		return fmt.Errorf("apply runtime event %s: %w", event.EventID, err)
	}
	if !result.Applied {
		return nil
	}
	if err := a.transcript.ApplyRunEvent(event, a.registry); err != nil {
		return err
	}
	a.applyPresentationEvent(event)
	a.transcript.DiscardExcess()
	a.syncAnimation()
	return nil
}

func (a *app) applyPresentationEvent(envelope agent.RunEvent) {
	switch event := envelope.Event.(type) {
	case agent.SegmentStarted:
		if event.Run.Lineage.IsRoot() {
			if settled := a.settleQueuedDispatch(); settled {
				a.openingRunID = ""
				a.status.active("working")
			} else if a.queuedDispatchCanceling() {
				a.status.active("canceling")
			} else {
				a.status.note("working · retrying local settlement")
			}
			a.executionClock.start(event.Run.Usage.Duration, time.Now())
		}
	case agent.BlockStarted:
		a.noteBlockStarted(event.Block)
	case agent.BlockCompleted:
		if event.Block.Kind == agent.BlockTool {
			a.status.active("working")
		}
	case agent.PlanChanged:
		a.activity.Set(a.conversation.Plan())
	case agent.RunProgress:
		if envelope.RunID == a.conversation.RunID() {
			a.header.SetUsage(a.conversation.Usage())
			a.status.progress(event)
		} else if strings.TrimSpace(event.Activity) != "" {
			a.status.active("subagent · " + event.Activity)
		}
	case agent.RunInterrupted:
		if a.conversation.Phase() == agent.ConversationWaiting {
			a.openInteractions(a.conversation.Interactions())
			a.header.SetUsage(a.conversation.Usage())
			a.status.note("waiting for your answers")
		}
	case agent.RunSuspended:
		if a.conversation.Phase() == agent.ConversationWaiting {
			a.openInteractions(a.conversation.Interactions())
			a.header.SetUsage(a.conversation.Usage())
			a.status.note("waiting for your answers")
		}
	case agent.RunFinished:
		if envelope.RunID == a.conversation.RunID() {
			a.noteRunFinished()
		}
	case agent.BlockDelta, agent.ToolArgumentsDelta, agent.CustomEvent:
	default:
	}
}

func (a *app) noteBlockStarted(block agent.Block) {
	if block.Kind == agent.BlockTool && block.Tool != nil {
		label := strings.TrimSpace(block.Tool.Summary)
		if label == "" {
			label = "using " + toolLabel(*block.Tool)
		}
		a.status.active(label)
	}
}

func (a *app) noteRunFinished() {
	a.status.note("finishing run")
	a.header.SetUsage(a.conversation.Usage())
}

func (a *app) finishFollowing() {
	a.following = false
	if a.sessionInvalidated {
		a.refreshInvalidatedSession(true)
		return
	}
	if a.conversation.Phase() != agent.ConversationIdle || a.conversation.Outcome().Status == "" {
		return
	}
	a.status.settled(a.conversation.Outcome(), a.conversation.Usage())
	a.prompt.SetBusy(false)
	settled := a.settleQueuedDispatch()
	if settled {
		a.openingRunID = ""
	} else if a.queuedDispatchCanceling() || a.pendingCancel != nil {
		a.status.note("canceling")
	} else {
		a.status.note("run complete · retrying local settlement")
	}
	if settled && a.drainQueue() {
		return
	}
	a.raiseAttention(outcomeAttention(a.conversation.Outcome()))
}

func outcomeNotification(outcome agent.Outcome) string {
	switch outcome.Status {
	case agent.OutcomeCompleted:
		return "lyra run completed"
	case agent.OutcomeCanceled:
		return "lyra run canceled"
	case agent.OutcomeTimedOut, agent.OutcomeMaxSteps, agent.OutcomeMaxBudget:
		return "lyra run stopped: " + string(outcome.Status)
	case agent.OutcomeFailed, agent.OutcomeLost:
		return "lyra run failed"
	default:
		return ""
	}
}

func (a *app) fail(err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	a.following = false
	a.dismissInteractionProjection()
	a.conversation.Failed(err)
	a.transcript.settleLive(a.conversation.Outcome())
	a.transcript.Append(presentError(a.transcript.theme, err.Error()))
	a.status.settled(a.conversation.Outcome(), a.conversation.Usage())
	a.header.SetUsage(a.conversation.Usage())
	a.prompt.SetBusy(false)
	a.syncAnimation()
	a.raiseAttention(failureAttention())
}

func (a *app) cancel() {
	if a.approval != nil {
		a.answerApproval(approvalDenyOnce)
		return
	}
	if a.questionnaire != nil {
		a.finishQuestionnaire(true)
		return
	}
	if a.pendingCancel != nil {
		a.status.doing = "retrying cancellation"
		a.requestRuntimeCancellation(a.pendingCancel.request, a.pendingCancel.policy)
		return
	}
	if !a.conversation.Busy() && !a.following {
		return
	}
	a.status.doing = "canceling"
	runID := a.conversation.RunID()
	if runID == "" {
		pending, staged, err := a.stageOpeningCancellation()
		if err != nil {
			a.message("could not preserve cancellation of unconfirmed run: " + err.Error())
			return
		}
		if !staged {
			return
		}
		a.dropStream()
		if err := a.conversation.CancelStarting(); err != nil {
			a.fail(err)
			return
		}
		a.prompt.SetBusy(false)
		a.status.note("canceled · reconciling runtime delivery")
		a.syncAnimation()
		a.reconcileCanceledStart(pending)
		return
	}
	a.cancelRuntime(agent.CancelRun{RunID: runID, Reason: "canceled by the terminal user"})
}

func (a *app) stageOpeningCancellation() (workbench.PendingRun, bool, error) {
	entry, ok := a.queue.Dispatching(a.session.ID)
	if !ok {
		return workbench.PendingRun{}, false, nil
	}
	if entry.CommandID == "" {
		return workbench.PendingRun{}, false, errors.New("dispatching queue entry is no longer available")
	}
	if a.workbench == nil {
		return workbench.PendingRun{}, false, errors.New("CLI workbench is unavailable")
	}
	if _, err := a.workbench.MarkPendingRunCanceling(
		a.session.ID, entry.CommandID, commandReplayGuard(a.runtimeProfile),
	); err != nil {
		return workbench.PendingRun{}, false, err
	}
	pending, ok := pendingRunByCommandID(a.workbench.PendingRuns(a.session.ID), entry.CommandID)
	if !ok {
		return workbench.PendingRun{}, false, errors.New("canceling run start disappeared from the durable outbox")
	}
	return pending, true, nil
}

func (a *app) reconcileCanceledStart(pending workbench.PendingRun) {
	dispatcher := a.loop.Dispatcher()
	a.operations.GoSession(pendingRunRecoveryOperation, false, func(ctx context.Context, lease operationLease) {
		opened, err := openStartRunWithBackoff(ctx, a.runtime, pending.Command, runtimeRecoveryBackoff)
		if context.Cause(ctx) != nil {
			return
		}
		_ = post(ctx, dispatcher, func() {
			if !a.operations.Current(lease) || a.closed || a.session.ID != pending.Command.SessionID ||
				!a.operations.Release(lease) {
				return
			}
			opened, accepted := observedSegmentStream(opened, err)
			if !accepted {
				if mutation.OutcomeUnknown(err) {
					a.fail(fmt.Errorf("reconcile canceled start: %w", err))
					return
				}
				if retireErr := a.retireCanceledStart(pending); retireErr != nil {
					a.fail(errors.Join(err, retireErr))
				}
				return
			}
			validationErr := opened.ValidateStart()
			if strings.TrimSpace(opened.RunID) == "" {
				a.fail(errors.Join(
					errors.New("reconcile canceled start: accepted receipt has no run identity"),
					err,
					validationErr,
				))
				return
			}
			if receiptErr := errors.Join(err, validationErr); receiptErr != nil {
				a.message("runtime returned an invalid start receipt; canceling accepted run: " + receiptErr.Error())
			}
			a.requestRuntimeCancellation(agent.CancelRun{
				CommandID: pending.CancelCommandID,
				RunID:     opened.RunID,
				Reason:    "canceled while start delivery was unconfirmed",
			}, preserveProjectionAndReportCanceled)
		})
	})
}

func openStartRunWithBackoff(
	ctx context.Context,
	runtime agent.RunLifecycle,
	command agent.StartRun,
	backoff retry.Backoff,
) (agent.SegmentStream, error) {
	return mutation.Confirm(ctx, backoff, func(ctx context.Context) (agent.SegmentStream, error) {
		return runtime.StartRun(ctx, command)
	})
}

func observedSegmentStream(stream agent.SegmentStream, err error) (agent.SegmentStream, bool) {
	if err == nil {
		return stream, true
	}
	receipt, accepted := agent.AcceptedMutationReceipt(err)
	return receipt, accepted
}

func (a *app) retireCanceledStart(pending workbench.PendingRun) error {
	if err := a.retireQueuedCommand(pending.Command.SessionID, pending.Command.CommandID); err != nil {
		return fmt.Errorf("retire canceled start: %w", err)
	}
	a.status.note("canceled")
	a.drainQueue()
	return nil
}

func (a *app) activeCancellation() (agent.CancelRun, bool) {
	if runID := a.conversation.RunID(); runID != "" && a.conversation.Busy() {
		return agent.CancelRun{RunID: runID, Reason: "terminal closed"}, true
	}
	if a.openingRunID != "" && a.conversation.Busy() {
		return agent.CancelRun{RunID: a.openingRunID, Reason: "terminal closed"}, true
	}
	return agent.CancelRun{}, false
}

func (a *app) cancelRuntime(target agent.CancelRun) {
	a.requestRuntimeCancellation(target, applyRuntimeSettlement)
}

// cancelRuntimePreservingFailure stops a run whose event stream has already been
// rejected locally. The control response is authoritative about runtime cleanup,
// but it must not replace the projection failure that made the stream untrustworthy.
func (a *app) cancelRuntimePreservingFailure(target agent.CancelRun) {
	a.requestRuntimeCancellation(target, preserveProjectionFailure)
}

type cancellationResultPolicy uint8

const (
	applyRuntimeSettlement cancellationResultPolicy = iota
	preserveProjectionFailure
	preserveProjectionAndReportCanceled
)

type pendingCancellation struct {
	request          agent.CancelRun
	openingCommandID agent.CommandID
	policy           cancellationResultPolicy
}

func (a *app) requestRuntimeCancellation(target agent.CancelRun, policy cancellationResultPolicy) {
	if target.CommandID == "" {
		commandID, err := agent.NewCommandID()
		if err != nil {
			a.message("could not prepare run cancellation: " + err.Error())
			return
		}
		target.CommandID = commandID
	}
	pending := pendingCancellation{
		request: target, openingCommandID: a.openingCommandForRun(target.RunID), policy: policy,
	}
	if pending.openingCommandID == "" && a.pendingCancel != nil && a.pendingCancel.request.RunID == target.RunID {
		pending.openingCommandID = a.pendingCancel.openingCommandID
	}
	if pending.openingCommandID == "" && a.workbench != nil {
		outbox := a.workbench.PendingRuns(a.session.ID)
		if len(outbox) > 0 && outbox[0].State == workbench.PendingRunCanceling &&
			outbox[0].CancelCommandID == target.CommandID {
			pending.openingCommandID = outbox[0].Command.CommandID
		}
	}
	a.pendingCancel = &pending
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(cancelRunOperation, true, func(ownerCtx context.Context, lease operationLease) {
		settled, err := a.cancelRootRun(ownerCtx, target)
		_ = post(ownerCtx, dispatcher, func() {
			a.handleRuntimeCancellation(lease, pending, settled, err)
		})
	})
}

// cancelRootRun makes cancellation idempotent at the terminal boundary. A run
// may finish between the user's gesture and the control request; in that case
// the durable run projection is the successful settlement of the same intent.
func (a *app) cancelRootRun(ctx context.Context, target agent.CancelRun) (agent.Run, error) {
	result, err := mutation.Confirm(ctx, runtimeRecoveryBackoff, func(ctx context.Context) (agent.RunCancellation, error) {
		attemptCtx, cancel := context.WithTimeout(ctx, runtimeControlTimeout)
		defer cancel()
		return a.runtime.CancelRun(attemptCtx, target)
	})
	if err == nil {
		if err := result.ValidateTarget(target.RunID); err != nil {
			return agent.Run{}, fmt.Errorf("cancel run: %w", err)
		}
		return result.Root, nil
	}
	if !errors.Is(err, agent.ErrRunFinished) {
		return agent.Run{}, err
	}
	settled, readErr := a.runtime.GetRun(ctx, target.RunID)
	if readErr != nil {
		return agent.Run{}, fmt.Errorf("read run after cancellation race: %w", readErr)
	}
	if validateErr := settled.Validate(); validateErr != nil {
		return agent.Run{}, fmt.Errorf("validate run after cancellation race: %w", validateErr)
	}
	if settled.ID != target.RunID || !settled.Lineage.IsRoot() || settled.Status != agent.RunStatusFinished {
		return agent.Run{}, fmt.Errorf("cancellation race returned non-terminal root run %s", settled.ID)
	}
	return settled, nil
}

func (a *app) handleRuntimeCancellation(
	lease operationLease,
	pending pendingCancellation,
	settled agent.Run,
	err error,
) {
	if !a.operations.Current(lease) || a.closed {
		return
	}
	if err != nil {
		// A rejected control request says nothing about the run itself. Keep the
		// conversation and cancellation target intact so the user can retry while
		// the runtime remains the source of truth for eventual settlement.
		a.message("could not cancel run: " + err.Error())
		return
	}
	if retireErr := a.retireCanceledRuntimeOwnership(pending.request.RunID, pending.openingCommandID); retireErr != nil {
		failure := fmt.Errorf("retire canceled runtime ownership: %w", retireErr)
		a.reportWorkbenchIssue(workbenchCancellationOwnership, failure)
		a.message("could not " + failure.Error())
		a.retryCanceledRuntimeOwnership(pending.request.RunID, pending.openingCommandID)
	} else {
		a.reportWorkbenchIssue(workbenchCancellationOwnership, nil)
	}
	if current := a.pendingCancel; current != nil && current.request.CommandID == pending.request.CommandID {
		a.pendingCancel = nil
	}
	a.openingRunID = ""
	a.dropStream()
	if pending.policy == preserveProjectionFailure {
		a.prompt.SetBusy(false)
		a.syncAnimation()
		a.drainQueue()
		return
	}
	if pending.policy == preserveProjectionAndReportCanceled {
		a.status.note("canceled")
		a.prompt.SetBusy(false)
		a.syncAnimation()
		a.drainQueue()
		return
	}
	if err := a.conversation.SettleRun(settled); err != nil {
		a.fail(err)
		return
	}
	a.transcript.settleLive(settled.Outcome)
	a.status.settled(settled.Outcome, settled.Usage)
	a.header.SetUsage(settled.Usage)
	a.prompt.SetBusy(false)
	a.syncAnimation()
	if a.sessionInvalidated {
		a.refreshInvalidatedSession(true)
		return
	}
	a.drainQueue()
}

func (a *app) openingCommandForRun(runID string) agent.CommandID {
	if runID == "" || runID != a.openingRunID {
		return ""
	}
	entry, ok := a.queue.Dispatching(a.session.ID)
	if !ok {
		return ""
	}
	return entry.CommandID
}

func (a *app) retireCanceledRuntimeOwnership(runID string, openingCommandID agent.CommandID) error {
	var err error
	if a.workbench != nil {
		if pending, ok := a.workbench.PendingResume(a.session.ID); ok && pending.Command.RunID == runID {
			err = errors.Join(err, a.workbench.AcknowledgePendingResume(a.session.ID, pending.Command.CommandID))
		}
	}
	if openingCommandID != "" {
		err = errors.Join(err, a.retireQueuedCommand(a.session.ID, openingCommandID))
	}
	return err
}

func (a *app) cancelRuntimeNow(ownerCtx context.Context, target agent.CancelRun) error {
	if target.CommandID == "" {
		commandID, err := agent.NewCommandID()
		if err != nil {
			return fmt.Errorf("prepare terminal-close cancellation: %w", err)
		}
		target.CommandID = commandID
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	result, err := mutation.Confirm(ctx, runtimeRecoveryBackoff, func(ctx context.Context) (agent.RunCancellation, error) {
		return a.runtime.CancelRun(ctx, target)
	})
	if errors.Is(err, agent.ErrRunFinished) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := result.ValidateTarget(target.RunID); err != nil {
		return fmt.Errorf("validate terminal-close cancellation: %w", err)
	}
	return nil
}

func (a *app) cancelOpeningRunNow(ownerCtx context.Context, pending workbench.PendingRun) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	opened, err := openStartRunWithBackoff(ctx, a.runtime, pending.Command, runtimeRecoveryBackoff)
	opened, accepted := observedSegmentStream(opened, err)
	if !accepted {
		if !mutation.OutcomeUnknown(err) {
			return a.retireCanceledStart(pending)
		}
		return fmt.Errorf("reconcile run start during terminal close: %w", err)
	}
	validationErr := opened.ValidateStart()
	if strings.TrimSpace(opened.RunID) == "" {
		return errors.Join(
			errors.New("reconcile run start during terminal close: accepted receipt has no run identity"),
			err,
			validationErr,
		)
	}
	cancelErr := a.cancelRuntimeNow(ctx, agent.CancelRun{
		CommandID: pending.CancelCommandID,
		RunID:     opened.RunID,
		Reason:    "terminal closed during start delivery",
	})
	if cancelErr != nil {
		return errors.Join(err, validationErr, fmt.Errorf("cancel run opened during terminal close: %w", cancelErr))
	}
	return a.retireCanceledStart(pending)
}

func (a *app) dropStream() {
	a.streamSeq++
	a.operations.Cancel(streamOperation)
	a.following = false
}

func (a *app) syncAnimation() {
	running := a.conversation.Phase() == agent.ConversationRunning
	switch {
	case running && a.stopClock == nil:
		a.stopClock = a.loop.Every(animationInterval, func() {
			a.status.tick(a.executionClock.elapsed(time.Now()))
		})
	case !running && a.stopClock != nil:
		a.stopClock()
		a.stopClock = nil
	}
	state := term.Progress{}
	if running {
		state.State = term.ProgressIndeterminate
	}
	a.loop.Session().SetProgress(state)
}
