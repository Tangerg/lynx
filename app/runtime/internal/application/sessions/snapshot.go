package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// ExportResult is the complete result of a session archive use case. The
// archive and its session view are derived while the same admission is held, so
// callers cannot pair one revision's archive with a later revision's view.
type ExportResult struct {
	Session  View
	Snapshot PortableSnapshot
	Items    []transcript.Item
}

// ExportSession reserves the session's single-writer slot and derives its
// portable archive and presentation from one coherent canonical state. Active
// and parked runs are rejected because their executor state is process-local
// and therefore cannot be represented by a portable session artifact.
func (c *Coordinator) ExportSession(ctx context.Context, sessionID string) (ExportResult, error) {
	admission, err := c.ClaimIdleSession(ctx, sessionID)
	if err != nil {
		return ExportResult{}, err
	}
	defer admission.Release()
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
	view, err := c.view(snapshot.Session, ActivityIdle)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Session: view, Snapshot: portable, Items: snapshot.Items}, nil
}

// Validate checks a snapshot's referential integrity — the session id is present
// and every run/item belongs to it — before the coordinator hands it out.
func (snapshot Snapshot) Validate() error {
	if snapshot.Session.ID() == "" {
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
		if run.ID() == "" || run.SessionID() != snapshot.Session.ID() {
			return nil, fmt.Errorf("sessions: snapshot run %q belongs to session %q, want %q", run.ID(), run.SessionID(), snapshot.Session.ID())
		}
		if _, exists := runs[run.ID()]; exists {
			return nil, fmt.Errorf("sessions: snapshot contains duplicate run %q", run.ID())
		}
		// A snapshot is a portable record of finished work, so only a terminal Run
		// belongs in one; whether its facts hold together is the Run's own rule.
		if !run.State().IsTerminal() {
			return nil, fmt.Errorf("sessions: snapshot run %q is %s, want terminal", run.ID(), run.State())
		}
		if err := run.Validate(); err != nil {
			return nil, fmt.Errorf("sessions: snapshot run %q: %w", run.ID(), err)
		}
		if run.MessageMark() > len(snapshot.Messages) {
			return nil, fmt.Errorf("sessions: snapshot run %q has invalid message watermark %d", run.ID(), run.MessageMark())
		}
		runs[run.ID()] = struct{}{}
	}
	return runs, nil
}

func (snapshot Snapshot) validateItems(runs map[string]struct{}) (map[string]transcript.Item, error) {
	items := make(map[string]transcript.Item, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if item.ID() == "" || item.SessionID() != snapshot.Session.ID() {
			return nil, fmt.Errorf("sessions: snapshot item %q belongs to session %q, want %q", item.ID(), item.SessionID(), snapshot.Session.ID())
		}
		if _, exists := items[item.ID()]; exists {
			return nil, fmt.Errorf("sessions: snapshot contains duplicate item %q", item.ID())
		}
		items[item.ID()] = item
		if _, found := runs[item.RunID()]; !found {
			return nil, fmt.Errorf("sessions: snapshot item %q references unknown run %q", item.ID(), item.RunID())
		}
		switch item.Status() {
		case transcript.ItemCompleted, transcript.ItemIncomplete:
		case transcript.ItemRunning:
			return nil, fmt.Errorf("sessions: snapshot terminal run item %q is still running", item.ID())
		default:
			return nil, fmt.Errorf("sessions: snapshot item %q has unknown status %q", item.ID(), item.Status())
		}
		if _, failed := item.Failure(); failed && (item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemIncomplete) {
			return nil, fmt.Errorf("sessions: snapshot item %q has an invalid tool failure", item.ID())
		}
		if err := item.Validate(); err != nil {
			return nil, fmt.Errorf("sessions: snapshot item %q: %w", item.ID(), err)
		}
	}
	return items, nil
}

func validateSnapshotRunTree(runs []run.Run, items map[string]transcript.Item) error {
	tree, err := newSnapshotRunTree(runs, items)
	if err != nil {
		return err
	}
	return tree.validateRootLineage(runs)
}

type snapshotRunTree struct {
	runByID             map[string]run.Run
	parentByRunID       map[string]string
	visitStateByRunID   map[string]snapshotRunVisitState
	resolvedRootByRunID map[string]string
}

type snapshotRunVisitState uint8

const (
	snapshotRunUnvisited snapshotRunVisitState = iota
	snapshotRunVisiting
	snapshotRunVisited
)

func newSnapshotRunTree(
	runs []run.Run,
	items map[string]transcript.Item,
) (*snapshotRunTree, error) {
	tree := &snapshotRunTree{
		runByID:             make(map[string]run.Run, len(runs)),
		parentByRunID:       make(map[string]string, len(runs)),
		visitStateByRunID:   make(map[string]snapshotRunVisitState, len(runs)),
		resolvedRootByRunID: make(map[string]string, len(runs)),
	}
	for _, run := range runs {
		tree.runByID[run.ID()] = run
	}
	for _, run := range runs {
		if run.Lineage().IsRoot() {
			continue
		}
		if err := tree.indexChild(run, items); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

func (tree *snapshotRunTree) indexChild(
	child run.Run,
	items map[string]transcript.Item,
) error {
	lineage := child.Lineage()
	parent, parentFound := tree.runByID[lineage.ParentRunID]
	if !parentFound {
		return fmt.Errorf("sessions: snapshot child run %q references unknown parent %q", child.ID(), lineage.ParentRunID)
	}
	root, rootFound := tree.runByID[lineage.RootRunID]
	if !rootFound {
		return fmt.Errorf("sessions: snapshot child run %q references unknown root %q", child.ID(), lineage.RootRunID)
	}
	if !root.Lineage().IsRoot() {
		return fmt.Errorf("sessions: snapshot child run %q names child %q as its root", child.ID(), lineage.RootRunID)
	}
	if !root.Capabilities().ChildRuns {
		return fmt.Errorf(
			"sessions: snapshot child run %q belongs to root %q whose run capabilities disallow child runs",
			child.ID(),
			root.ID(),
		)
	}
	item, found := items[lineage.SpawnedByItemID]
	if !found {
		return fmt.Errorf("sessions: snapshot run %q references unknown spawning item %q", child.ID(), lineage.SpawnedByItemID)
	}
	if item.Kind() != transcript.ToolCall {
		return fmt.Errorf("sessions: snapshot run %q spawning item %q is not a tool call", child.ID(), lineage.SpawnedByItemID)
	}
	if item.RunID() != parent.ID() {
		return fmt.Errorf(
			"sessions: snapshot child run %q spawning item %q belongs to run %q, want parent %q",
			child.ID(),
			item.ID(),
			item.RunID(),
			parent.ID(),
		)
	}
	tree.parentByRunID[child.ID()] = parent.ID()
	return nil
}

func (tree *snapshotRunTree) validateRootLineage(runs []run.Run) error {
	for _, run := range runs {
		rootRunID, err := tree.resolveRootRunID(run.ID())
		if err != nil {
			return err
		}
		if run.Lineage().IsChild() && rootRunID != run.Lineage().RootRunID {
			return fmt.Errorf(
				"sessions: snapshot child run %q reaches root %q through parents, want %q",
				run.ID(),
				rootRunID,
				run.Lineage().RootRunID,
			)
		}
	}
	return nil
}

func (tree *snapshotRunTree) resolveRootRunID(runID string) (string, error) {
	switch tree.visitStateByRunID[runID] {
	case snapshotRunVisiting:
		return "", fmt.Errorf("sessions: snapshot run tree contains a cycle at %q", runID)
	case snapshotRunVisited:
		return tree.resolvedRootByRunID[runID], nil
	case snapshotRunUnvisited:
	}
	tree.visitStateByRunID[runID] = snapshotRunVisiting
	rootRunID := runID
	if parentRunID := tree.parentByRunID[runID]; parentRunID != "" {
		var err error
		rootRunID, err = tree.resolveRootRunID(parentRunID)
		if err != nil {
			return "", err
		}
	}
	tree.visitStateByRunID[runID] = snapshotRunVisited
	tree.resolvedRootByRunID[runID] = rootRunID
	return rootRunID, nil
}
