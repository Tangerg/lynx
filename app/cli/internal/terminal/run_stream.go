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
	"github.com/Tangerg/lynx/app/cli/internal/requestid"
)

const (
	animationInterval     = 100 * time.Millisecond
	runtimeControlTimeout = 5 * time.Second
)

func (a *app) startRun(message agent.Message, status string) bool {
	requestID, err := requestid.New()
	if err != nil {
		a.fail(err)
		return false
	}
	if err := a.conversation.Starting(); err != nil {
		a.fail(err)
		return false
	}
	a.transcript.Follow()
	a.activity.Reset()
	a.header.SetUsage(agent.Usage{})
	a.prompt.SetBusy(true)
	a.status.active(status)
	a.started = time.Now()
	a.startRequest = requestID
	a.syncAnimation()
	input := agent.StartRun{
		RequestID: requestID,
		SessionID: a.session.ID,
		Message:   message.Clone(),
		Options:   a.options,
	}
	a.follow(func(ctx context.Context) (runSubscription, error) {
		run, err := a.runtime.StartRun(ctx, input)
		if err != nil {
			return runSubscription{}, err
		}
		if err := run.Validate(); err != nil {
			return runSubscription{}, fmt.Errorf("start run response: %w", err)
		}
		if run.SessionID != input.SessionID {
			return runSubscription{}, fmt.Errorf("start run response: run belongs to session %s, want %s", run.SessionID, input.SessionID)
		}
		return runSubscription{runID: run.ID, after: run.StartedAfter}, nil
	})
	return true
}

type runSubscription struct {
	runID string
	after agent.Cursor
}

func (a *app) follow(open func(context.Context) (runSubscription, error)) {
	a.dropStream()
	sequence := a.streamSeq
	a.following = true
	a.operations.Go(streamOperation, true, func(ctx context.Context, _ operationLease) {
		follower := streamFollower{
			app: a, ctx: ctx, dispatcher: a.loop.Dispatcher(), sequence: sequence,
			open: open, applyEnvelope: a.apply,
			policy: reconnect.New(a.settings.UI.ReconnectAttempts),
		}
		follower.run()
	})
}

type streamFollower struct {
	app           *app
	ctx           context.Context
	dispatcher    program.Dispatcher
	sequence      uint64
	open          func(context.Context) (runSubscription, error)
	applyEnvelope func(agent.Envelope) error
	policy        reconnect.Policy
	after         agent.Cursor
	failures      int
}

func (f *streamFollower) run() {
	opened, ok := f.openSubscription()
	if !ok {
		return
	}
	f.after = opened.after
	f.failures = 0
	for f.followOnce(opened.runID) {
	}
}

func (f *streamFollower) openSubscription() (runSubscription, bool) {
	for {
		opened, err := f.open(f.ctx)
		if err == nil {
			return opened, true
		}
		if context.Cause(f.ctx) != nil || !f.retry(err, false) {
			return runSubscription{}, false
		}
	}
}

func (f *streamFollower) followOnce(runID string) bool {
	before := f.after
	stream, followErr := f.app.runtime.FollowRun(f.ctx, agent.FollowRun{RunID: runID, After: f.after})
	if followErr == nil && stream == nil {
		followErr = errors.New("runtime returned a nil event stream")
	}
	active := true
	if followErr == nil {
		active, followErr = f.consume(stream)
	}
	if !active {
		return false
	}
	snapshot, err := f.snapshot()
	if err != nil || !snapshot.active {
		return false
	}
	f.after = snapshot.after
	if snapshot.phase != agent.ConversationRunning {
		f.finish()
		return false
	}
	if followErr == nil {
		followErr = fmt.Errorf("%w: runtime stream ended without parking or finishing the run", agent.ErrDisconnected)
	}
	if context.Cause(f.ctx) != nil {
		return false
	}
	return f.retry(followErr, f.after > before)
}

func (f *streamFollower) consume(stream agent.RunStream) (bool, error) {
	for envelope, streamErr := range stream {
		if streamErr != nil {
			return true, streamErr
		}
		active, err := f.apply(envelope)
		if !active || err != nil {
			return active, err
		}
	}
	return true, nil
}

func (f *streamFollower) apply(envelope agent.Envelope) (bool, error) {
	active := true
	var applyErr error
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			active = false
			return
		}
		applyErr = f.applyEnvelope(envelope)
		f.after = f.app.conversation.Cursor()
	})
	return active, errors.Join(err, applyErr)
}

type followSnapshot struct {
	active bool
	after  agent.Cursor
	phase  agent.ConversationPhase
}

func (f *streamFollower) snapshot() (followSnapshot, error) {
	snapshot := followSnapshot{active: true}
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			snapshot.active = false
			return
		}
		snapshot.after = f.app.conversation.Cursor()
		snapshot.phase = f.app.conversation.Phase()
	})
	return snapshot, err
}

func (f *streamFollower) retry(cause error, progressed bool) bool {
	if progressed {
		f.failures = 0
	}
	f.failures++
	delay, retry := f.policy.Next(f.failures, cause)
	if !retry {
		f.app.postStreamFailure(f.ctx, f.dispatcher, f.sequence, cause)
		return false
	}
	if err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq == f.sequence {
			f.app.status.note(fmt.Sprintf("reconnecting %d/%d", f.failures, f.policy.Attempts))
			f.app.syncAnimation()
		}
	}); err != nil {
		return false
	}
	return reconnect.Wait(f.ctx, delay) == nil
}

func (f *streamFollower) finish() {
	_ = post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq == f.sequence {
			f.app.finishFollowing()
		}
	})
}

func (a *app) postStreamFailure(ctx context.Context, dispatcher program.Dispatcher, sequence uint64, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		return
	}
	_ = post(ctx, dispatcher, func() {
		if a.streamSeq == sequence {
			target, cancel := a.activeCancellation()
			a.fail(err)
			if cancel {
				a.cancelRuntime(target)
			}
		}
	})
}

func (a *app) activeCancellation() (agent.CancelRun, bool) {
	if runID := a.conversation.RunID(); runID != "" {
		return agent.CancelRun{RunID: runID}, true
	}
	if a.startRequest != "" {
		return agent.CancelRun{SessionID: a.session.ID, RequestID: a.startRequest}, true
	}
	return agent.CancelRun{}, false
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
		// The UI callback already owns the request. Join it before returning so
		// stack-owned reply values cannot outlive the caller that reads them.
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

func (a *app) apply(envelope agent.Envelope) error {
	result, err := a.conversation.ApplyEnvelope(envelope)
	if err != nil {
		return fmt.Errorf("apply runtime event %T at cursor %d: %w", envelope.Event, envelope.Cursor, err)
	}
	if !result.Applied {
		return nil
	}
	if err := a.transcript.Apply(envelope.Event, a.registry); err != nil {
		return err
	}
	a.applyPresentationEvent(envelope.Event)
	a.transcript.Retain(a.loop)
	a.syncAnimation()
	return nil
}

func (a *app) applyPresentationEvent(event agent.Event) {
	switch event := event.(type) {
	case agent.RunStarted:
		a.commitQueuedDispatch()
		a.startRequest = ""
		a.status.active("working")
	case agent.BlockStarted:
		a.noteBlockStarted(event.Block)
	case agent.BlockCompleted:
		if event.Block.Kind == agent.BlockTool {
			a.status.active("working")
		}
	case agent.PlanChanged:
		a.activity.Set(a.conversation.Plan())
	case agent.RunInterrupted:
		a.openInteraction(a.conversation.Interaction())
		a.status.note("waiting for your answer")
	case agent.RunFinished:
		a.noteRunFinished()
	case agent.BlockDelta, agent.RunResumed:
		// Their visible state is already represented by the transcript.
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
	a.startRequest = ""
	a.pendingCancel = nil
	a.status.note("finishing run")
	a.header.SetUsage(a.conversation.Usage())
	// The outcome is authoritative, but the stream owner may still be resolving an
	// ambiguous idempotent control call. It releases the prompt in finishFollowing
	// only after that transport lifecycle has actually settled.
}

func (a *app) finishFollowing() {
	a.following = false
	if a.conversation.Phase() != agent.ConversationIdle || a.conversation.Outcome().Status == "" {
		return
	}
	a.status.settled(a.conversation.Outcome(), a.conversation.Usage())
	a.prompt.SetBusy(false)
	if a.drainQueue() {
		return
	}
	if a.settings.UI.Notifications {
		a.loop.Session().Notify(outcomeNotification(a.conversation.Outcome()))
	}
}

func outcomeNotification(outcome agent.Outcome) string {
	switch outcome.Status {
	case agent.OutcomeCompleted:
		return "lyra run completed"
	case agent.OutcomeCanceled:
		return "lyra run canceled"
	case agent.OutcomeFailed:
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
	a.startRequest = ""
	a.releaseQueuedDispatch()
	a.conversation.Failed(err)
	a.transcript.settleLive(a.conversation.Outcome())
	a.transcript.Append(presentError(a.transcript.theme, err.Error()))
	a.status.settled(a.conversation.Outcome(), a.conversation.Usage())
	a.header.SetUsage(a.conversation.Usage())
	a.prompt.SetBusy(false)
	a.syncAnimation()
}

func (a *app) cancel() {
	if a.approval != nil {
		a.answerApproval("deny")
		return
	}
	if a.question != nil {
		a.answerQuestion(true)
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
		requestID := a.startRequest
		a.discardQueuedDispatch()
		a.dropStream()
		a.following = false
		a.startRequest = ""
		if err := a.conversation.CancelStarting(); err != nil {
			a.fail(err)
			return
		}
		a.status.settled(a.conversation.Outcome(), a.conversation.Usage())
		a.header.SetUsage(a.conversation.Usage())
		a.prompt.SetBusy(false)
		a.syncAnimation()
		if requestID != "" {
			a.cancelRuntime(agent.CancelRun{SessionID: a.session.ID, RequestID: requestID})
		}
		return
	}
	a.cancelRuntime(agent.CancelRun{RunID: runID})
}

func (a *app) cancelRuntime(target agent.CancelRun) {
	targetCopy := target
	a.pendingCancel = &targetCopy
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(cancelRunOperation, true, func(ownerCtx context.Context, lease operationLease) {
		ctx, cancel := context.WithTimeout(ownerCtx, runtimeControlTimeout)
		defer cancel()
		policy := reconnect.New(a.settings.UI.ReconnectAttempts)
		err := reconnect.Control(ctx, policy, func() error { return a.runtime.CancelRun(ctx, target) })
		_ = post(ctx, dispatcher, func() {
			if a.operations.Current(lease) && !a.closed {
				if err != nil {
					a.fail(err)
					return
				}
				a.pendingCancel = nil
				a.drainQueue()
			}
		})
	})
}

func (a *app) cancelRuntimeNow(ownerCtx context.Context, target agent.CancelRun) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), runtimeControlTimeout)
	defer cancel()
	policy := reconnect.New(a.settings.UI.ReconnectAttempts)
	_ = reconnect.Control(ctx, policy, func() error { return a.runtime.CancelRun(ctx, target) })
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
			a.status.tick(time.Since(a.started))
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
