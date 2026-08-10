package mock

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/cli/internal/client"
)

func (r *Runtime) StartRun(ctx context.Context, in client.StartRun) (client.Run, error) {
	if err := in.Validate(); err != nil {
		return client.Run{}, fmt.Errorf("mock: %w", err)
	}
	attempt, owner, err := r.reserveStart(ctx, in)
	if err != nil {
		return client.Run{}, err
	}
	if !owner {
		return awaitStart(ctx, r, attempt)
	}

	build := r.Script
	if build == nil {
		build = Conversation
	}
	script, buildErr := buildScriptSafely(build, strings.TrimSpace(in.Message.Text))

	r.mu.Lock()
	run, err := r.commitStartLocked(attempt, script, buildErr)
	started := client.Run{}
	if err == nil {
		started = projectRun(run)
	}
	r.mu.Unlock()
	if err != nil {
		return client.Run{}, err
	}
	go r.play(run, run.script.Prelude, run.script.interrupts())
	return started, nil
}

func (r *Runtime) reserveStart(ctx context.Context, in client.StartRun) (*startAttempt, bool, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[in.SessionID]
	if !ok {
		return nil, false, fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	if in.RequestID != "" {
		key := requestKey(in.SessionID, in.RequestID)
		if _, canceled := r.canceled[key]; canceled {
			return nil, false, fmt.Errorf("%w: request %s", client.ErrRunCanceled, in.RequestID)
		}
		if existing := r.starts[key]; existing != nil {
			if !sameStart(existing.input, in) {
				return nil, false, fmt.Errorf("%w: request %s", client.ErrRequestConflict, in.RequestID)
			}
			return existing, false, nil
		}
	}
	if session.active != "" || session.starting != nil {
		return nil, false, fmt.Errorf("%w: %s", client.ErrSessionBusy, in.SessionID)
	}
	attempt := &startAttempt{input: cloneStart(in), ready: make(chan struct{})}
	session.starting = attempt
	if in.RequestID != "" {
		r.starts[requestKey(in.SessionID, in.RequestID)] = attempt
	}
	return attempt, true, nil
}

func awaitStart(ctx context.Context, runtime *Runtime, attempt *startAttempt) (client.Run, error) {
	select {
	case <-attempt.ready:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return attempt.run, attempt.err
	case <-ctx.Done():
		return client.Run{}, context.Cause(ctx)
	}
}

func (r *Runtime) commitStartLocked(attempt *startAttempt, script Script, buildErr error) (*runState, error) {
	if attempt.finished {
		return nil, attempt.err
	}
	session := r.sessions[attempt.input.SessionID]
	if session == nil || session.starting != attempt {
		err := fmt.Errorf("%w: request %s", client.ErrRunCanceled, attempt.input.RequestID)
		r.finishStartLocked(attempt, client.Run{}, err)
		return nil, err
	}
	if buildErr != nil {
		err := fmt.Errorf("mock: build script: %w", buildErr)
		session.starting = nil
		r.finishStartLocked(attempt, client.Run{}, err)
		return nil, err
	}
	r.next++
	run := &runState{
		id: fmt.Sprintf("run_mock_%d", r.next), sessionID: attempt.input.SessionID,
		startedAfter: client.Cursor(len(session.events)), status: client.RunActive,
		script: script, start: cloneStart(attempt.input), answers: make(map[string]client.Answer),
		cancel: make(chan struct{}),
	}
	r.runs[run.id] = run
	session.starting = nil
	session.active = run.id
	r.emitLocked(run, client.RunStarted{RunID: run.id, SessionID: run.sessionID, Options: attempt.input.Options})
	r.emitLocked(run, client.BlockCompleted{Block: client.Block{
		ID: run.id + "_prompt", Kind: client.BlockUser,
		Text: strings.TrimSpace(attempt.input.Message.Text), Attachments: slices.Clone(attempt.input.Message.Attachments),
	}})
	started := projectRun(run)
	r.finishStartLocked(attempt, started, nil)
	return run, nil
}

func (r *Runtime) finishStartLocked(attempt *startAttempt, run client.Run, err error) {
	if attempt.finished {
		return
	}
	attempt.run, attempt.err, attempt.finished = run, err, true
	close(attempt.ready)
}

func (r *Runtime) ResumeRun(ctx context.Context, in client.ResumeRun) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	run, attempt, owner, err := r.reserveResume(ctx, in)
	if err != nil || attempt == nil {
		return err
	}
	if !owner {
		return awaitResume(ctx, r, attempt)
	}
	steps, continuationErr := continueSafely(run.script, in.Answer)
	r.mu.Lock()
	err = r.commitResumeLocked(run, attempt, steps, continuationErr)
	r.mu.Unlock()
	if err != nil {
		return err
	}
	go r.play(run, steps, false)
	return nil
}

func (r *Runtime) reserveResume(ctx context.Context, in client.ResumeRun) (*runState, *resumeAttempt, bool, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, nil, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[in.RunID]
	if !ok {
		return nil, nil, false, fmt.Errorf("%w: %s", client.ErrRunNotFound, in.RunID)
	}
	attempt, owner, err := resolveResumeAttemptLocked(run, in)
	return run, attempt, owner, err
}

func resolveResumeAttemptLocked(run *runState, in client.ResumeRun) (*resumeAttempt, bool, error) {
	if answered, exists := run.answers[in.InterruptID]; exists {
		if client.EqualAnswers(answered, in.Answer) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("%w: interrupt %s", client.ErrRequestConflict, in.InterruptID)
	}
	if pending := run.resuming; pending != nil {
		if pending.interruptID != in.InterruptID || !client.EqualAnswers(pending.answer, in.Answer) {
			return nil, false, fmt.Errorf("%w: interrupt %s", client.ErrRequestConflict, in.InterruptID)
		}
		return pending, false, nil
	}
	if run.status != client.RunWaiting || client.InteractionID(run.interaction) != in.InterruptID {
		return nil, false, fmt.Errorf("%w: %s", client.ErrInterruptNotOpen, in.InterruptID)
	}
	if err := client.ValidateAnswer(run.interaction, in.Answer); err != nil {
		return nil, false, fmt.Errorf("mock: %w", err)
	}
	attempt := &resumeAttempt{
		interruptID: in.InterruptID,
		answer:      client.CloneAnswer(in.Answer),
		ready:       make(chan struct{}),
	}
	run.resuming = attempt
	return attempt, true, nil
}

func awaitResume(ctx context.Context, runtime *Runtime, attempt *resumeAttempt) error {
	select {
	case <-attempt.ready:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return attempt.err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}

func (r *Runtime) commitResumeLocked(run *runState, attempt *resumeAttempt, steps []Step, continuationErr error) error {
	if run.resuming != attempt {
		return attempt.err
	}
	if continuationErr != nil {
		err := fmt.Errorf("mock: continue script: %w", continuationErr)
		r.finishResumeLocked(run, attempt, err)
		return err
	}
	if run.status != client.RunWaiting {
		err := fmt.Errorf("%w: run %s", client.ErrRunCanceled, run.id)
		r.finishResumeLocked(run, attempt, err)
		return err
	}
	run.answers[attempt.interruptID] = client.CloneAnswer(attempt.answer)
	if approval, ok := run.interaction.(client.Approval); ok {
		if answer, ok := attempt.answer.(client.ApprovalAnswer); ok && answer.Remember != "" && answer.Remember != client.RememberNone {
			r.rememberApprovalLocked(run, approval, answer)
		}
	}
	run.status = client.RunActive
	run.interaction = nil
	r.emitLocked(run, client.RunResumed{InterruptID: attempt.interruptID})
	r.finishResumeLocked(run, attempt, nil)
	return nil
}

func (r *Runtime) finishResumeLocked(run *runState, attempt *resumeAttempt, err error) {
	if run.resuming == attempt {
		run.resuming = nil
	}
	attempt.err = err
	close(attempt.ready)
}

func (r *Runtime) CancelRun(ctx context.Context, in client.CancelRun) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run, err := r.resolveCancellationLocked(in)
	if err != nil || run == nil {
		return err
	}
	r.cancelRunLocked(run)
	return nil
}

func (r *Runtime) resolveCancellationLocked(in client.CancelRun) (*runState, error) {
	if in.RunID != "" {
		run := r.runs[in.RunID]
		if run == nil {
			return nil, fmt.Errorf("%w: %s", client.ErrRunNotFound, in.RunID)
		}
		return run, nil
	}
	return r.cancelStartLocked(in)
}

func (r *Runtime) cancelStartLocked(in client.CancelRun) (*runState, error) {
	session := r.sessions[in.SessionID]
	if session == nil {
		return nil, fmt.Errorf("%w: %s", client.ErrSessionNotFound, in.SessionID)
	}
	key := requestKey(in.SessionID, in.RequestID)
	r.canceled[key] = struct{}{}
	attempt := r.starts[key]
	if attempt == nil {
		return nil, nil
	}
	if !attempt.finished {
		if session.starting == attempt {
			session.starting = nil
		}
		r.finishStartLocked(attempt, client.Run{}, fmt.Errorf("%w: request %s", client.ErrRunCanceled, in.RequestID))
		return nil, nil
	}
	if attempt.err != nil {
		return nil, nil
	}
	return r.runs[attempt.run.ID], nil
}

func (r *Runtime) cancelRunLocked(run *runState) {
	run.cancelOnce.Do(func() { close(run.cancel) })
	if pending := run.resuming; pending != nil {
		r.finishResumeLocked(run, pending, fmt.Errorf("%w: run %s", client.ErrRunCanceled, run.id))
	}
	r.finishLocked(run, client.RunFinished{Outcome: client.Outcome{Status: client.OutcomeCanceled}})
}
