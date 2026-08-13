// Package sessionrollback owns crash-safe settlement of runtime session
// rollback and the corresponding CLI-local opening-input recovery.
package sessionrollback

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type runtime interface {
	RollbackSession(context.Context, agent.RollbackSession) (agent.RollbackResult, error)
	GetSession(context.Context, string) (agent.SessionSnapshot, error)
}

// ReplayWindow identifies one runtime's durable command replay store and the
// conservative interval during which a newly staged command remains replayable.
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

// Preview is the exact authoritatively-read before/after projection authorized
// by one rollback confirmation.
type Preview struct {
	request        agent.RollbackSession
	beforeRevision uint64
	beforeRunIDs   []string
	afterRunIDs    []string
	openingText    string
	openingImages  int
}

// PreviewSession derives a rollback proof and recoverable opening input from
// one authoritative session snapshot.
func PreviewSession(snapshot agent.SessionSnapshot, request agent.RollbackSession) (Preview, error) {
	if err := request.Validate(); err != nil {
		return Preview{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Preview{}, fmt.Errorf("preview rollback: %w", err)
	}
	if snapshot.Session.ID != request.SessionID {
		return Preview{}, errors.New("preview rollback: runtime returned another session")
	}
	boundary := -1
	if request.ToRunID != "" {
		boundary = slices.IndexFunc(snapshot.Runs, func(run agent.Run) bool { return run.ID == request.ToRunID })
		if boundary < 0 {
			return Preview{}, fmt.Errorf("%w: %s", agent.ErrRunNotFound, request.ToRunID)
		}
		if !snapshot.Runs[boundary].Lineage.IsRoot() {
			return Preview{}, fmt.Errorf("rollback run %s is not a root run", request.ToRunID)
		}
	}
	allIDs := make([]string, len(snapshot.Runs))
	for index, run := range snapshot.Runs {
		allIDs[index] = run.ID
	}
	preview := Preview{
		request: request, beforeRevision: snapshot.Session.Revision,
		beforeRunIDs: slices.Clone(allIDs), afterRunIDs: slices.Clone(allIDs),
	}
	if request.Scope != agent.RestoreFiles {
		dropFrom := 0
		if boundary >= 0 {
			dropFrom = len(snapshot.Runs)
			for index := boundary + 1; index < len(snapshot.Runs); index++ {
				if snapshot.Runs[index].Lineage.IsRoot() {
					dropFrom = index
					break
				}
			}
		}
		preview.afterRunIDs = slices.Clone(allIDs[:dropFrom])
		preview.openingText, preview.openingImages = openingInput(snapshot.Transcript, allIDs[dropFrom:])
	}
	return preview, nil
}

func openingInput(transcript []agent.Block, droppedIDs []string) (string, int) {
	for _, runID := range droppedIDs {
		for _, block := range transcript {
			if block.RunID != runID || block.Kind != agent.BlockUser {
				continue
			}
			images := 0
			for _, attachment := range block.Attachments {
				if attachment.Kind == agent.AttachmentImage {
					images++
				}
			}
			if strings.TrimSpace(block.Text) != "" || images > 0 {
				return block.Text, images
			}
		}
	}
	return "", 0
}

func (preview Preview) Request() agent.RollbackSession { return preview.request }

func (preview Preview) DroppedCount() int {
	return len(preview.beforeRunIDs) - len(preview.afterRunIDs)
}

func (preview Preview) ValidateCommit(snapshot agent.SessionSnapshot) error {
	return validateBefore(preview.journal("", ReplayWindow{}, time.Time{}), snapshot)
}

func (preview Preview) ValidateApplied(snapshot agent.SessionSnapshot) error {
	return validateApplied(preview.journal("", ReplayWindow{}, time.Time{}), snapshot)
}

func (preview Preview) journal(
	commandID agent.CommandID,
	window ReplayWindow,
	stagedAt time.Time,
) workbench.PendingSessionRollback {
	pending := workbench.PendingSessionRollback{
		Phase: workbench.SessionRollbackPrepared, CommandID: commandID,
		SessionID: preview.request.SessionID, ToRunID: preview.request.ToRunID, Scope: preview.request.Scope,
		BeforeRevision: preview.beforeRevision, BeforeRunIDs: slices.Clone(preview.beforeRunIDs),
		AfterRunIDs: slices.Clone(preview.afterRunIDs), OpeningText: preview.openingText,
		OpeningImages: preview.openingImages, StagedAt: stagedAt,
	}
	if preview.request.Scope != agent.RestoreHistory {
		pending.ReplayNamespace = strings.TrimSpace(window.Namespace)
		pending.ReplayUntil = stagedAt.Add(window.Retention)
	}
	return pending
}

// Outcome distinguishes confirmation, definitive refusal, and a command whose
// acknowledgement must remain durable for later recovery.
type Outcome uint8

const (
	Unknown Outcome = iota
	Rejected
	Confirmed
)

// Result binds settlement to the exact durable command and authoritative
// session projection.
type Result struct {
	Pending  workbench.PendingSessionRollback
	Outcome  Outcome
	Snapshot agent.SessionSnapshot
}

// Execute verifies an unchanged preview, stages one command identity, then
// settles its runtime outcome without consuming the local recovery record.
func Execute(
	ctx context.Context,
	runtime runtime,
	authoring *workbench.Store,
	preview Preview,
	window ReplayWindow,
	backoff retry.Backoff,
) (Result, error) {
	if authoring == nil {
		return Result{}, errors.New("CLI workbench is unavailable")
	}
	latest, err := runtime.GetSession(ctx, preview.request.SessionID)
	if err != nil {
		return Result{}, err
	}
	if err := preview.ValidateCommit(latest); err != nil {
		return Result{}, err
	}
	commandID, err := agent.NewCommandID()
	if err != nil {
		return Result{}, fmt.Errorf("create session rollback identity: %w", err)
	}
	stagedAt := window.now()
	pending := preview.journal(commandID, window, stagedAt)
	if err := authoring.StageSessionRollback(pending); err != nil {
		return Result{}, fmt.Errorf("stage session rollback: %w", err)
	}
	return Settle(ctx, runtime, pending, window, backoff)
}

// Settle observes or replays one prepared command. History projections can
// prove both before and after states. File-affecting commands are replayed only
// while the same runtime idempotency store still guarantees their response.
func Settle(
	ctx context.Context,
	runtime runtime,
	pending workbench.PendingSessionRollback,
	window ReplayWindow,
	backoff retry.Backoff,
) (Result, error) {
	result := Result{Pending: pending}
	if err := pending.Validate(); err != nil {
		return result, err
	}
	snapshot, err := runtime.GetSession(ctx, pending.SessionID)
	if err != nil {
		result.Outcome = Unknown
		return result, fmt.Errorf("read rollback outcome: %w", err)
	}
	if err := validateApplied(pending, snapshot); err == nil {
		result.Outcome, result.Snapshot = Confirmed, snapshot
		return result, nil
	}
	if err := validateBefore(pending, snapshot); err != nil {
		result.Outcome = Unknown
		return result, fmt.Errorf("authoritative session matches neither side of the pending rollback: %w", err)
	}
	if pending.Scope != agent.RestoreHistory && !replaySafe(pending, window) {
		result.Outcome = Unknown
		return result, errors.New("file rollback replay guarantee expired or belongs to another runtime")
	}

	rollbackResult, rollbackErr := mutation.Confirm(ctx, backoff, func(ctx context.Context) (agent.RollbackResult, error) {
		return runtime.RollbackSession(ctx, pending.Request())
	})
	if errors.Is(rollbackErr, agent.ErrCommandStoreMismatch) {
		result.Outcome = Unknown
		return result, fmt.Errorf("rollback session outcome is unknown: %w", rollbackErr)
	}
	after, readErr := runtime.GetSession(ctx, pending.SessionID)
	if readErr != nil {
		result.Outcome = Unknown
		var commandErr error
		if rollbackErr != nil {
			commandErr = fmt.Errorf("rollback session: %w", rollbackErr)
		}
		return result, errors.Join(commandErr, fmt.Errorf("read rollback outcome: %w", readErr))
	}
	result.Snapshot = after
	if rollbackErr == nil {
		if err := validateAcknowledged(pending, rollbackResult, after); err != nil {
			result.Outcome = Unknown
			return result, err
		}
		result.Outcome = Confirmed
		return result, nil
	}
	if err := validateApplied(pending, after); err == nil {
		result.Outcome = Confirmed
		return result, rollbackErr
	}
	if pending.Scope == agent.RestoreHistory && !mutation.OutcomeUnknown(rollbackErr) {
		if err := validateBefore(pending, after); err == nil {
			result.Outcome = Rejected
			return result, rollbackErr
		}
	}
	result.Outcome = Unknown
	return result, errors.Join(
		fmt.Errorf("rollback session: %w", rollbackErr),
		errors.New("authoritative session does not prove whether the rollback committed"),
	)
}

func validateBefore(pending workbench.PendingSessionRollback, snapshot agent.SessionSnapshot) error {
	if err := validateSnapshot(pending, snapshot); err != nil {
		return err
	}
	if snapshot.Session.Revision != pending.BeforeRevision ||
		!slices.Equal(runIDs(snapshot), pending.BeforeRunIDs) {
		return errors.New("session changed after the rollback preview; review the action again")
	}
	return nil
}

func validateApplied(pending workbench.PendingSessionRollback, snapshot agent.SessionSnapshot) error {
	if pending.Scope == agent.RestoreFiles {
		return errors.New("files-only rollback has no authoritative session outcome")
	}
	if err := validateSnapshot(pending, snapshot); err != nil {
		return err
	}
	if len(pending.BeforeRunIDs) == len(pending.AfterRunIDs) ||
		snapshot.Session.Revision <= pending.BeforeRevision ||
		!slices.Equal(runIDs(snapshot), pending.AfterRunIDs) {
		return errors.New("authoritative session does not prove the rollback committed")
	}
	return nil
}

func validateAcknowledged(
	pending workbench.PendingSessionRollback,
	result agent.RollbackResult,
	snapshot agent.SessionSnapshot,
) error {
	if err := result.Validate(); err != nil {
		return err
	}
	if err := validateSnapshot(pending, snapshot); err != nil {
		return err
	}
	if result.Session.ID != pending.SessionID || !slices.Equal(runIDs(snapshot), pending.AfterRunIDs) {
		return errors.New("rollback acknowledgement and authoritative session disagree")
	}
	droppedIDs := make([]string, len(result.Dropped))
	for index, dropped := range result.Dropped {
		droppedIDs[index] = dropped.RunID
	}
	wantDropped := pending.BeforeRunIDs[len(pending.AfterRunIDs):]
	if pending.Scope == agent.RestoreFiles {
		wantDropped = nil
	}
	if !slices.Equal(droppedIDs, wantDropped) {
		return errors.New("rollback acknowledgement reports another dropped run set")
	}
	return nil
}

func validateSnapshot(pending workbench.PendingSessionRollback, snapshot agent.SessionSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("read rollback outcome: %w", err)
	}
	if snapshot.Session.ID != pending.SessionID {
		return errors.New("read rollback outcome: runtime returned another session")
	}
	return nil
}

func runIDs(snapshot agent.SessionSnapshot) []string {
	ids := make([]string, len(snapshot.Runs))
	for index, run := range snapshot.Runs {
		ids[index] = run.ID
	}
	return ids
}

func replaySafe(pending workbench.PendingSessionRollback, window ReplayWindow) bool {
	return strings.TrimSpace(window.Namespace) != "" &&
		strings.TrimSpace(window.Namespace) == pending.ReplayNamespace &&
		!window.now().After(pending.ReplayUntil)
}

// Confirm upgrades the exact prepared journal after its result reaches the
// caller's presentation boundary.
func Confirm(authoring *workbench.Store, result Result) error {
	return authoring.ConfirmSessionRollback(result.Pending.SessionID, result.Pending.CommandID)
}

// Reject retires only the exact prepared journal after a definitive refusal.
func Reject(authoring *workbench.Store, result Result) error {
	return authoring.RejectSessionRollback(result.Pending.SessionID, result.Pending.CommandID)
}

// Recover settles every prepared rollback before sessions or drafts become
// visible. Confirmed records remain session-owned until activation consumes
// their opening input.
func Recover(
	ctx context.Context,
	runtime runtime,
	authoring *workbench.Store,
	window ReplayWindow,
	backoff retry.Backoff,
) error {
	for _, pending := range authoring.PendingSessionRollbacks() {
		if pending.Phase == workbench.SessionRollbackConfirmed {
			continue
		}
		result, err := Settle(ctx, runtime, pending, window, backoff)
		switch result.Outcome {
		case Confirmed:
			if confirmErr := Confirm(authoring, result); confirmErr != nil {
				return errors.Join(err, confirmErr)
			}
		case Rejected:
			if rejectErr := Reject(authoring, result); rejectErr != nil {
				return errors.Join(err, rejectErr)
			}
		case Unknown:
			if errors.Is(err, agent.ErrSessionNotFound) {
				if retireErr := authoring.RetireSessionState(pending.SessionID); retireErr != nil {
					return errors.Join(err, retireErr)
				}
				continue
			}
			return err
		}
	}
	return nil
}
