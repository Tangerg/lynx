package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// ExportResult is the complete result of a session archive use case. The
// archive and its presentation are derived while the same session admission is
// held, so Delivery cannot accidentally pair one revision's archive with a
// later revision's session view.
type ExportResult struct {
	Session  SessionView
	Snapshot PortableSnapshot
	Items    []transcript.Item
}

// ExportSession reserves the session's single-writer slot and derives its
// portable archive and presentation from one coherent canonical state. Active
// and parked runs are rejected because their executor state is process-local
// and therefore cannot be represented by a portable session artifact.
func (c *Coordinator) ExportSession(ctx context.Context, sessionID string) (ExportResult, error) {
	admission, err := c.ClaimRunSlot(ctx, sessionID)
	if err != nil {
		return ExportResult{}, err
	}
	defer admission.Release()
	if c.snapshots == nil {
		return ExportResult{}, errors.New("sessions: snapshot reader is unavailable")
	}
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return ExportResult{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return ExportResult{}, err
	}
	portable, err := snapshot.PortableSnapshot()
	if err != nil {
		return ExportResult{}, fmt.Errorf("sessions: prepare portable snapshot: %w", err)
	}
	view, err := c.view(ctx, snapshot.Session, SessionIdle)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Session: view, Snapshot: portable, Items: snapshot.Items}, nil
}

// Validate checks a snapshot's referential integrity — the session id is present
// and every run/item belongs to it — before the coordinator hands it out.
func (snapshot Snapshot) Validate() error {
	if snapshot.Session.ID == "" {
		return errors.New("sessions: snapshot session id is required")
	}
	runs, err := snapshot.validateRuns()
	if err != nil {
		return err
	}
	items, err := snapshot.validateItems(runs)
	if err != nil {
		return err
	}
	if err := validateSnapshotRunTree(snapshot.Runs, items); err != nil {
		return err
	}
	return snapshot.ValidateToolResults()
}

func (snapshot Snapshot) validateRuns() (map[string]struct{}, error) {
	runs := make(map[string]struct{}, len(snapshot.Runs))
	for _, run := range snapshot.Runs {
		if run.ID == "" || run.SessionID != snapshot.Session.ID {
			return nil, fmt.Errorf("sessions: snapshot run %q belongs to session %q, want %q", run.ID, run.SessionID, snapshot.Session.ID)
		}
		if _, exists := runs[run.ID]; exists {
			return nil, fmt.Errorf("sessions: snapshot contains duplicate run %q", run.ID)
		}
		// A snapshot is a portable record of finished work, so only a terminal Run
		// belongs in one; whether its facts hold together is the Run's own rule.
		if !run.State.IsTerminal() {
			return nil, fmt.Errorf("sessions: snapshot run %q is %s, want terminal", run.ID, run.State)
		}
		if err := run.Validate(); err != nil {
			return nil, fmt.Errorf("sessions: snapshot run %q: %w", run.ID, err)
		}
		if run.MessageMark > len(snapshot.Messages) {
			return nil, fmt.Errorf("sessions: snapshot run %q has invalid message watermark %d", run.ID, run.MessageMark)
		}
		runs[run.ID] = struct{}{}
	}
	return runs, nil
}

func (snapshot Snapshot) validateItems(runs map[string]struct{}) (map[string]transcript.Item, error) {
	items := make(map[string]transcript.Item, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.ID == "" || item.SessionID != snapshot.Session.ID {
			return nil, fmt.Errorf("sessions: snapshot item %q belongs to session %q, want %q", item.ID, item.SessionID, snapshot.Session.ID)
		}
		if _, exists := items[item.ID]; exists {
			return nil, fmt.Errorf("sessions: snapshot contains duplicate item %q", item.ID)
		}
		items[item.ID] = item
		if _, found := runs[item.RunID]; !found {
			return nil, fmt.Errorf("sessions: snapshot item %q references unknown run %q", item.ID, item.RunID)
		}
		switch item.Status {
		case transcript.ItemCompleted, transcript.ItemIncomplete:
		case transcript.ItemRunning:
			return nil, fmt.Errorf("sessions: snapshot terminal run item %q is still running", item.ID)
		default:
			return nil, fmt.Errorf("sessions: snapshot item %q has unknown status %d", item.ID, item.Status)
		}
		if item.Error != nil && (item.Kind != transcript.ToolCall || item.Status != transcript.ItemIncomplete) {
			return nil, fmt.Errorf("sessions: snapshot item %q has an invalid tool error", item.ID)
		}
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("sessions: snapshot item %q: %w", item.ID, err)
		}
	}
	return items, nil
}

func validateSnapshotRunTree(runs []transcript.Run, items map[string]transcript.Item) error {
	byID := make(map[string]transcript.Run, len(runs))
	for _, run := range runs {
		byID[run.ID] = run
	}
	parents := make(map[string]string, len(runs))
	for _, run := range runs {
		if run.Lineage().IsRoot() {
			continue
		}
		parent, parentFound := byID[run.ParentRunID]
		if !parentFound {
			return fmt.Errorf("sessions: snapshot child run %q references unknown parent %q", run.ID, run.ParentRunID)
		}
		root, rootFound := byID[run.RootRunID]
		if !rootFound {
			return fmt.Errorf("sessions: snapshot child run %q references unknown root %q", run.ID, run.RootRunID)
		}
		if !root.Lineage().IsRoot() {
			return fmt.Errorf("sessions: snapshot child run %q names child %q as its root", run.ID, run.RootRunID)
		}
		item, found := items[run.SpawnedByItemID]
		if !found {
			return fmt.Errorf("sessions: snapshot run %q references unknown spawning item %q", run.ID, run.SpawnedByItemID)
		}
		if item.Kind != transcript.ToolCall {
			return fmt.Errorf("sessions: snapshot run %q spawning item %q is not a tool call", run.ID, run.SpawnedByItemID)
		}
		if item.RunID != parent.ID {
			return fmt.Errorf(
				"sessions: snapshot child run %q spawning item %q belongs to run %q, want parent %q",
				run.ID,
				item.ID,
				item.RunID,
				parent.ID,
			)
		}
		parents[run.ID] = parent.ID
	}

	states := make(map[string]uint8, len(runs))
	treeRoots := make(map[string]string, len(runs))
	var visit func(string) (string, error)
	visit = func(runID string) (string, error) {
		switch states[runID] {
		case 1:
			return "", fmt.Errorf("sessions: snapshot run tree contains a cycle at %q", runID)
		case 2:
			return treeRoots[runID], nil
		}
		states[runID] = 1
		rootID := runID
		if parentID := parents[runID]; parentID != "" {
			var err error
			rootID, err = visit(parentID)
			if err != nil {
				return "", err
			}
		}
		states[runID] = 2
		treeRoots[runID] = rootID
		return rootID, nil
	}
	for _, run := range runs {
		rootID, err := visit(run.ID)
		if err != nil {
			return err
		}
		if run.Lineage().IsChild() && rootID != run.RootRunID {
			return fmt.Errorf(
				"sessions: snapshot child run %q reaches root %q through parents, want %q",
				run.ID,
				rootID,
				run.RootRunID,
			)
		}
	}
	return nil
}
