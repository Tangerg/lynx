// Package sessiondeletion owns crash-safe settlement of runtime session
// deletion and the corresponding CLI-local authoring state.
package sessiondeletion

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
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

func (window ReplayWindow) now() time.Time {
	if window.Now == nil {
		return time.Now().UTC()
	}
	return window.Now().UTC()
}

func (window ReplayWindow) guard() (workbench.ReplayGuard, error) {
	if strings.TrimSpace(window.Namespace) == "" && window.Retention == 0 {
		return workbench.ReplayGuard{}, nil
	}
	if strings.TrimSpace(window.Namespace) == "" || window.Retention <= 0 {
		return workbench.ReplayGuard{}, errors.New("session deletion replay guarantee is incomplete")
	}
	return workbench.ReplayGuard{
		Namespace: strings.TrimSpace(window.Namespace), Until: window.now().Add(window.Retention),
	}, nil
}

// Outcome distinguishes an authoritative confirmation from a definitive
// refusal and an outcome which must remain durable for later recovery.
type Outcome uint8

const (
	Unknown Outcome = iota
	Rejected
	Confirmed
)

// Result binds settlement to the exact durable runtime command.
type Result struct {
	Request agent.DeleteSession
	Outcome Outcome
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
		return Result{Request: pending.Request(), Outcome: Confirmed}, nil
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
		return Result{Request: request, Outcome: Unknown}, errors.New("session deletion replay guarantee expired or belongs to another runtime")
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
) (Outcome, error) {
	_, err := mutation.ConfirmAdmitted(ctx, backoff, func() error {
		if !replaySafe(replay, window) {
			return mutation.ErrReplayGuaranteeUnavailable
		}
		return nil
	}, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, runtime.DeleteSession(ctx, request)
	})
	if err == nil || errors.Is(err, agent.ErrSessionNotFound) {
		return Confirmed, nil
	}
	if mutation.OutcomeUnknown(err) {
		return Unknown, fmt.Errorf("delete session outcome is unknown: %w", err)
	}
	_, readErr := runtime.GetSession(ctx, request.SessionID)
	if errors.Is(readErr, agent.ErrSessionNotFound) {
		return Confirmed, nil
	}
	if readErr != nil {
		return Unknown, errors.Join(
			fmt.Errorf("delete session: %w", err),
			fmt.Errorf("read deletion outcome: %w", readErr),
		)
	}
	return Rejected, err
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
			return fmt.Errorf(
				"recover session deletion %s: replay guarantee expired or belongs to another runtime",
				pending.SessionID,
			)
		}
		outcome, err := Settle(ctx, runtime, result.Request, pending.Replay, window, backoff)
		result.Outcome = outcome
		switch outcome {
		case Confirmed:
			if confirmErr := Confirm(authoring, result); confirmErr != nil {
				return confirmErr
			}
		case Rejected:
			if rejectErr := Reject(authoring, result); rejectErr != nil {
				return errors.Join(err, rejectErr)
			}
		case Unknown:
			return err
		}
	}
	return nil
}

func replaySafe(guard workbench.ReplayGuard, window ReplayWindow) bool {
	if strings.TrimSpace(window.Namespace) == "" && window.Retention == 0 {
		return guard.Empty()
	}
	return strings.TrimSpace(window.Namespace) != "" && guard.Namespace == strings.TrimSpace(window.Namespace) &&
		window.now().Before(guard.Until)
}
