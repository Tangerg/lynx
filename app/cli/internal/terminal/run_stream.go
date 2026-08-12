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
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/runrecovery"
)

const (
	animationInterval     = 100 * time.Millisecond
	runtimeControlTimeout = 5 * time.Second
)

type activeDurationClock struct {
	carried          time.Duration
	segmentStartedAt time.Time
}

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

func (a *app) startRun(message agent.Message, status string) bool {
	if err := a.conversation.Starting(); err != nil {
		a.fail(err)
		return false
	}
	a.transcript.Follow()
	a.activity.Reset()
	a.header.SetUsage(agent.Usage{})
	a.prompt.SetBusy(true)
	a.status.active(status)
	a.executionClock.start(0, time.Now())
	a.syncAnimation()
	input := agent.StartRun{SessionID: a.session.ID, Message: message.Clone(), Options: a.options}
	a.follow(func(ctx context.Context) (agent.SegmentStream, error) {
		opened, err := a.runtime.StartRun(ctx, input)
		if err != nil {
			return agent.SegmentStream{}, err
		}
		if err := opened.ValidateStart(); err != nil {
			return agent.SegmentStream{}, fmt.Errorf("start run: %w", err)
		}
		return opened, nil
	})
	return true
}

func (a *app) follow(open func(context.Context) (agent.SegmentStream, error)) {
	a.dropStream()
	sequence := a.streamSeq
	a.following = true
	a.operations.Go(streamOperation, true, func(ctx context.Context, _ operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: a.loop.Dispatcher(), sequence: sequence,
			open: open, applyEvent: a.apply,
			policy: reconnect.New(a.settings.UI.ReconnectAttempts),
		}
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
		recovered, err := runrecovery.AttachSession(ctx, a.runtime, a.session.ID)
		if err != nil {
			follower.postFailure("", fmt.Errorf("restore active session: %w", err))
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
	failures   int
	checkpoint string
}

// eventApplicationError marks a runtime event that reached the terminal but could
// not be folded into its conversation projection. Reconnecting cannot repair an
// invalid or conflicting event; only transport failures are eligible for replay.
type eventApplicationError struct{ err error }

func (e *eventApplicationError) Error() string { return e.err.Error() }
func (e *eventApplicationError) Unwrap() error { return e.err }

func (f *streamFollower) run() {
	current, err := f.open(f.ctx)
	if err != nil {
		f.postFailure("", err)
		return
	}
	f.runStream(current)
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
		if !f.waitBeforeReconnect(runID, cause) {
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

func (f *streamFollower) waitBeforeReconnect(runID string, cause error) bool {
	f.failures++
	delay, retry := f.policy.Next(f.failures, cause)
	if !retry {
		f.postFailure(runID, cause)
		return false
	}
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq == f.sequence {
			f.app.status.note(fmt.Sprintf("reconnecting %d/%d", f.failures, f.policy.Attempts))
			f.app.syncAnimation()
		}
	})
	return err == nil && reconnect.Wait(f.ctx, delay) == nil
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
			a.commitQueuedDispatch()
			a.executionClock.start(event.Run.Usage.Duration, time.Now())
			a.status.active("working")
		}
	case agent.BlockStarted:
		a.noteBlockStarted(event.Block)
	case agent.BlockCompleted:
		if event.Block.Kind == agent.BlockTool {
			a.status.active("working")
		}
	case agent.PlanChanged:
		a.activity.Set(a.conversation.Plan())
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
	case agent.BlockDelta:
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
	a.pendingCancel = nil
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
	if a.drainQueue() {
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
	a.releaseQueuedDispatch()
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
		a.answerApproval("deny")
		return
	}
	if a.questionnaire != nil {
		a.finishQuestionnaire(true)
		return
	}
	if a.pendingCancel != nil {
		a.status.doing = "retrying cancellation"
		a.cancelRuntime(*a.pendingCancel)
		return
	}
	if !a.conversation.Busy() && !a.following {
		return
	}
	a.status.doing = "canceling"
	runID := a.conversation.RunID()
	if runID == "" {
		a.discardQueuedDispatch()
		a.dropStream()
		if err := a.conversation.CancelStarting(); err != nil {
			a.fail(err)
			return
		}
		a.status.settled(a.conversation.Outcome(), a.conversation.Usage())
		a.header.SetUsage(a.conversation.Usage())
		a.prompt.SetBusy(false)
		a.syncAnimation()
		return
	}
	a.cancelRuntime(agent.CancelRun{RunID: runID, Reason: "canceled by the terminal user"})
}

func (a *app) activeCancellation() (agent.CancelRun, bool) {
	if runID := a.conversation.RunID(); runID != "" && a.conversation.Busy() {
		return agent.CancelRun{RunID: runID, Reason: "terminal closed"}, true
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
)

func (a *app) requestRuntimeCancellation(target agent.CancelRun, policy cancellationResultPolicy) {
	targetCopy := target
	a.pendingCancel = &targetCopy
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(cancelRunOperation, true, func(ownerCtx context.Context, lease operationLease) {
		ctx, cancel := context.WithTimeout(ownerCtx, runtimeControlTimeout)
		defer cancel()
		settled, err := a.runtime.CancelRun(ctx, target)
		_ = post(ctx, dispatcher, func() {
			a.handleRuntimeCancellation(lease, settled, err, policy)
		})
	})
}

func (a *app) handleRuntimeCancellation(lease operationLease, settled agent.Run, err error, policy cancellationResultPolicy) {
	if !a.operations.Current(lease) || a.closed {
		return
	}
	if err != nil {
		if policy != preserveProjectionFailure {
			a.fail(err)
		}
		return
	}
	a.pendingCancel = nil
	a.dropStream()
	if policy == preserveProjectionFailure {
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

func (a *app) cancelRuntimeNow(ownerCtx context.Context, target agent.CancelRun) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	_, _ = a.runtime.CancelRun(ctx, target)
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
