// Package oneshot owns unattended runs that stream to a non-interactive renderer.
package oneshot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/requestid"
)

const cancelTimeout = 5 * time.Second

// Renderer consumes the accepted event prefix of a run and releases its output
// resources when the run ends or fails.
type Renderer interface {
	Render(agent.Envelope) error
	Close() error
}

type runInitializer interface {
	Begin(agent.Run, agent.RunOptions) error
}

// Invocation contains the dependencies and product policy for one unattended run.
type Invocation struct {
	Runtime           agent.RunLifecycle
	Renderer          Renderer
	Start             agent.StartRun
	ApproveAll        bool
	ReconnectAttempts int
}

// Execute starts, follows, and settles one unattended run. Approvals are denied
// unless ApproveAll is set; questions remain parked for an interactive agent.
func Execute(ctx context.Context, invocation Invocation) (runErr error) {
	if invocation.Runtime == nil {
		return errors.New("one-shot run requires a runtime")
	}
	if invocation.Renderer == nil {
		return errors.New("one-shot run requires a renderer")
	}
	defer func() { runErr = errors.Join(runErr, invocation.Renderer.Close()) }()
	if err := ensureRequestID(&invocation.Start); err != nil {
		return err
	}
	policy := reconnect.New(invocation.ReconnectAttempts)
	watcher := watchCancellation(ctx, invocation.Runtime, policy, agent.CancelRun{
		SessionID: invocation.Start.SessionID,
		RequestID: invocation.Start.RequestID,
	})
	cancelOnExit := true
	defer func() { watcher.Finish(cancelOnExit) }()

	run, err := reconnect.ControlValue(ctx, policy, func() (agent.Run, error) {
		return invocation.Runtime.StartRun(ctx, invocation.Start)
	})
	if err != nil {
		return err
	}
	if err := validateStartedRun(run, invocation.Start.SessionID); err != nil {
		return err
	}
	if initializer, ok := invocation.Renderer.(runInitializer); ok {
		if err := initializer.Begin(run, invocation.Start.Options); err != nil {
			return err
		}
	}
	disposition, err := drive(ctx, invocation.Runtime, invocation.Renderer, invocation.Start, run, invocation.ApproveAll, policy)
	if disposition.preservesRun() {
		cancelOnExit = false
	}
	return err
}

func validateStartedRun(run agent.Run, sessionID string) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("start run response: %w", err)
	}
	if run.SessionID != sessionID {
		return fmt.Errorf("start run response: run belongs to session %s, want %s", run.SessionID, sessionID)
	}
	return nil
}

func ensureRequestID(start *agent.StartRun) error {
	if start.RequestID != "" {
		return nil
	}
	id, err := requestid.New()
	if err != nil {
		return err
	}
	start.RequestID = id
	return nil
}

type cancellationWatcher struct {
	exit chan bool
	done chan struct{}
}

func watchCancellation(ctx context.Context, runtime agent.RunLifecycle, policy reconnect.Policy, request agent.CancelRun) *cancellationWatcher {
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
		cancelCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancelTimeout)
		defer cancel()
		_ = reconnect.Control(cancelCtx, policy, func() error {
			return runtime.CancelRun(cancelCtx, request)
		})
	}()
	return watcher
}

func (w *cancellationWatcher) Finish(cancelRun bool) {
	w.exit <- cancelRun
	<-w.done
}

type disposition uint8

const (
	abandoned disposition = iota
	settled
	parked
)

func (d disposition) preservesRun() bool { return d == settled || d == parked }

func drive(
	ctx context.Context,
	runtime agent.RunLifecycle,
	renderer Renderer,
	start agent.StartRun,
	run agent.Run,
	approveAll bool,
	policy reconnect.Policy,
) (disposition, error) {
	conversation := agent.NewConversationAt(run.StartedAfter)
	failures := 0
	for {
		before := conversation.Cursor()
		stream, err := runtime.FollowRun(ctx, agent.FollowRun{RunID: run.ID, After: before})
		if err != nil {
			if retryErr := waitToReconnect(ctx, policy, &failures, false, err); retryErr != nil {
				return abandoned, retryErr
			}
			continue
		}
		if stream == nil {
			return abandoned, errors.New("runtime returned a nil event stream")
		}
		state := consume(stream, conversation, renderer)
		progressed := conversation.Cursor() > before
		if state.outcome != nil {
			return settled, errorForOutcome(*state.outcome)
		}
		if state.interrupted != nil {
			if err := resume(ctx, runtime, policy, run.ID, start.SessionID, state.interrupted, approveAll); err != nil {
				if _, required := errors.AsType[*interactionRequiredError](err); required {
					return parked, err
				}
				return abandoned, err
			}
			failures = 0
			continue
		}
		if state.err == nil {
			state.err = fmt.Errorf("%w: runtime subscription ended without interrupting or finishing the run", agent.ErrDisconnected)
		}
		if retryErr := waitToReconnect(ctx, policy, &failures, progressed, state.err); retryErr != nil {
			return abandoned, retryErr
		}
	}
}

type followResult struct {
	interrupted agent.Interaction
	outcome     *agent.Outcome
	err         error
}

func consume(stream agent.RunStream, conversation *agent.Conversation, renderer Renderer) followResult {
	var state followResult
	for envelope, streamErr := range stream {
		if streamErr != nil {
			state.err = streamErr
			break
		}
		result, err := conversation.ApplyEnvelope(envelope)
		if err != nil {
			state.err = fmt.Errorf("accept runtime event at cursor %d: %w", envelope.Cursor, err)
			break
		}
		if !result.Applied {
			continue
		}
		if err := renderer.Render(envelope); err != nil {
			state.err = err
			break
		}
		switch event := envelope.Event.(type) {
		case agent.RunInterrupted:
			state.interrupted = event.Interaction
		case agent.RunFinished:
			state.outcome = new(event.Outcome)
		}
	}
	return state
}

type outcomeError struct{ outcome agent.Outcome }

func (e *outcomeError) Error() string {
	if e.outcome.Status == agent.OutcomeFailed {
		return "run failed: " + e.outcome.Error
	}
	return "run " + string(e.outcome.Status)
}

func errorForOutcome(outcome agent.Outcome) error {
	if outcome.Status == agent.OutcomeCompleted {
		return nil
	}
	return &outcomeError{outcome: outcome}
}

func resume(
	ctx context.Context,
	runtime agent.RunLifecycle,
	policy reconnect.Policy,
	runID string,
	sessionID string,
	interaction agent.Interaction,
	approveAll bool,
) error {
	answer, interruptID, err := unattendedAnswer(interaction, approveAll, sessionID)
	if err != nil {
		return err
	}
	request := agent.ResumeRun{RunID: runID, InterruptID: interruptID, Answer: answer}
	return reconnect.Control(ctx, policy, func() error { return runtime.ResumeRun(ctx, request) })
}

func waitToReconnect(ctx context.Context, policy reconnect.Policy, failures *int, progressed bool, cause error) error {
	if progressed {
		*failures = 0
	}
	*failures++
	delay, ok := policy.Next(*failures, cause)
	if !ok {
		return cause
	}
	return reconnect.Wait(ctx, delay)
}

func unattendedAnswer(interaction agent.Interaction, approveAll bool, sessionID string) (agent.Answer, string, error) {
	switch item := interaction.(type) {
	case agent.Approval:
		return approvalAnswer(approveAll), item.InterruptID, nil
	case agent.Question:
		return nil, item.InterruptID, &interactionRequiredError{title: item.Title, sessionID: sessionID}
	default:
		return nil, "", errors.New("runtime returned an unknown interaction")
	}
}

func approvalAnswer(approveAll bool) agent.ApprovalAnswer {
	if approveAll {
		return agent.ApprovalAnswer{Decision: agent.ApprovalAllow, Remember: agent.RememberNone}
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
