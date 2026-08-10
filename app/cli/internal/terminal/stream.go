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

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/requestid"
)

const (
	animationRate  = 100 * time.Millisecond
	controlTimeout = 5 * time.Second
)

func (a *app) startRun(message client.Message, status string) bool {
	requestID, err := requestid.New()
	if err != nil {
		a.fail(err)
		return false
	}
	if err := a.state.Starting(); err != nil {
		a.fail(err)
		return false
	}
	a.transcript.Follow()
	a.activity.Reset()
	a.header.SetUsage(client.Usage{})
	a.prompt.SetBusy(true)
	a.status.active(status)
	a.started = time.Now()
	a.startRequest = requestID
	a.syncAnimation()
	input := client.StartRun{
		RequestID: requestID,
		SessionID: a.session.ID,
		Message:   cloneMessage(message),
		Options:   a.options,
	}
	a.follow(func(ctx context.Context) (subscription, error) {
		run, err := a.runtime.StartRun(ctx, input)
		if err != nil {
			return subscription{}, err
		}
		if err := run.Validate(); err != nil {
			return subscription{}, fmt.Errorf("start run response: %w", err)
		}
		if run.SessionID != input.SessionID {
			return subscription{}, fmt.Errorf("start run response: run belongs to session %s, want %s", run.SessionID, input.SessionID)
		}
		return subscription{runID: run.ID, after: run.StartedAfter}, nil
	})
	return true
}

type subscription struct {
	runID string
	after client.Cursor
}

func (a *app) follow(open func(context.Context) (subscription, error)) {
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
	open          func(context.Context) (subscription, error)
	applyEnvelope func(client.Envelope) error
	policy        reconnect.Policy
	after         client.Cursor
	failures      int
}

func (f *streamFollower) run() {
	subscription, ok := f.openSubscription()
	if !ok {
		return
	}
	f.after = subscription.after
	f.failures = 0
	for f.followOnce(subscription.runID) {
	}
}

func (f *streamFollower) openSubscription() (subscription, bool) {
	for {
		opened, err := f.open(f.ctx)
		if err == nil {
			return opened, true
		}
		if context.Cause(f.ctx) != nil || !f.retry(err, false) {
			return subscription{}, false
		}
	}
}

func (f *streamFollower) followOnce(runID string) bool {
	before := f.after
	stream, followErr := f.app.runtime.FollowRun(f.ctx, client.FollowRun{RunID: runID, After: f.after})
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
	state, err := f.state()
	if err != nil || !state.active {
		return false
	}
	f.after = state.after
	if state.phase != client.Running {
		f.finish()
		return false
	}
	if followErr == nil {
		followErr = fmt.Errorf("%w: runtime stream ended without parking or finishing the run", client.ErrDisconnected)
	}
	if context.Cause(f.ctx) != nil {
		return false
	}
	return f.retry(followErr, f.after > before)
}

func (f *streamFollower) consume(stream client.Stream) (bool, error) {
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

func (f *streamFollower) apply(envelope client.Envelope) (bool, error) {
	active := true
	var applyErr error
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			active = false
			return
		}
		applyErr = f.applyEnvelope(envelope)
		f.after = f.app.state.Cursor()
	})
	return active, errors.Join(err, applyErr)
}

type streamUIState struct {
	active bool
	after  client.Cursor
	phase  client.Phase
}

func (f *streamFollower) state() (streamUIState, error) {
	state := streamUIState{active: true}
	err := post(f.ctx, f.dispatcher, func() {
		if f.app.streamSeq != f.sequence {
			state.active = false
			return
		}
		state.after = f.app.state.Cursor()
		state.phase = f.app.state.Phase()
	})
	return state, err
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

func (a *app) activeCancellation() (client.CancelRun, bool) {
	if runID := a.state.RunID(); runID != "" {
		return client.CancelRun{RunID: runID}, true
	}
	if a.startRequest != "" {
		return client.CancelRun{SessionID: a.session.ID, RequestID: a.startRequest}, true
	}
	return client.CancelRun{}, false
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

func (a *app) apply(envelope client.Envelope) error {
	result, err := a.state.ApplyEnvelope(envelope)
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

func (a *app) applyPresentationEvent(event client.Event) {
	switch event := event.(type) {
	case client.RunStarted:
		a.commitQueuedDispatch()
		a.startRequest = ""
		a.status.active("working")
	case client.BlockStarted:
		a.noteBlockStarted(event.Block)
	case client.BlockCompleted:
		if event.Block.Kind == client.BlockTool {
			a.status.active("working")
		}
	case client.PlanChanged:
		a.activity.Set(a.state.Plan())
	case client.RunInterrupted:
		a.openInteraction(a.state.Interaction())
		a.status.note("waiting for your answer")
	case client.RunFinished:
		a.noteRunFinished()
	case client.BlockDelta, client.RunResumed:
		// Their visible state is already represented by the transcript.
	default:
	}
}

func (a *app) noteBlockStarted(block client.Block) {
	if block.Kind == client.BlockTool && block.Tool != nil {
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
	a.header.SetUsage(a.state.Usage())
	// The outcome is authoritative, but the stream owner may still be resolving an
	// ambiguous idempotent control call. It releases the prompt in finishFollowing
	// only after that transport lifecycle has actually settled.
}

func (a *app) finishFollowing() {
	a.following = false
	if a.state.Phase() != client.Idle || a.state.Outcome().Status == "" {
		return
	}
	a.status.settled(a.state.Outcome(), a.state.Usage())
	a.prompt.SetBusy(false)
	if a.drainQueue() {
		return
	}
	if a.settings.UI.Notifications {
		a.loop.Session().Notify(outcomeNotification(a.state.Outcome()))
	}
}

func outcomeNotification(outcome client.Outcome) string {
	switch outcome.Status {
	case client.OutcomeCompleted:
		return "lyra run completed"
	case client.OutcomeCanceled:
		return "lyra run canceled"
	case client.OutcomeFailed:
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
	a.state.Failed(err)
	a.transcript.settleLive(a.state.Outcome())
	a.transcript.Append(presentError(a.transcript.theme, err.Error()))
	a.status.settled(a.state.Outcome(), a.state.Usage())
	a.header.SetUsage(a.state.Usage())
	a.prompt.SetBusy(false)
	a.syncAnimation()
}

func (a *app) cancel() {
	if a.review != nil {
		a.answerReview("deny")
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
	if !a.state.Busy() && !a.following {
		return
	}
	a.status.doing = "canceling"
	runID := a.state.RunID()
	if runID == "" {
		requestID := a.startRequest
		a.discardQueuedDispatch()
		a.dropStream()
		a.following = false
		a.startRequest = ""
		if err := a.state.CancelStarting(); err != nil {
			a.fail(err)
			return
		}
		a.status.settled(a.state.Outcome(), a.state.Usage())
		a.header.SetUsage(a.state.Usage())
		a.prompt.SetBusy(false)
		a.syncAnimation()
		if requestID != "" {
			a.cancelRuntime(client.CancelRun{SessionID: a.session.ID, RequestID: requestID})
		}
		return
	}
	a.cancelRuntime(client.CancelRun{RunID: runID})
}

func (a *app) cancelRuntime(target client.CancelRun) {
	targetCopy := target
	a.pendingCancel = &targetCopy
	dispatcher := a.loop.Dispatcher()
	a.operations.Go(cancelRunOperation, true, func(ownerCtx context.Context, lease operationLease) {
		ctx, cancel := context.WithTimeout(ownerCtx, controlTimeout)
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

func (a *app) cancelRuntimeNow(ownerCtx context.Context, target client.CancelRun) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ownerCtx), controlTimeout)
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
	running := a.state.Phase() == client.Running
	switch {
	case running && a.stopClock == nil:
		a.stopClock = a.loop.Every(animationRate, func() {
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
