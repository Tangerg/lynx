package runs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type ItemReplacement struct {
	Expected    transcript.Item
	Replacement transcript.Item
}

// WaitingSubtreeCancellationCommit is the immutable write-set for canceling a
// child while its Run tree is waiting.
type WaitingSubtreeCancellationCommit struct {
	RootRunID        string
	TargetRunID      string
	SessionID        string
	RootRun          transcript.Run
	ExpectedPending  Pending
	RemainingPending *Pending
	Checkpoint       ExecutorCheckpoint
	TerminalRuns     []transcript.Run
	TerminalItems    []ItemReplacement
	ParentItem       ItemReplacement
	Resume           *run.TreeResumeDraft
	OpeningEvents    []EventCommit
}

type WaitingSubtreeCancellationResult struct {
	TargetRun transcript.Run
	RootRun   transcript.Run
}

// OpeningCommit is the atomic acceptance write-set for one fresh admission or
// one continuation.
type OpeningCommit struct {
	Admit            *run.RunDraft
	Resume           *run.TreeResumeDraft
	ScheduledSession *session.Session
	SessionModel     *SessionModelUpdate
	ScheduleFiring   string
	Events           []EventCommit
}

// SessionModelUpdate is committed with the Run admission that established it.
type SessionModelUpdate struct {
	SessionID string
	Model     string
}

type StateChange uint8

const (
	StateUnchanged StateChange = iota
	StateSuspend
	StateTerminalize
)

type EventCommit struct {
	RunID     string
	SessionID string
	State     StateChange
	Outcome   run.Outcome
	Items     []transcript.Item
	Run       *transcript.Run
	GoalRun   *goal.RunRecord
	// ObsoleteCheckpointRootID identifies the executor checkpoint aggregate the
	// root Run terminal makes obsolete. Child terminal commits leave it empty.
	ObsoleteCheckpointRootID string
}

// Validate proves that one event projection is owner-bound and that any Goal
// charge is exactly the accounting fact implied by its terminal Run.
func (c EventCommit) Validate() error {
	if strings.TrimSpace(c.RunID) == "" || c.RunID != strings.TrimSpace(c.RunID) {
		return errors.New("runs: event commit Run ID must be non-empty without surrounding whitespace")
	}
	if strings.TrimSpace(c.SessionID) == "" || c.SessionID != strings.TrimSpace(c.SessionID) {
		return errors.New("runs: event commit Session ID must be non-empty without surrounding whitespace")
	}
	if c.ObsoleteCheckpointRootID != strings.TrimSpace(c.ObsoleteCheckpointRootID) {
		return errors.New("runs: event commit checkpoint root ID has surrounding whitespace")
	}
	seenItems := make(map[string]struct{}, len(c.Items))
	for index, item := range c.Items {
		if item.ID == "" || item.RunID != c.RunID || item.SessionID != c.SessionID {
			return fmt.Errorf("runs: event commit Item[%d] is not owned by Run %q", index, c.RunID)
		}
		if _, duplicate := seenItems[item.ID]; duplicate {
			return fmt.Errorf("runs: event commit repeats Item %q", item.ID)
		}
		seenItems[item.ID] = struct{}{}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("runs: event commit Item %q: %w", item.ID, err)
		}
	}

	switch c.State {
	case StateUnchanged:
		if c.Run != nil || c.GoalRun != nil || c.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: unchanged event commit carries lifecycle facts")
		}
		return nil
	case StateSuspend:
		if c.Run == nil || c.Run.State != run.Waiting {
			return errors.New("runs: suspend event commit has no waiting Run")
		}
		if c.GoalRun != nil || c.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: suspend event commit carries terminal facts")
		}
	case StateTerminalize:
		if c.Run == nil || !c.Run.State.IsTerminal() || c.Run.Outcome == nil || *c.Run.Outcome != c.Outcome {
			return errors.New("runs: terminal event commit has no matching terminal Run")
		}
	default:
		return fmt.Errorf("runs: event commit has unknown state change %d", c.State)
	}

	if c.Run.ID != c.RunID || c.Run.SessionID != c.SessionID {
		return errors.New("runs: event commit Run ownership differs from its envelope")
	}
	validatedRun := *c.Run
	if c.State == StateTerminalize && validatedRun.MessageMark == transcript.UnknownMessageMark {
		// The reducer cannot know the final conversation watermark. The terminal
		// transaction resolves it while committing this Run; every other terminal
		// fact must already satisfy the domain invariant.
		validatedRun.MessageMark = 0
	}
	if err := validatedRun.Validate(); err != nil {
		return fmt.Errorf("runs: event commit Run: %w", err)
	}
	if c.State == StateSuspend {
		return nil
	}
	return validateTerminalGoalRun(*c.Run, c.GoalRun)
}

func validateTerminalGoalRun(run transcript.Run, record *goal.RunRecord) error {
	if run.GoalLeaseID == "" {
		if record != nil {
			return fmt.Errorf("runs: non-Goal Run %q carries a Goal Run", run.ID)
		}
		return nil
	}
	if !run.Lineage().IsRoot() {
		return fmt.Errorf("runs: child Run %q carries a root Goal lease", run.ID)
	}
	if record == nil {
		return fmt.Errorf("runs: Goal-owned terminal Run %q has no Goal Run", run.ID)
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("runs: terminal Goal Run: %w", err)
	}
	costUSD := 0.0
	if run.Metrics.Usage != nil && run.Metrics.Usage.CostUSD != nil {
		costUSD = *run.Metrics.Usage.CostUSD
	}
	if run.Outcome == nil || record.SessionID != run.SessionID || record.LeaseID != run.GoalLeaseID ||
		record.RunID != run.ID || record.Outcome != *run.Outcome || record.CostUSD != costUSD ||
		record.Steps != run.Metrics.Steps || !record.CompletedAt.Equal(run.FinishedAt) {
		return fmt.Errorf("runs: Goal Run differs from terminal Run %q", run.ID)
	}
	return nil
}

func (c EventCommit) isEmpty() bool {
	return len(c.Items) == 0 &&
		c.Run == nil &&
		c.GoalRun == nil &&
		c.ObsoleteCheckpointRootID == "" &&
		c.State == StateUnchanged
}

// Validate proves that the opening is exactly one fresh admission or one tree
// continuation and that every accompanying projection is item-only. Persistence
// Port implementations may reject unavailable stores or concurrent state changes, but they
// do not reinterpret this application write-set.
func (c OpeningCommit) Validate() error {
	if (c.Admit == nil) == (c.Resume == nil) {
		return errors.New("runs: opening requires exactly one admission action")
	}
	if c.Admit != nil {
		if c.ScheduledSession != nil && c.ScheduledSession.ID != c.Admit.SessionID {
			return errors.New("runs: opening scheduled Session differs from admitted Run")
		}
		if c.SessionModel != nil {
			if c.SessionModel.SessionID != c.Admit.SessionID {
				return errors.New("runs: opening Session model differs from admitted Run")
			}
			if strings.TrimSpace(c.SessionModel.Model) == "" || c.SessionModel.Model != strings.TrimSpace(c.SessionModel.Model) {
				return errors.New("runs: opening Session model must be non-empty without surrounding whitespace")
			}
		}
		if c.ScheduleFiring != strings.TrimSpace(c.ScheduleFiring) {
			return errors.New("runs: opening schedule firing has surrounding whitespace")
		}
	} else if c.ScheduledSession != nil || c.SessionModel != nil || c.ScheduleFiring != "" {
		return errors.New("runs: resumed opening carries fresh-run facts")
	}
	for index, commit := range c.Events {
		if err := commit.Validate(); err != nil {
			return fmt.Errorf("runs: opening event[%d]: %w", index, err)
		}
		if commit.State != StateUnchanged || len(commit.Items) == 0 {
			return fmt.Errorf("runs: opening event[%d] is not an item-only projection", index)
		}
	}
	return nil
}

// TreeBarrierCommit is the one durable write-set produced when any executor
// suspension stops a Run tree. Pending owns the complete continuation hand-off;
// Runs contains one StateSuspend commit for every active Run in deterministic
// postorder. No individual Run commit may write or consume the root-owned set.
type TreeBarrierCommit struct {
	Pending    Pending
	Runs       []EventCommit
	Checkpoint ExecutorCheckpoint
}

// Validate proves that the barrier is the complete suspension projection for
// the pending continuation tree and that its checkpoint belongs to the same
// run. The Effects port only persists this already-defined write-set.
func (c TreeBarrierCommit) Validate() error {
	if err := c.Pending.Validate(); err != nil {
		return fmt.Errorf("runs: tree barrier Pending: %w", err)
	}
	rootContinuation, ok := c.Pending.RootContinuation()
	if !ok {
		return errors.New("runs: tree barrier has no root continuation")
	}
	if err := c.Checkpoint.ValidateOwnership(rootContinuation.ProcessID, c.Pending.SessionID); err != nil {
		return fmt.Errorf("runs: tree barrier checkpoint ownership: %w", err)
	}
	if c.Checkpoint.Scope.GoalLeaseID != c.Pending.GoalLeaseID {
		return fmt.Errorf(
			"runs: tree barrier checkpoint goal lease %q does not match Pending %q: %w",
			c.Checkpoint.Scope.GoalLeaseID,
			c.Pending.GoalLeaseID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if c.Checkpoint.ModelSelection != rootContinuation.ModelSelection {
		return fmt.Errorf("runs: tree barrier checkpoint model differs from root continuation: %w", ErrInvalidExecutorCheckpoint)
	}
	if c.Checkpoint.Limits != rootContinuation.Limits {
		return fmt.Errorf("runs: tree barrier checkpoint limits differ from root continuation: %w", ErrInvalidExecutorCheckpoint)
	}
	if len(c.Runs) != len(c.Pending.Continuations) {
		return fmt.Errorf(
			"runs: tree barrier has %d Run commits for %d continuations",
			len(c.Runs),
			len(c.Pending.Continuations),
		)
	}
	continuations := make(map[string]Continuation, len(c.Pending.Continuations))
	for _, continuation := range c.Pending.Continuations {
		continuations[continuation.RunID] = continuation
	}
	seen := make(map[string]struct{}, len(c.Runs))
	for index, commit := range c.Runs {
		if err := commit.Validate(); err != nil {
			return fmt.Errorf("runs: tree barrier Run[%d]: %w", index, err)
		}
		if commit.State != StateSuspend || commit.Run == nil || commit.Run.State != run.Waiting {
			return fmt.Errorf("runs: tree barrier Run[%d] is not an waiting Run projection", index)
		}
		if commit.SessionID != c.Pending.SessionID || commit.Run.SessionID != c.Pending.SessionID {
			return fmt.Errorf("runs: tree barrier Run[%d] Session differs from Pending", index)
		}
		continuation, exists := continuations[commit.RunID]
		if !exists {
			return fmt.Errorf("runs: tree barrier Run[%d] has no continuation", index)
		}
		if commit.Run.Lineage() != continuation.Lineage ||
			commit.Run.ModelSelection != continuation.ModelSelection ||
			!commit.Run.CreatedAt.Equal(continuation.RunCreatedAt) ||
			!commit.Run.Metrics.Equal(continuation.Metrics) ||
			commit.Run.Limits != continuation.Limits {
			return fmt.Errorf("runs: tree barrier Run[%d] differs from its continuation", index)
		}
		if !commit.Run.Capabilities.Equal(c.Pending.Capabilities) {
			return fmt.Errorf("runs: tree barrier Run[%d] capabilities differ from Pending", index)
		}
		if commit.RunID == c.Pending.RootRunID {
			if commit.Run.GoalLeaseID != c.Pending.GoalLeaseID {
				return errors.New("runs: tree barrier root Run goal lease differs from Pending")
			}
		} else if commit.Run.GoalLeaseID != "" {
			return fmt.Errorf("runs: tree barrier child Run[%d] carries a root Goal lease", index)
		}
		if _, duplicate := seen[commit.RunID]; duplicate {
			return fmt.Errorf("runs: tree barrier repeats Run %q", commit.RunID)
		}
		seen[commit.RunID] = struct{}{}
	}
	return nil
}
