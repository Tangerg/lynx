package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// ReadSnapshot reserves the session's single-writer slot and reads its complete
// canonical state coherently. Active and parked runs are rejected because their
// executor state is process-local and therefore cannot be represented by a
// portable session artifact.
func (c *Coordinator) ReadSnapshot(ctx context.Context, sessionID string) (Snapshot, error) {
	admission, err := c.ClaimRunSlot(ctx, sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	defer admission.Release()
	if c.snapshots == nil {
		return Snapshot{}, errors.New("sessions: snapshot reader is unavailable")
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

// Validate checks a snapshot's referential integrity — the session id is present
// and every run/item belongs to it — before the coordinator hands it out.
func (snapshot Snapshot) Validate() error {
	if snapshot.Session.ID == "" {
		return errors.New("sessions: snapshot session id is required")
	}
	runs := make(map[string]struct{}, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		if run.ID == "" || run.SessionID != snapshot.Session.ID {
			return fmt.Errorf("sessions: snapshot run %q belongs to session %q, want %q", run.ID, run.SessionID, snapshot.Session.ID)
		}
		if _, exists := runs[run.ID]; exists {
			return fmt.Errorf("sessions: snapshot contains duplicate run %q", run.ID)
		}
		if !run.State.IsTerminal() {
			return fmt.Errorf("sessions: snapshot run %q is %s, want terminal", run.ID, run.State)
		}
		if run.Outcome == nil || run.Result == nil {
			return fmt.Errorf("sessions: snapshot terminal run %q has no outcome or result", run.ID)
		}
		expected, ok := execution.Running.Terminate(*run.Outcome)
		if !ok || expected != run.State {
			return fmt.Errorf("sessions: snapshot run %q state %s does not match outcome %s", run.ID, run.State, run.Outcome)
		}
		if run.FinishedAt.IsZero() || len(run.Interrupts) != 0 {
			return fmt.Errorf("sessions: snapshot terminal run %q has an incomplete terminal boundary", run.ID)
		}
		if (*run.Outcome == execution.OutcomeError) != (run.Result.Error != nil) {
			return fmt.Errorf("sessions: snapshot run %q error result does not match outcome %s", run.ID, run.Outcome)
		}
		if run.Result.Steps < 0 || run.Result.Duration < 0 {
			return fmt.Errorf("sessions: snapshot run %q has negative result accounting", run.ID)
		}
		if err := validateSnapshotUsage(run.Result.Usage); err != nil {
			return fmt.Errorf("sessions: snapshot run %q usage: %w", run.ID, err)
		}
		if err := validateSnapshotProblem(run.Result.Error, transcript.RunProblem); err != nil {
			return fmt.Errorf("sessions: snapshot run %q problem: %w", run.ID, err)
		}
		if run.MessageMark < 0 || run.MessageMark > len(snapshot.Messages) {
			return fmt.Errorf("sessions: snapshot run %q has invalid message watermark %d", run.ID, run.MessageMark)
		}
		runs[run.ID] = struct{}{}
	}
	items := make(map[string]transcript.Item, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.ID == "" || item.SessionID != snapshot.Session.ID {
			return fmt.Errorf("sessions: snapshot item %q belongs to session %q, want %q", item.ID, item.SessionID, snapshot.Session.ID)
		}
		if _, exists := items[item.ID]; exists {
			return fmt.Errorf("sessions: snapshot contains duplicate item %q", item.ID)
		}
		items[item.ID] = item
		if _, found := runs[item.RunID]; !found {
			return fmt.Errorf("sessions: snapshot item %q references unknown run %q", item.ID, item.RunID)
		}
		switch item.Status {
		case transcript.ItemCompleted, transcript.ItemIncomplete:
		case transcript.ItemRunning:
			return fmt.Errorf("sessions: snapshot terminal run item %q is still running", item.ID)
		default:
			return fmt.Errorf("sessions: snapshot item %q has unknown status %d", item.ID, item.Status)
		}
		if item.Error != nil && (item.Kind != transcript.ToolCall || item.Status != transcript.ItemIncomplete) {
			return fmt.Errorf("sessions: snapshot item %q has an invalid tool error", item.ID)
		}
		if err := validateSnapshotItem(item); err != nil {
			return fmt.Errorf("sessions: snapshot item %q: %w", item.ID, err)
		}
	}
	if err := validateSnapshotRunTree(snapshot.Runs, items); err != nil {
		return err
	}
	return snapshot.ValidateToolResults()
}

func validateSnapshotRunTree(runs []transcript.Run, items map[string]transcript.Item) error {
	parents := make(map[string]string, len(runs))
	for _, run := range runs {
		if run.SpawnedByItemID == "" {
			continue
		}
		item, found := items[run.SpawnedByItemID]
		if !found {
			return fmt.Errorf("sessions: snapshot run %q references unknown spawning item %q", run.ID, run.SpawnedByItemID)
		}
		if item.Kind != transcript.ToolCall {
			return fmt.Errorf("sessions: snapshot run %q spawning item %q is not a tool call", run.ID, run.SpawnedByItemID)
		}
		if item.RunID == run.ID {
			return fmt.Errorf("sessions: snapshot run %q is spawned by its own item", run.ID)
		}
		parents[run.ID] = item.RunID
	}

	states := make(map[string]uint8, len(runs))
	var visit func(string) error
	visit = func(runID string) error {
		switch states[runID] {
		case 1:
			return fmt.Errorf("sessions: snapshot run tree contains a cycle at %q", runID)
		case 2:
			return nil
		}
		states[runID] = 1
		if parentID := parents[runID]; parentID != "" {
			if err := visit(parentID); err != nil {
				return err
			}
		}
		states[runID] = 2
		return nil
	}
	for _, run := range runs {
		if err := visit(run.ID); err != nil {
			return err
		}
	}
	return nil
}
