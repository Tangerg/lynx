package sessions

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// RollbackPlan is the atomic durable command for truncating a session back to a
// run boundary. A parked run among DropRunIDs needs no terminalization: dropping
// its record is also how it releases the session's admission slot.
type RollbackPlan struct {
	SessionID         string
	KeepMark          int
	DropRunIDs        []string
	CheckpointRootIDs []string
	// Todos is the task list the boundary held. Applying it is a NEW state commit
	// (Replace, never delete-and-rewrite): the live revision has to move forward or a
	// client holding a higher one discards the rolled-back list as stale.
	Todos TodoBoundary
}

type ForkPlan struct {
	ParentID string
	Messages []chat.Message
	// Todos is the parent's task list as of the fork boundary, seeded into the child
	// so the branch starts from the plan the conversation it copies was following.
	// Empty seeds nothing: a child with no list is a child whose list was empty.
	Todos []todo.Item
	Title string
}

// RestorePlan is the atomic durable command for replacing a session aggregate.
// It is intentionally distinct from Snapshot, the export read model: the
// explicit command makes the persistence boundary's destructive operation
// visible instead of silently accepting every snapshot-shaped value.
type RestorePlan struct {
	Session     session.Session
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []transcript.Run
	ToolResults []offload.ToolResultBlob
	// Todos is the restored task list's semantic value. It is REPLACED rather than
	// cleared-and-rewritten: the projection's revision must come out greater than
	// whatever the target session already published, and a delete would restart the
	// revision space at one — leaving a client that holds a higher revision ignoring
	// the imported value as stale.
	Todos []todo.Item
}

func restorePlan(snapshot Snapshot) RestorePlan {
	plan := RestorePlan(snapshot)
	plan.Runs = runsInParentFirstOrder(snapshot.Runs)
	return plan
}

// runsInParentFirstOrder gives persistence a creation-safe tree order while
// preserving the archive's order among peers. Snapshot validation has already
// proved that every parent exists and the graph is acyclic.
func runsInParentFirstOrder(runs []transcript.Run) []transcript.Run {
	ordered := append([]transcript.Run(nil), runs...)
	byID := make(map[string]transcript.Run, len(runs))
	for _, run := range runs {
		byID[run.ID] = run
	}
	depths := make(map[string]int, len(runs))
	var depth func(transcript.Run) int
	depth = func(run transcript.Run) int {
		if known, ok := depths[run.ID]; ok {
			return known
		}
		if run.Lineage().IsRoot() {
			depths[run.ID] = 0
			return 0
		}
		value := depth(byID[run.ParentRunID]) + 1
		depths[run.ID] = value
		return value
	}
	slices.SortStableFunc(ordered, func(left, right transcript.Run) int {
		return cmp.Compare(depth(left), depth(right))
	})
	return ordered
}

// DeletePlan removes exactly one addressed conversation. User-created forks are
// independent conversations and delegated work is represented by child Runs,
// not hidden Session rows.
type DeletePlan struct {
	SessionID string
}

// TerminalPlan is the complete durable projection for ending a parked Run tree
// by cancellation or executor-state loss. Runs is canonical postorder so every
// descendant terminalizes before its parent; the root is last. The Runs,
// interrupt Items, root-owned Pending, executor checkpoint, admission, and
// optional Goal charge all move in one transaction.
type TerminalPlan struct {
	Runs             []transcript.Run
	Items            []transcript.Item
	CheckpointRootID string
	// GoalTurn is present exactly when the root Run was admitted by an autonomous Goal.
	// Keeping it in the same write-set makes every terminal path—not only the
	// normal reducer path—charge the lease atomically with the Run transition.
	GoalTurn *goal.TurnRecord
}

// RootRun returns the root terminal projection. A valid plan always has one.
func (plan TerminalPlan) RootRun() (transcript.Run, bool) {
	if len(plan.Runs) == 0 {
		return transcript.Run{}, false
	}
	root := plan.Runs[len(plan.Runs)-1]
	return root, root.Lineage().IsRoot()
}

// Validate proves that the parked-tree terminal write-set is complete,
// canonical, owner-bound, and carries exactly the Goal accounting fact implied
// by its root terminal Run.
func (plan TerminalPlan) Validate() error {
	root, ok := plan.RootRun()
	if !ok {
		return errors.New("sessions: terminal plan must end with one root Run")
	}
	members := make([]execution.RunTreeMember, 0, len(plan.Runs))
	ownedRuns := make(map[string]struct{}, len(plan.Runs))
	actualOrder := make([]string, 0, len(plan.Runs))
	for index, run := range plan.Runs {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("sessions: terminal plan Run[%d]: %w", index, err)
		}
		if run.SessionID != root.SessionID {
			return fmt.Errorf("sessions: terminal plan Run %q belongs to Session %q, want %q", run.ID, run.SessionID, root.SessionID)
		}
		if run.Outcome == nil || root.Outcome == nil || *run.Outcome != *root.Outcome {
			return fmt.Errorf("sessions: terminal plan Run %q has a different terminal outcome", run.ID)
		}
		if _, duplicate := ownedRuns[run.ID]; duplicate {
			return fmt.Errorf("sessions: terminal plan repeats Run %q", run.ID)
		}
		ownedRuns[run.ID] = struct{}{}
		actualOrder = append(actualOrder, run.ID)
		members = append(members, execution.RunTreeMember{RunID: run.ID, Lineage: run.Lineage()})
	}
	tree, err := execution.NewRunTree(root.ID, members)
	if err != nil {
		return fmt.Errorf("sessions: terminal plan Run tree: %w", err)
	}
	if !slices.Equal(actualOrder, tree.Postorder()) {
		return errors.New("sessions: terminal plan Runs are not in canonical postorder")
	}
	if strings.TrimSpace(plan.CheckpointRootID) == "" || plan.CheckpointRootID != strings.TrimSpace(plan.CheckpointRootID) {
		return errors.New("sessions: terminal plan checkpoint root ID must be non-empty without surrounding whitespace")
	}
	seenItems := make(map[string]struct{}, len(plan.Items))
	for index, item := range plan.Items {
		_, owned := ownedRuns[item.RunID]
		if item.ID == "" || item.SessionID != root.SessionID || !owned || item.Status != transcript.ItemIncomplete {
			return fmt.Errorf("sessions: terminal plan Item[%d] is not an incomplete Item owned by its Run tree", index)
		}
		if _, duplicate := seenItems[item.ID]; duplicate {
			return fmt.Errorf("sessions: terminal plan repeats Item %q", item.ID)
		}
		seenItems[item.ID] = struct{}{}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("sessions: terminal plan Item %q: %w", item.ID, err)
		}
	}
	return validateTerminalGoalTurn(root, plan.GoalTurn)
}

func validateTerminalGoalTurn(run transcript.Run, turn *goal.TurnRecord) error {
	if run.GoalLeaseID == "" {
		if turn != nil {
			return fmt.Errorf("sessions: terminal plan non-Goal Run %q carries a Goal turn", run.ID)
		}
		return nil
	}
	if turn == nil {
		return fmt.Errorf("sessions: terminal plan Goal-owned Run %q has no Goal turn", run.ID)
	}
	if err := turn.Validate(); err != nil {
		return fmt.Errorf("sessions: terminal plan Goal turn: %w", err)
	}
	costUSD := 0.0
	if run.Metrics.Usage != nil && run.Metrics.Usage.CostUSD != nil {
		costUSD = *run.Metrics.Usage.CostUSD
	}
	if run.Outcome == nil || turn.SessionID != run.SessionID || turn.LeaseID != run.GoalLeaseID ||
		turn.RunID != run.ID || turn.Outcome != *run.Outcome || turn.CostUSD != costUSD ||
		turn.Steps != run.Metrics.Steps || !turn.CompletedAt.Equal(run.FinishedAt) {
		return fmt.Errorf("sessions: terminal plan Goal turn differs from Run %q", run.ID)
	}
	return nil
}
