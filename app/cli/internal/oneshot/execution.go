// Package oneshot owns unattended runs that stream to a non-interactive renderer.
package oneshot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/runrecovery"
)

const cancellationTimeout = 5 * time.Second

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
}

// Execute drives one stable Run across as many Segments as its interrupts
// require. The embedded adapter gives controls stable runtime identities, but
// this use case deliberately does not repeat user intent after an ambiguous
// return; it rebinds an acknowledged segment stream and otherwise cold-recovers.
func Execute(ctx context.Context, invocation Invocation) (runErr error) {
	if invocation.Runtime == nil {
		return errors.New("one-shot run requires a runtime")
	}
	if invocation.Renderer == nil {
		return errors.New("one-shot run requires a renderer")
	}
	if err := invocation.Start.Validate(); err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, invocation.Renderer.Close()) }()

	opened, err := invocation.Runtime.StartRun(ctx, invocation.Start)
	if err != nil {
		return err
	}
	cancelOnExit := true
	var watcher *cancellationWatcher
	if opened.RunID != "" {
		watcher = watchCancellation(ctx, invocation.Runtime, opened.RunID)
		defer func() { watcher.Finish(cancelOnExit) }()
	}
	if err := opened.ValidateStart(); err != nil {
		return fmt.Errorf("start run: %w", err)
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
	if err := invocation.Renderer.Begin(run, invocation.Start.Options); err != nil {
		return err
	}

	disposition, err := drive(ctx, invocation, opened)
	if disposition.preservesRun() {
		cancelOnExit = false
	}
	return err
}

type cancellationWatcher struct {
	exit chan bool
	done chan struct{}
}

func watchCancellation(ctx context.Context, runtime agent.RunLifecycle, runID string) *cancellationWatcher {
	watcher := &cancellationWatcher{exit: make(chan bool, 1), done: make(chan struct{})}
	go func() {
		defer close(watcher.done)
		shouldCancel := true
		select {
		case shouldCancel = <-watcher.exit:
		case <-ctx.Done():
		}
		if !shouldCancel {
			return
		}
		cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancellationTimeout)
		defer cancel()
		_, _ = runtime.CancelRun(cancelCtx, agent.CancelRun{RunID: runID, Reason: "CLI execution ended before the run settled"})
	}()
	return watcher
}

func (w *cancellationWatcher) Finish(cancelRun bool) {
	w.exit <- cancelRun
	<-w.done
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

func (d *executionDriver) run(ctx context.Context) (disposition, error) {
	for {
		followed := consume(d.current.Events, d.conversation, d.invocation.Renderer)
		if followed.outcome != nil {
			return settled, errorForOutcome(*followed.outcome)
		}
		if len(followed.interactions) != 0 {
			if err := d.resume(ctx, followed.interactions, d.current.RunID); err != nil {
				return interactionDisposition(err), err
			}
			continue
		}
		cause := followed.err
		if cause == nil {
			cause = fmt.Errorf("%w: segment stream ended without a terminal event", agent.ErrDisconnected)
		}
		if followed.applied > 0 {
			d.failures = 0
		}
		disposition, err := d.reconnect(ctx, cause)
		if disposition != continuing {
			return disposition, err
		}
	}
}

func (d *executionDriver) resume(ctx context.Context, interactions []agent.Interaction, runID string) error {
	answers, err := unattendedAnswers(interactions, d.invocation.ApproveAll, d.invocation.Start.SessionID)
	if err != nil {
		return err
	}
	continued, err := d.invocation.Runtime.ResumeRun(ctx, agent.ResumeRun{RunID: runID, Answers: answers})
	if err != nil {
		return err
	}
	if err := validateContinuation(continued, d.openedRunID); err != nil {
		return err
	}
	d.current = continued
	d.failures = 0
	return nil
}

func interactionDisposition(err error) disposition {
	if _, required := errors.AsType[*interactionRequiredError](err); required {
		return parked
	}
	return abandoned
}

func (d *executionDriver) reconnect(ctx context.Context, cause error) (disposition, error) {
	for {
		d.failures++
		delay, retry := d.policy.Next(d.failures, cause)
		if !retry {
			return abandoned, cause
		}
		if err := reconnect.Wait(ctx, delay); err != nil {
			return abandoned, err
		}
		rebound, err := d.invocation.Runtime.SubscribeRun(ctx, agent.SubscribeRun{
			RunID: d.current.RunID, SegmentID: d.current.SegmentID, AfterEventID: d.conversation.Checkpoint(),
		})
		if err == nil {
			if err := rebound.ValidateSubscription(); err != nil {
				return abandoned, fmt.Errorf("subscribe run: %w", err)
			}
			d.current = rebound
			return continuing, nil
		}
		if !runrecovery.Required(err) {
			cause = err
			continue
		}
		recovered, recoveryErr := runrecovery.Recover(ctx, d.invocation.Runtime, d.invocation.Start.SessionID, d.current.RunID)
		if recoveryErr != nil {
			if !runrecovery.Required(recoveryErr) {
				cause = recoveryErr
			}
			continue
		}
		return d.installRecovery(ctx, recovered)
	}
}

func (d *executionDriver) installRecovery(ctx context.Context, recovered runrecovery.State) (disposition, error) {
	if err := d.invocation.Renderer.Reconcile(recovered.Snapshot); err != nil {
		return abandoned, err
	}
	if err := restoreRecoveredConversation(d.conversation, recovered); err != nil {
		return abandoned, err
	}
	switch recovered.Run.Status {
	case agent.RunStatusFinished:
		return settled, errorForOutcome(recovered.Run.Outcome)
	case agent.RunStatusWaiting:
		if err := d.resume(ctx, recovered.Snapshot.Interactions, recovered.Run.ID); err != nil {
			return interactionDisposition(err), err
		}
	case agent.RunStatusRunning:
		d.current = recovered.Stream
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
	if err := stream.ValidateResume(nil); err != nil {
		return err
	}
	if stream.RunID != runID {
		return fmt.Errorf("resume run response: run %s does not match %s", stream.RunID, runID)
	}
	return nil
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

func (e *outcomeError) Error() string {
	if detail := e.outcome.Description(); detail != "" {
		return "run " + string(e.outcome.Status) + ": " + detail
	}
	return "run " + string(e.outcome.Status)
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

func (e *interactionRequiredError) Error() string {
	return fmt.Sprintf("run needs answers to %q; continue it interactively with --session %s", e.title, e.sessionID)
}
