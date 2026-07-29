package sessions

import (
	"cmp"
	"slices"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// RollbackPlan is the atomic durable command for truncating a session back to a
// run boundary. A parked run among DropRunIDs needs no terminalization: dropping
// its record is also how it releases the session's admission slot.
type RollbackPlan struct {
	SessionID      string
	KeepMark       int
	DropRunIDs     []string
	DropSessionIDs []string
	ProcessIDs     []string
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

// DeletePlan is the post-order session set removed by one delete cascade. It
// contains the addressed session plus its owned internal-subtask descendants;
// user-created forks are independent and are not included.
type DeletePlan struct {
	SessionIDs []string
}

// TerminalPlan is the complete durable projection for ending a parked run by
// cancellation or executor-state loss. The run becomes terminal, its interrupt
// items become incomplete, and its open-interrupt/admission records are closed
// in the same transaction.
type TerminalPlan struct {
	Run       transcript.Run
	Items     []transcript.Item
	ProcessID string
}
