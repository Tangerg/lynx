package sessions

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// MaterialSnapshot is the coherent durable state needed to reconstruct one
// mounted Session. It is a live read model, so active Runs and open interrupts
// are valid members and Plan revision metadata is retained.
type MaterialSnapshot struct {
	Session    session.Session
	Items      []transcript.Item
	Runs       []run.Run
	Interrupts []runs.Pending
	Plan       plan.State
	Goal       *goal.Goal
}

// MaterialSnapshot reads the complete mounted-session projection at one
// database snapshot. No process-local admission is required: concurrent writes
// either precede or follow the storage transaction and can never split the
// returned Run, interrupt, transcript, and Plan facts.
func (c *Coordinator) MaterialSnapshot(ctx context.Context, sessionID string) (MaterialSnapshot, error) {
	snapshot, err := c.materialSnapshots.ReadMaterialSnapshot(ctx, sessionID)
	if err != nil {
		return MaterialSnapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return MaterialSnapshot{}, err
	}
	return snapshot, nil
}

// Validate checks the cross-projection identities a storage transaction must
// preserve before the snapshot crosses the Application boundary.
func (m MaterialSnapshot) Validate() error {
	if err := m.Session.Validate(); err != nil {
		return fmt.Errorf("sessions: material snapshot Session: %w", err)
	}
	sessionID := m.Session.ID()
	runsByID := make(map[string]run.Run, len(m.Runs))
	for _, record := range m.Runs {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("sessions: material snapshot Run %q: %w", record.ID(), err)
		}
		if record.SessionID() != sessionID {
			return fmt.Errorf("sessions: material snapshot Run %q belongs to Session %q, want %q", record.ID(), record.SessionID(), sessionID)
		}
		if _, duplicate := runsByID[record.ID()]; duplicate {
			return fmt.Errorf("sessions: material snapshot repeats Run %q", record.ID())
		}
		runsByID[record.ID()] = record
	}
	itemsByID := make(map[string]transcript.Item, len(m.Items))
	for _, item := range m.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("sessions: material snapshot Item %q: %w", item.ID(), err)
		}
		if item.SessionID() != sessionID {
			return fmt.Errorf("sessions: material snapshot Item %q belongs to Session %q, want %q", item.ID(), item.SessionID(), sessionID)
		}
		owner, found := runsByID[item.RunID()]
		if !found {
			return fmt.Errorf("sessions: material snapshot Item %q references unknown Run %q", item.ID(), item.RunID())
		}
		if item.Status() == transcript.ItemRunning && owner.State().IsTerminal() {
			return fmt.Errorf("sessions: material snapshot terminal Run Item %q is still running", item.ID())
		}
		if _, duplicate := itemsByID[item.ID()]; duplicate {
			return fmt.Errorf("sessions: material snapshot repeats Item %q", item.ID())
		}
		itemsByID[item.ID()] = item
	}
	if err := validateSnapshotRunTree(m.Runs, itemsByID); err != nil {
		return fmt.Errorf("sessions: material snapshot Run tree: %w", err)
	}
	interruptsByRoot := make(map[string]struct{}, len(m.Interrupts))
	for _, pending := range m.Interrupts {
		if err := pending.ValidateProjection(m.Runs, m.Items); err != nil {
			return fmt.Errorf("sessions: material snapshot interrupt %q: %w", pending.RootRunID, err)
		}
		if pending.SessionID != sessionID {
			return fmt.Errorf("sessions: material snapshot interrupt %q belongs to Session %q, want %q", pending.RootRunID, pending.SessionID, sessionID)
		}
		root, found := runsByID[pending.RootRunID]
		if !found {
			return fmt.Errorf("sessions: material snapshot interrupt references unknown root Run %q", pending.RootRunID)
		}
		if root.Lineage().IsChild() || root.State() != run.Waiting {
			return fmt.Errorf("sessions: material snapshot interrupt Run %q is not a waiting root", pending.RootRunID)
		}
		if _, duplicate := interruptsByRoot[pending.RootRunID]; duplicate {
			return fmt.Errorf("sessions: material snapshot repeats interrupt root %q", pending.RootRunID)
		}
		interruptsByRoot[pending.RootRunID] = struct{}{}
	}
	for _, record := range m.Runs {
		if record.State() != run.Waiting {
			continue
		}
		rootRunID := record.Lineage().TreeRootID(record.ID())
		if _, found := interruptsByRoot[rootRunID]; !found {
			return fmt.Errorf(
				"sessions: material snapshot waiting Run %q has no Pending owner for root %q",
				record.ID(),
				rootRunID,
			)
		}
	}
	if err := m.Plan.Validate(); err != nil {
		return fmt.Errorf("sessions: material snapshot Plan: %w", err)
	}
	if m.Goal != nil {
		if err := m.Goal.ValidateSnapshot(); err != nil {
			return fmt.Errorf("sessions: material snapshot Goal: %w", err)
		}
		if m.Goal.SessionID != sessionID {
			return fmt.Errorf(
				"sessions: material snapshot Goal belongs to Session %q, want %q",
				m.Goal.SessionID,
				sessionID,
			)
		}
	}
	return nil
}
