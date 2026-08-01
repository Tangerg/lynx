package runs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

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
	Outcome   execution.Outcome
	Items     []transcript.Item
	Run       *transcript.Run
	GoalTurn  *goal.TurnRecord
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
		if c.Run != nil || c.GoalTurn != nil || c.ObsoleteCheckpointRootID != "" {
			return errors.New("runs: unchanged event commit carries lifecycle facts")
		}
		return nil
	case StateSuspend:
		if c.Run == nil || c.Run.State != execution.Interrupted {
			return errors.New("runs: suspend event commit has no interrupted Run")
		}
		if c.GoalTurn != nil || c.ObsoleteCheckpointRootID != "" {
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
		// The reducer cannot know the final conversation watermark. The effects
		// adapter resolves it inside the same transaction that terminalizes this
		// Run; every other terminal fact must already satisfy the domain invariant.
		validatedRun.MessageMark = 0
	}
	if err := validatedRun.Validate(); err != nil {
		return fmt.Errorf("runs: event commit Run: %w", err)
	}
	if c.State == StateSuspend {
		return nil
	}
	return validateTerminalGoalTurn(*c.Run, c.GoalTurn)
}

func validateTerminalGoalTurn(run transcript.Run, turn *goal.TurnRecord) error {
	if run.GoalLeaseID == "" {
		if turn != nil {
			return fmt.Errorf("runs: non-Goal Run %q carries a Goal turn", run.ID)
		}
		return nil
	}
	if !run.Lineage().IsRoot() {
		return fmt.Errorf("runs: child Run %q carries a root Goal lease", run.ID)
	}
	if turn == nil {
		return fmt.Errorf("runs: Goal-owned terminal Run %q has no Goal turn", run.ID)
	}
	if err := turn.Validate(); err != nil {
		return fmt.Errorf("runs: terminal Goal turn: %w", err)
	}
	costUSD := 0.0
	if run.Metrics.Usage != nil && run.Metrics.Usage.CostUSD != nil {
		costUSD = *run.Metrics.Usage.CostUSD
	}
	if run.Outcome == nil || turn.SessionID != run.SessionID || turn.LeaseID != run.GoalLeaseID ||
		turn.RunID != run.ID || turn.Outcome != *run.Outcome || turn.CostUSD != costUSD ||
		turn.Steps != run.Metrics.Steps || !turn.CompletedAt.Equal(run.FinishedAt) {
		return fmt.Errorf("runs: Goal turn differs from terminal Run %q", run.ID)
	}
	return nil
}

func (c EventCommit) isEmpty() bool {
	return len(c.Items) == 0 &&
		c.Run == nil &&
		c.GoalTurn == nil &&
		c.ObsoleteCheckpointRootID == "" &&
		c.State == StateUnchanged
}

// TreeBarrierCommit is the one durable write-set produced when any executor
// suspension stops a Run tree. Pending owns the complete continuation hand-off;
// Runs contains one StateSuspend commit for every active Run in deterministic
// postorder. No individual Run commit may write or consume the root-owned set.
type TreeBarrierCommit struct {
	Pending    interrupts.Pending
	Runs       []EventCommit
	Checkpoint execution.ExecutorCheckpoint
}
