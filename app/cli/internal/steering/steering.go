// Package steering owns crash-safe delivery and settlement of run steering
// commands whose attachments are borrowed from the session composer.
package steering

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
	SteerRun(context.Context, agent.SteerRun) error
}

// ReplayWindow identifies the runtime command store that can still return the
// original outcome for one stable idempotency key.
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

// Stage atomically transfers the source draft's attachments into a durable
// command journal before delivery can begin.
func Stage(
	authoring *workbench.Store,
	sessionID string,
	request agent.SteerRun,
	sourceDraft agent.Message,
	window ReplayWindow,
) (workbench.PendingSteer, error) {
	if authoring == nil {
		return workbench.PendingSteer{}, errors.New("CLI workbench is unavailable")
	}
	if err := request.Validate(); err != nil {
		return workbench.PendingSteer{}, err
	}
	if request.CommandID == "" {
		return workbench.PendingSteer{}, errors.New("steer command id is empty")
	}
	stagedAt := window.now()
	pending := workbench.PendingSteer{
		SessionID: strings.TrimSpace(sessionID), Command: request.Clone(), StagedAt: stagedAt,
	}
	if namespace := strings.TrimSpace(window.Namespace); namespace != "" {
		if window.Retention <= 0 {
			return workbench.PendingSteer{}, errors.New("steer replay retention is not positive")
		}
		pending.ReplayNamespace = namespace
		pending.ReplayUntil = stagedAt.Add(window.Retention)
	}
	if err := authoring.StagePendingSteer(pending, sourceDraft); err != nil {
		return workbench.PendingSteer{}, fmt.Errorf("stage steer command: %w", err)
	}
	return pending, nil
}

// Result binds settlement to the exact durable command.
type Result struct {
	Pending workbench.PendingSteer
	Outcome mutation.Outcome
}

// Deliver settles a freshly staged command. Its first attempt does not depend
// on a replay window because the command has not previously left this process.
func Deliver(
	ctx context.Context,
	runtime runtime,
	pending workbench.PendingSteer,
	window ReplayWindow,
	backoff retry.Backoff,
) (Result, error) {
	result := Result{Pending: pending}
	if runtime == nil {
		return result, errors.New("steer runtime is unavailable")
	}
	if err := pending.Validate(); err != nil {
		return result, err
	}
	var admit mutation.Admission
	if pending.ReplayNamespace != "" || !pending.ReplayUntil.IsZero() ||
		window.Namespace != "" || window.Retention != 0 {
		admit = func() error {
			if !replaySafe(pending, window) {
				return mutation.ErrReplayGuaranteeUnavailable
			}
			return nil
		}
	}
	_, err := mutation.ConfirmAdmitted(ctx, backoff, admit, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, runtime.SteerRun(ctx, pending.Command)
	})
	if err == nil {
		result.Outcome = mutation.Confirmed
		return result, nil
	}
	if mutation.OutcomeUnknown(err) {
		result.Outcome = mutation.Unknown
		return result, fmt.Errorf("steer command outcome is unknown: %w", err)
	}
	result.Outcome = mutation.Rejected
	return result, err
}

// Recover replays every unsettled command only while the same runtime
// idempotency namespace still guarantees its original response. Definitive
// refusals atomically return attachments to the durable session draft.
func Recover(
	ctx context.Context,
	runtime runtime,
	authoring *workbench.Store,
	window ReplayWindow,
	backoff retry.Backoff,
) error {
	if authoring == nil {
		return errors.New("CLI workbench is unavailable")
	}
	for _, pending := range authoring.PendingSteers() {
		if !replaySafe(pending, window) {
			return fmt.Errorf(
				"recover steer command for session %s: replay guarantee expired or belongs to another runtime",
				pending.SessionID,
			)
		}
		result, err := Deliver(ctx, runtime, pending, window, backoff)
		switch result.Outcome {
		case mutation.Confirmed:
			if acknowledgeErr := authoring.AcknowledgePendingSteer(
				pending.SessionID, pending.Command.CommandID,
			); acknowledgeErr != nil {
				return errors.Join(err, acknowledgeErr)
			}
		case mutation.Rejected:
			draft, _, draftErr := authoring.Draft(pending.SessionID)
			if draftErr != nil {
				return errors.Join(err, draftErr)
			}
			if _, rejectErr := authoring.RejectPendingSteer(
				pending.SessionID, pending.Command.CommandID, draft,
			); rejectErr != nil {
				return errors.Join(err, rejectErr)
			}
		case mutation.Unknown:
			return err
		default:
			return errors.New("steer settlement returned an invalid outcome")
		}
	}
	return nil
}

func replaySafe(pending workbench.PendingSteer, window ReplayWindow) bool {
	return strings.TrimSpace(window.Namespace) != "" &&
		strings.TrimSpace(window.Namespace) == pending.ReplayNamespace &&
		window.now().Before(pending.ReplayUntil)
}
