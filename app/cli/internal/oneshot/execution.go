// Package oneshot owns unattended runs that stream to a non-interactive renderer.
package oneshot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/runrecovery"
)

const cancellationTimeout = 5 * time.Second

var acknowledgementBackoff = retry.Backoff{Base: 50 * time.Millisecond, Maximum: time.Second}

type Renderer interface {
	Begin(agent.Run, agent.RunOptions) error
	Render(agent.RunEvent) error
	Reconcile(agent.SessionSnapshot) error
	Close() error
}

type Runtime interface {
	agent.RunLifecycle
	agent.SessionReader
}

type Invocation struct {
	Runtime           Runtime
	Renderer          Renderer
	Start             agent.StartRun
	ApproveAll        bool
	ReconnectAttempts int
	ReplayRetention   time.Duration
}

// Execute drives one stable Run across as many Segments as its interrupts
// require. Ambiguous mutation acknowledgements reuse the same command identity
// only while the runtime's advertised replay retention still owns it.
func Execute(ctx context.Context, invocation Invocation) (runErr error) {
	if invocation.Runtime == nil {
		return errors.New("one-shot run requires a runtime")
	}
	if invocation.Renderer == nil {
		return errors.New("one-shot run requires a renderer")
	}
	if invocation.Start.CommandID == "" {
		invocation.Start.CommandID, runErr = agent.NewCommandID()
		if runErr != nil {
			return runErr
		}
	}
	if err := invocation.Start.Validate(); err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, invocation.Renderer.Close()) }()

	opened, err := openRun(
		ctx, invocation.Runtime, invocation.Start, replayAdmission(invocation.ReplayRetention),
	)
	if err != nil {
		if receipt, accepted := agent.AcceptedMutationReceipt(err); accepted {
			opened = receipt
		} else {
			return err
		}
	}
	cancelOnExit := true
	var watcher *cancellationWatcher
	if opened.RunID != "" {
		watcher = watchCancellation(ctx, invocation.Runtime, opened.RunID, invocation.ReplayRetention)
		defer func() { runErr = errors.Join(runErr, watcher.Finish(cancelOnExit)) }()
	}
	if err != nil {
		return err
	}
	if validateStartErr := opened.ValidateStart(); validateStartErr != nil {
		return fmt.Errorf("start run: %w", validateStartErr)
	}
	run := agent.Run{
		ID: opened.RunID, SessionID: invocation.Start.SessionID,
		Provider: invocation.Start.Options.Provider, Model: invocation.Start.Options.Model,
		Status: agent.RunStatusRunning, ActiveSegmentID: opened.SegmentID, Limits: invocation.Start.Options.Limits,
	}
	if run.Provider == "" {
		// The runtime default is intentionally opaque to the caller. Validation
		// permits the pair to be empty.
		run.Model = ""
	}
	if beginErr := invocation.Renderer.Begin(run, invocation.Start.Options); beginErr != nil {
		return beginErr
	}

	disposition, err := drive(ctx, invocation, opened)
	if disposition.preservesRun() {
		cancelOnExit = false
	}
	return err
}

func openRun(
	ctx context.Context,
	runtime Runtime,
	command agent.StartRun,
	admit mutation.Admission,
) (agent.SegmentStream, error) {
	return mutation.ConfirmAdmitted(
		ctx, acknowledgementBackoff, admit,
		func(ctx context.Context) (agent.SegmentStream, error) { return runtime.StartRun(ctx, command) },
	)
}

func replayAdmission(retention time.Duration) mutation.Admission {
	if retention <= 0 {
		return nil
	}
	return mutation.ReplayUntil(time.Now().UTC().Add(retention), nil)
}

type cancellationWatcher struct {
	exit   chan bool
	result chan error
}

func watchCancellation(
	ctx context.Context,
	runtime agent.RunLifecycle,
	runID string,
	replayRetention time.Duration,
) *cancellationWatcher {
	watcher := &cancellationWatcher{exit: make(chan bool, 1), result: make(chan error, 1)}
	go func() {
		shouldCancel := true
		select {
		case shouldCancel = <-watcher.exit:
		case <-ctx.Done():
		}
		if !shouldCancel {
			watcher.result <- nil
			return
		}
		watcher.result <- cancelAbandonedRun(ctx, runtime, runID, replayRetention)
	}()
	return watcher
}

func (c *cancellationWatcher) Finish(cancelRun bool) error {
	c.exit <- cancelRun
	return <-c.result
}

func cancelAbandonedRun(
	ctx context.Context,
	runtime agent.RunLifecycle,
	runID string,
	replayRetention time.Duration,
) error {
	commandID, err := agent.NewCommandID()
	if err != nil {
		return fmt.Errorf("prepare abandoned run cancellation: %w", err)
	}
	cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancellationTimeout)
	defer cancel()
	result, err := mutation.ConfirmAdmitted(
		cancelCtx, acknowledgementBackoff, replayAdmission(replayRetention),
		func(ctx context.Context) (agent.RunCancellation, error) {
			return runtime.CancelRun(ctx, agent.CancelRun{
				CommandID: commandID, RunID: runID, Reason: "CLI execution ended before the run settled",
			})
		},
	)
	if errors.Is(err, agent.ErrRunFinished) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cancel abandoned run %s: %w", runID, err)
	}
	if err := result.ValidateTarget(runID); err != nil {
		return fmt.Errorf("cancel abandoned run %s: %w", runID, err)
	}
	return nil
}

type disposition uint8

const (
	continuing disposition = iota
	abandoned
	settled
	parked
)

func (d disposition) preservesRun() bool { return d == settled || d == parked }

func drive(ctx context.Context, invocation Invocation, opened agent.SegmentStream) (disposition, error) {
	driver := executionDriver{
		invocation: invocation, openedRunID: opened.RunID,
		conversation: agent.NewConversation(), policy: reconnect.New(invocation.ReconnectAttempts), current: opened,
	}
	return driver.run(ctx)
}

type executionDriver struct {
	invocation   Invocation
	openedRunID  string
	conversation *agent.Conversation
	policy       reconnect.Policy
	current      agent.SegmentStream
	failures     int
}

func (e *executionDriver) run(ctx context.Context) (disposition, error) {
	for {
		followed := consume(e.current.Events, e.conversation, e.invocation.Renderer)
		if followed.outcome != nil {
			return settled, errorForOutcome(*followed.outcome)
		}
		if len(followed.interactions) != 0 {
			if err := e.resume(ctx, followed.interactions, e.current.RunID); err != nil {
				return interactionDisposition(err), err
			}
			continue
		}
		cause := followed.err
		if cause == nil {
			cause = fmt.Errorf("%w: segment stream ended without a terminal event", agent.ErrDisconnected)
		}
		if followed.applied > 0 {
			e.failures = 0
		}
		reconnectDisposition, err := e.reconnect(ctx, cause)
		if reconnectDisposition != continuing {
			return reconnectDisposition, err
		}
	}
}

func (e *executionDriver) resume(ctx context.Context, interactions []agent.Interaction, runID string) error {
	answers, err := unattendedAnswers(interactions, e.invocation.ApproveAll, e.invocation.Start.SessionID)
	if err != nil {
		return err
	}
	commandID, err := agent.NewCommandID()
	if err != nil {
		return err
	}
	command := agent.ResumeRun{CommandID: commandID, RunID: runID, Answers: answers}
	continued, err := mutation.ConfirmAdmitted(
		ctx, acknowledgementBackoff, replayAdmission(e.invocation.ReplayRetention),
		func(ctx context.Context) (agent.SegmentStream, error) {
			return e.invocation.Runtime.ResumeRun(ctx, command)
		},
	)
	if err != nil {
		return err
	}
	if err := validateContinuation(continued, e.openedRunID); err != nil {
		return err
	}
	e.current = continued
	e.failures = 0
	return nil
}

func interactionDisposition(err error) disposition {
	if _, required := errors.AsType[*interactionRequiredError](err); required {
		return parked
	}
	return abandoned
}

func (e *executionDriver) reconnect(ctx context.Context, cause error) (disposition, error) {
	for {
		e.failures++
		delay, shouldRetry := e.policy.Next(e.failures, cause)
		if !shouldRetry {
			return abandoned, cause
		}
		if err := retry.Wait(ctx, delay); err != nil {
			return abandoned, err
		}
		rebound, err := e.invocation.Runtime.SubscribeRun(ctx, agent.SubscribeRun{
			RunID: e.current.RunID, SegmentID: e.current.SegmentID, AfterEventID: e.conversation.Checkpoint(),
		})
		if err == nil {
			if validateSubscriptionErr := rebound.ValidateSubscription(); validateSubscriptionErr != nil {
				return abandoned, fmt.Errorf("subscribe run: %w", validateSubscriptionErr)
			}
			e.current = rebound
			return continuing, nil
		}
		if !runrecovery.Required(err) {
			cause = err
			continue
		}
		recovered, recoveryErr := runrecovery.Recover(ctx, e.invocation.Runtime, e.invocation.Start.SessionID, e.current.RunID)
		if recoveryErr != nil {
			if !runrecovery.Required(recoveryErr) {
				cause = recoveryErr
			}
			continue
		}
		return e.installRecovery(ctx, recovered)
	}
}

func (e *executionDriver) installRecovery(ctx context.Context, recovered runrecovery.State) (disposition, error) {
	if err := e.invocation.Renderer.Reconcile(recovered.Snapshot); err != nil {
		return abandoned, err
	}
	if err := restoreRecoveredConversation(e.conversation, recovered); err != nil {
		return abandoned, err
	}
	switch recovered.Run.Status {
	case agent.RunStatusFinished:
		return settled, errorForOutcome(recovered.Run.Outcome)
	case agent.RunStatusWaiting:
		if err := e.resume(ctx, recovered.Snapshot.Interactions, recovered.Run.ID); err != nil {
			return interactionDisposition(err), err
		}
	case agent.RunStatusRunning:
		e.current = recovered.Stream
	}
	return continuing, nil
}

func restoreRecoveredConversation(conversation *agent.Conversation, recovered runrecovery.State) error {
	if recovered.Run.Status == agent.RunStatusRunning {
		return conversation.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream)
	}
	return conversation.RestoreSnapshot(recovered.Snapshot)
}

func validateContinuation(stream agent.SegmentStream, runID string) error {
	return stream.ValidateResume(runID, nil)
}

type followResult struct {
	interactions []agent.Interaction
	outcome      *agent.Outcome
	err          error
	applied      int
}

func consume(stream agent.EventStream, conversation *agent.Conversation, renderer Renderer) followResult {
	var followed followResult
	for event, streamErr := range stream {
		if streamErr != nil {
			followed.err = streamErr
			break
		}
		result, err := conversation.ApplyRunEvent(event)
		if err != nil {
			followed.err = fmt.Errorf("accept runtime event %s: %w", event.EventID, err)
			break
		}
		if !result.Applied {
			continue
		}
		followed.applied++
		if err := renderer.Render(event); err != nil {
			followed.err = err
			break
		}
		switch payload := event.Event.(type) {
		case agent.RunInterrupted:
			followed.interactions = agent.CloneInteractions(payload.Interactions)
		case agent.RunFinished:
			followed.outcome = new(payload.Outcome)
		}
	}
	return followed
}

type outcomeError struct{ outcome agent.Outcome }

func (o *outcomeError) Error() string {
	if detail := o.outcome.Description(); detail != "" {
		return "run " + string(o.outcome.Status) + ": " + detail
	}
	return "run " + string(o.outcome.Status)
}

func errorForOutcome(outcome agent.Outcome) error {
	if outcome.Status == agent.OutcomeCompleted {
		return nil
	}
	return &outcomeError{outcome: outcome}
}

func unattendedAnswers(interactions []agent.Interaction, approveAll bool, sessionID string) ([]agent.InterruptAnswer, error) {
	answers := make([]agent.InterruptAnswer, 0, len(interactions))
	for _, interaction := range interactions {
		switch item := interaction.(type) {
		case agent.Approval:
			answers = append(answers, agent.InterruptAnswer{ItemID: item.ItemID, Answer: approvalAnswer(approveAll)})
		case agent.Question:
			return nil, &interactionRequiredError{title: item.Title, sessionID: sessionID}
		default:
			return nil, errors.New("runtime returned an unknown interaction")
		}
	}
	return answers, nil
}

func approvalAnswer(approveAll bool) agent.ApprovalAnswer {
	if approveAll {
		return agent.ApprovalAnswer{Decision: agent.ApprovalApprove}
	}
	return agent.ApprovalAnswer{
		Decision: agent.ApprovalDeny,
		Reason:   "declined: this run is unattended (rerun with --approve-all to allow it)",
	}
}

type interactionRequiredError struct {
	title     string
	sessionID string
}

func (i *interactionRequiredError) Error() string {
	return fmt.Sprintf("run needs answers to %q; continue it interactively with --session %s", i.title, i.sessionID)
}
