package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ForkSpec describes where a session fork should branch.
type ForkSpec struct {
	ParentID  string
	FromRunID string
	Title     string
}

// ForkBoundary is where a fork branches: the parent history prefix the child is
// seeded with, and the run that prefix stops at. They travel together because the
// child's session-scoped state is copied from that same run's boundary — a fork
// whose conversation and Plan came from different runs would hand the branch
// a plan that never went with what it remembers.
type ForkBoundary struct {
	Messages []chat.Message
	// RunIDs are the terminal Run facts whose user-visible transcript belongs to
	// Messages. A fork remaps them into the child instead of leaving the model with
	// context the client cannot see through runs.list/items.list.
	RunIDs []string
	// RunID is the boundary run, empty when the parent has no terminal run to
	// branch from (nothing to copy).
	RunID string
}

// ResolveForkBoundary applies a durable run boundary to parent history.
// Non-terminal runs never contribute messages: their current tail can still
// change and therefore is not a portable fork boundary. An explicit target
// must itself be terminal; an implicit whole-conversation fork stops at the
// latest terminal run.
func ResolveForkBoundary(msgs []chat.Message, runs []run.Run, fromRunID string) (ForkBoundary, error) {
	ordered := slices.Clone(runs)
	slices.SortStableFunc(ordered, func(a, b run.Run) int {
		return a.CreatedAt().Compare(b.CreatedAt())
	})
	for _, run := range ordered {
		if run.State().IsTerminal() && (run.MessageMark() < 0 || run.MessageMark() > len(msgs)) {
			return ForkBoundary{}, fmt.Errorf("sessions: terminal run %q has invalid message watermark %d", run.ID(), run.MessageMark())
		}
	}

	// A root Run and the child Runs it spawned are one Run boundary. A terminal
	// child inside an active root does not make that active Run portable, so
	// include a group only when every run in it is terminal.
	terminal := make([]transcript.RunNode, 0, len(ordered))
	targetTerminal := fromRunID == ""
	for start := 0; start < len(ordered); {
		if ordered[start].Lineage().IsChild() {
			return ForkBoundary{}, fmt.Errorf("sessions: run timeline starts a group with child Run %q", ordered[start].ID())
		}
		end := start + 1
		for end < len(ordered) && ordered[end].Lineage().IsChild() {
			end++
		}
		stable := true
		for _, run := range ordered[start:end] {
			stable = stable && run.State().IsTerminal()
		}
		if stable {
			for _, run := range ordered[start:end] {
				terminal = append(terminal, transcript.RunNode{
					ID: run.ID(), SpawnedByItemID: run.Lineage().SpawnedByItemID,
					CreatedAt: run.CreatedAt(), MessageMark: run.MessageMark(),
				})
				if run.ID() == fromRunID {
					targetTerminal = true
				}
			}
		}
		start = end
	}
	if !targetTerminal {
		return ForkBoundary{}, transcript.ErrRunNotFound
	}
	if len(terminal) == 0 {
		return ForkBoundary{}, nil
	}
	if fromRunID == "" {
		fromRunID = terminal[len(terminal)-1].ID
	}
	b, err := transcript.Timeline(terminal).BoundaryAt(fromRunID, false)
	if err != nil {
		return ForkBoundary{}, err
	}
	kept := terminal[:len(terminal)-len(b.Dropped)]
	runIDs := make([]string, len(kept))
	for index, node := range kept {
		runIDs[index] = node.ID
	}
	return ForkBoundary{
		Messages: slices.Clone(msgs[:b.KeepMessageMark]),
		RunIDs:   runIDs,
		RunID:    b.KeepRunID,
	}, nil
}

// Fork creates a child session, seeds it with the resolved parent history prefix
// and the Plan that boundary held, and renames it as ONE atomic write-set
// (§8.1). The application resolves the boundary and commits the branch through
// its persistence port.
func (c *Coordinator) Fork(ctx context.Context, spec ForkSpec) (session.Session, error) {
	if c.snapshots == nil || c.writes == nil {
		return session.Session{}, errors.New("sessions: fork persistence is unavailable")
	}
	if c.newID == nil {
		return session.Session{}, errors.New("sessions: Session identity generator is unavailable")
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, spec.ParentID)
	if err != nil {
		return session.Session{}, err
	}
	boundary, err := ResolveForkBoundary(snapshot.Messages, snapshot.Runs, spec.FromRunID)
	if err != nil {
		return session.Session{}, err
	}
	// A child starts fresh, so an unrecorded boundary and a recorded empty one seed
	// the same nothing — the distinction a rollback needs has no branch to make here.
	plan, err := c.planBoundary(ctx, boundary.RunID)
	if err != nil {
		return session.Session{}, err
	}
	planReplacement, err := c.prepareInitialPlanReplacement(plan.Steps)
	if err != nil {
		return session.Session{}, err
	}
	child, err := snapshot.Session.Fork(c.newID(), spec.Title, c.now())
	if err != nil {
		return session.Session{}, err
	}
	forked, err := c.copyForkSnapshot(snapshot, child, boundary, plan.Steps)
	if err != nil {
		return session.Session{}, err
	}
	if _, err := c.writes.ApplyFork(ctx, ForkPlan{
		ParentID:        spec.ParentID,
		Child:           child,
		Messages:        forked.Messages,
		Items:           forked.Items,
		Runs:            forked.Runs,
		ToolResults:     forked.ToolResults,
		PlanReplacement: planReplacement,
	}); err != nil {
		return session.Session{}, err
	}
	c.publishSessionMoved(child.ID())
	if len(forked.Runs) > 0 {
		c.publishRunsMoved(child.ID(), forked.Runs)
	}
	if planReplacement != nil {
		c.publishStateMoved(child.ID())
	}
	return child, nil
}
