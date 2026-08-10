// Package oneshot owns unattended runs that stream to a non-interactive renderer.
package oneshot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/client"
	"github.com/Tangerg/lynx/app/cli/internal/reconnect"
	"github.com/Tangerg/lynx/app/cli/internal/requestid"
)

const cancelTimeout = 5 * time.Second

// Renderer consumes the accepted event prefix of a run and releases its output
// resources when the run ends or fails.
type Renderer interface {
	Render(client.Envelope) error
	Close() error
}

type resultInitializer interface {
	Begin(client.Run, client.RunOptions) error
}

// Config contains the dependencies and product policy for one unattended run.
type Config struct {
	Runtime           client.Runs
	Renderer          Renderer
	Start             client.StartRun
	ApproveAll        bool
	ReconnectAttempts int
}

// Run starts, follows, and settles one unattended run. Approvals are denied
// unless ApproveAll is set; questions remain parked for an interactive client.
func Run(ctx context.Context, config Config) (runErr error) {
	if config.Runtime == nil {
		return errors.New("one-shot run requires a runtime")
	}
	if config.Renderer == nil {
		return errors.New("one-shot run requires a renderer")
	}
	defer func() { runErr = errors.Join(runErr, config.Renderer.Close()) }()
	if err := ensureRequestID(&config.Start); err != nil {
		return err
	}
	policy := reconnect.New(config.ReconnectAttempts)
	guard := watchCancellation(ctx, config.Runtime, policy, client.CancelRun{
		SessionID: config.Start.SessionID,
		RequestID: config.Start.RequestID,
	})
	cancelOnExit := true
	defer func() { guard.Close(cancelOnExit) }()

	run, err := reconnect.ControlValue(ctx, policy, func() (client.Run, error) {
		return config.Runtime.StartRun(ctx, config.Start)
	})
	if err != nil {
		return err
	}
	if err := validateStartedRun(run, config.Start.SessionID); err != nil {
		return err
	}
	if initializer, ok := config.Renderer.(resultInitializer); ok {
		if err := initializer.Begin(run, config.Start.Options); err != nil {
			return err
		}
	}
	disposition, err := drive(ctx, config.Runtime, config.Renderer, config.Start, run, config.ApproveAll, policy)
	if disposition.preservesRun() {
		cancelOnExit = false
	}
	return err
}

func validateStartedRun(run client.Run, sessionID string) error {
	if err := run.Validate(); err != nil {
		return fmt.Errorf("start run response: %w", err)
	}
	if run.SessionID != sessionID {
		return fmt.Errorf("start run response: run belongs to session %s, want %s", run.SessionID, sessionID)
	}
	return nil
}

func ensureRequestID(start *client.StartRun) error {
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

type cancellationGuard struct {
	exit chan bool
	done chan struct{}
}

func watchCancellation(ctx context.Context, runtime client.Runs, policy reconnect.Policy, request client.CancelRun) *cancellationGuard {
	guard := &cancellationGuard{exit: make(chan bool, 1), done: make(chan struct{})}
	go func() {
		defer close(guard.done)
		shouldCancel := true
		select {
		case shouldCancel = <-guard.exit:
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
	return guard
}

func (g *cancellationGuard) Close(cancel bool) {
	g.exit <- cancel
	<-g.done
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
	runtime client.Runs,
	renderer Renderer,
	start client.StartRun,
	run client.Run,
	approveAll bool,
	policy reconnect.Policy,
) (disposition, error) {
	conversation := client.NewConversationAt(run.StartedAfter)
	failures := 0
	for {
		before := conversation.Cursor()
		stream, err := runtime.FollowRun(ctx, client.FollowRun{RunID: run.ID, After: before})
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
			return settled, outcomeError(*state.outcome)
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
			state.err = fmt.Errorf("%w: runtime subscription ended without interrupting or finishing the run", client.ErrDisconnected)
		}
		if retryErr := waitToReconnect(ctx, policy, &failures, progressed, state.err); retryErr != nil {
			return abandoned, retryErr
		}
	}
}

type subscriptionState struct {
	interrupted client.Interaction
	outcome     *client.Outcome
	err         error
}

func consume(stream client.Stream, conversation *client.Conversation, renderer Renderer) subscriptionState {
	var state subscriptionState
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
		case client.RunInterrupted:
			state.interrupted = event.Interaction
		case client.RunFinished:
			state.outcome = new(event.Outcome)
		}
	}
	return state
}

type outcomeErrorValue struct{ outcome client.Outcome }

func (e *outcomeErrorValue) Error() string {
	if e.outcome.Status == client.OutcomeFailed {
		return "run failed: " + e.outcome.Error
	}
	return "run " + string(e.outcome.Status)
}

func outcomeError(outcome client.Outcome) error {
	if outcome.Status == client.OutcomeCompleted {
		return nil
	}
	return &outcomeErrorValue{outcome: outcome}
}

func resume(
	ctx context.Context,
	runtime client.Runs,
	policy reconnect.Policy,
	runID string,
	sessionID string,
	interaction client.Interaction,
	approveAll bool,
) error {
	answer, interruptID, err := unattendedAnswer(interaction, approveAll, sessionID)
	if err != nil {
		return err
	}
	request := client.ResumeRun{RunID: runID, InterruptID: interruptID, Answer: answer}
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

func unattendedAnswer(interaction client.Interaction, approveAll bool, sessionID string) (client.Answer, string, error) {
	switch item := interaction.(type) {
	case client.Approval:
		return approvalAnswer(approveAll), item.InterruptID, nil
	case client.Question:
		return nil, item.InterruptID, &interactionRequiredError{title: item.Title, sessionID: sessionID}
	default:
		return nil, "", errors.New("runtime returned an unknown interaction")
	}
}

func approvalAnswer(approveAll bool) client.ApprovalAnswer {
	if approveAll {
		return client.ApprovalAnswer{Decision: client.ApprovalAllow, Remember: client.RememberNone}
	}
	return client.ApprovalAnswer{
		Decision: client.ApprovalDeny,
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
