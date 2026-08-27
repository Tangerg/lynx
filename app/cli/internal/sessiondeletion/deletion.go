// Package sessiondeletion owns crash-safe settlement of runtime session
// deletion and the corresponding CLI-local authoring state.
package sessiondeletion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/cli/internal/agent"
	"github.com/Tangerg/scope/app/cli/internal/mutation"
	"github.com/Tangerg/scope/app/cli/internal/retry"
	"github.com/Tangerg/scope/app/cli/internal/workbench"
)

type runtime interface {
	DeleteSession(context.Context, agent.DeleteSession) error
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
}

// ReplayWindow identifies the runtime command store and conservative replay
// interval advertised when a deletion intent is first staged.
type ReplayWindow struct {
	Namespace string
	Retention time.Duration
	Now       func() time.Time
}

func (r ReplayWindow) now() time.Time {
	if r.Now == nil {
		return time.Now().UTC()
	}
	return r.Now().UTC()
}

func (r ReplayWindow) guard() (workbench.ReplayGuard, error) {
	if strings.TrimSpace(r.Namespace) == "" && r.Retention == 0 {
		return workbench.ReplayGuard{}, nil
	}
	if strings.TrimSpace(r.Namespace) == "" || r.Retention <= 0 {
		return workbench.ReplayGuard{}, errors.New("session deletion replay guarantee is incomplete")
	}
	return workbench.ReplayGuard{
		Namespace: strings.TrimSpace(r.Namespace), Until: r.now().Add(r.Retention),
	}, nil
}

func (r ReplayWindow) sameStore(guard workbench.ReplayGuard) bool {
	if strings.TrimSpace(r.Namespace) == "" && r.Retention == 0 {
		return guard.Empty()
	}
	return strings.TrimSpace(r.Namespace) != "" &&
		guard.Namespace == strings.TrimSpace(r.Namespace)
}

// Result binds settlement to the exact durable runtime command.
type Result struct {
	Request agent.DeleteSession
	Outcome mutation.Outcome
}

// Execute stages or resumes one deletion intent, then converges its runtime
// outcome without modifying local authoring state. Callers apply Confirm or
// Reject only after receiving the result on their own presentation boundary.
func Execute(
	ctx context.Context,
	runtime runtime,
	authoring *workbench.Store,
	sessionID string,
	window ReplayWindow,
	backoff retry.Backoff,
) (Result, error) {
	if authoring == nil {
		return Result{}, errors.New("CLI workbench is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	pending, exists := authoring.PendingSessionDeletion(sessionID)
	if exists && pending.Phase == workbench.SessionDeletionConfirmed {
		return Result{Request: pending.Request(), Outcome: mutation.Confirmed}, nil
	}
	request := pending.Request()
	if !exists {
		commandID, err := agent.NewCommandID()
		if err != nil {
			return Result{}, fmt.Errorf("create session deletion identity: %w", err)
		}
		request = agent.DeleteSession{CommandID: commandID, SessionID: sessionID}
		replay, err := window.guard()
		if err != nil {
			return Result{}, err
		}
		if err := authoring.StageSessionDeletion(request, replay); err != nil {
			return Result{}, fmt.Errorf("stage session deletion: %w", err)
		}
		pending, exists = authoring.PendingSessionDeletion(sessionID)
		if !exists {
			return Result{}, errors.New("staged session deletion is absent")
		}
	}
	if !replaySafe(pending.Replay, window) {
		outcome, err := resolveExpired(ctx, runtime, pending.SessionID, pending.Replay, window)
		return Result{Request: request, Outcome: outcome}, err
	}
	outcome, err := Settle(ctx, runtime, request, pending.Replay, window, backoff)
	return Result{Request: request, Outcome: outcome}, err
}

// Settle observes the authoritative runtime outcome. A successful delete may
// still return a post-commit cleanup error, so an authoritative not-found read
// is the confirmation when no acknowledgement was delivered.
func Settle(
	ctx context.Context,
	runtime runtime,
	request agent.DeleteSession,
	replay workbench.ReplayGuard,
	window ReplayWindow,
	backoff retry.Backoff,
) (mutation.Outcome, error) {
	_, err := mutation.ConfirmAdmitted(ctx, backoff, func() error {
		if !replaySafe(replay, window) {
			return mutation.ErrReplayGuaranteeUnavailable
		}
		return nil
	}, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, runtime.DeleteSession(ctx, request)
	})
	if err == nil || errors.Is(err, agent.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		outcome, resolveErr := resolveExpired(ctx, runtime, request.SessionID, replay, window)
		if outcome != mutation.Unknown {
			return outcome, resolveErr
		}
		return mutation.Unknown, errors.Join(
			fmt.Errorf("delete session outcome is unknown: %w", err), resolveErr,
		)
	}
	if mutation.OutcomeUnknown(err) {
		return mutation.Unknown, fmt.Errorf("delete session outcome is unknown: %w", err)
	}
	_, readErr := runtime.GetSession(ctx, request.SessionID)
	if errors.Is(readErr, agent.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if readErr != nil {
		return mutation.Unknown, errors.Join(
			fmt.Errorf("delete session: %w", err),
			fmt.Errorf("read deletion outcome: %w", readErr),
		)
	}
	return mutation.Rejected, err
}

// Confirm upgrades a prepared command to a durable tombstone and retires all
// local state. It is idempotent for an already-confirmed cleanup record.
func Confirm(authoring *workbench.Store, result Result) error {
	pending, exists := authoring.PendingSessionDeletion(result.Request.SessionID)
	if exists && pending.Phase == workbench.SessionDeletionPrepared {
		return authoring.ConfirmSessionDeletion(result.Request.SessionID, result.Request.CommandID)
	}
	return authoring.RetireSessionState(result.Request.SessionID)
}

// Reject removes only the exact prepared intent after a definitive refusal.
func Reject(authoring *workbench.Store, result Result) error {
	return authoring.RejectSessionDeletion(result.Request.SessionID, result.Request.CommandID)
}

// Recover settles every journal before any session draft is made visible.
func Recover(
	ctx context.Context,
	runtime runtime,
	authoring *workbench.Store,
	window ReplayWindow,
	backoff retry.Backoff,
) error {
	for _, pending := range authoring.PendingSessionDeletions() {
		if pending.Phase == workbench.SessionDeletionConfirmed {
			if err := authoring.RetireSessionState(pending.SessionID); err != nil {
				return err
			}
			continue
		}
		result := Result{Request: pending.Request()}
		if !replaySafe(pending.Replay, window) {
			outcome, err := resolveExpired(ctx, runtime, pending.SessionID, pending.Replay, window)
			result.Outcome = outcome
			switch outcome {
			case mutation.Confirmed:
				if confirmErr := Confirm(authoring, result); confirmErr != nil {
					return confirmErr
				}
			case mutation.Rejected:
				if rejectErr := Reject(authoring, result); rejectErr != nil {
					return rejectErr
				}
			case mutation.Unknown:
				return fmt.Errorf("recover session deletion %s: %w", pending.SessionID, err)
			}
			continue
		}
		outcome, err := Settle(ctx, runtime, result.Request, pending.Replay, window, backoff)
		result.Outcome = outcome
		switch outcome {
		case mutation.Confirmed:
			if confirmErr := Confirm(authoring, result); confirmErr != nil {
				return confirmErr
			}
		case mutation.Rejected:
			if rejectErr := Reject(authoring, result); rejectErr != nil {
				return errors.Join(err, rejectErr)
			}
		case mutation.Unknown:
			return err
		}
	}
	return nil
}

func resolveExpired(
	ctx context.Context,
	runtime runtime,
	sessionID string,
	replay workbench.ReplayGuard,
	window ReplayWindow,
) (mutation.Outcome, error) {
	if !window.sameStore(replay) {
		return mutation.Unknown, errors.New("session deletion belongs to another runtime")
	}
	_, err := runtime.GetSession(ctx, sessionID)
	if errors.Is(err, agent.ErrSessionNotFound) {
		return mutation.Confirmed, nil
	}
	if err != nil {
		return mutation.Unknown, fmt.Errorf("read deletion outcome: %w", err)
	}
	return mutation.Rejected, nil
}

func replaySafe(guard workbench.ReplayGuard, window ReplayWindow) bool {
	if strings.TrimSpace(window.Namespace) == "" && window.Retention == 0 {
		return guard.Empty()
	}
	return strings.TrimSpace(window.Namespace) != "" && guard.Namespace == strings.TrimSpace(window.Namespace) &&
		window.now().Before(guard.Until)
}
