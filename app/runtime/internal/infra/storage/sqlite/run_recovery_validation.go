package sqlite

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// validateParkedRun checks the complete park boundary before boot keeps it
// resumable. A matching row in interrupts is not sufficient: resume also needs
// every referenced running item and a usable process snapshot. An impossible
// partial transcript write means database corruption (the transcript park is one
// transaction), so startup fails loud; a missing/unusable process snapshot is an
// external-resource loss and returns resumable=false so reconciliation can
// terminalize the Run as run_lost.
//
// The Run's own lifecycle facts are not re-checked against a second copy of
// themselves — the Run row is the only place they live, and no write can put
// terminal facts on a non-terminal row.
func (s *RunStore) validateParkedRun(ctx context.Context, active nonTerminalRun, pending interrupts.Pending, validateSnapshot ProcessSnapshotValidator) (bool, error) {
	if err := active.validateParkedInterrupt(pending); err != nil {
		return false, err
	}
	items, err := NewTranscriptStore(s.db).List(ctx, active.sessionID)
	if err != nil {
		return false, fmt.Errorf("sqlite: validate parked run %q transcript: %w", active.runID, err)
	}
	itemsByID := indexTranscriptItems(items)
	interruptItems, err := active.validatePendingInterruptItems(pending.Interrupts, itemsByID)
	if err != nil {
		return false, err
	}
	if err := active.validateRunningInterruptItems(items, interruptItems); err != nil {
		return false, err
	}
	root, ok := pending.RootContinuation()
	if !ok {
		return false, fmt.Errorf("sqlite: validate parked run %q: root continuation is missing", active.runID)
	}
	if err := active.validateDrainedTools(root.DrainedTools, itemsByID, interruptItems); err != nil {
		return false, err
	}
	return s.hasResumableProcessSnapshot(ctx, root.ProcessID, validateSnapshot)
}

func (active nonTerminalRun) validateParkedInterrupt(pending interrupts.Pending) error {
	if pending.RootRunID != active.runID || pending.SessionID != active.sessionID {
		return fmt.Errorf("sqlite: validate parked run %q: interrupt identity is %q/%q, want %q/%q", active.runID, pending.SessionID, pending.RootRunID, active.sessionID, active.runID)
	}
	// These columns decode via time.Unix(0, ns), so the schema default 0 becomes the
	// 1970 epoch — whose time.IsZero() is false (Go's zero time is year 1). Test the
	// decoded nanos against 0 to actually detect an unset timestamp / incomplete boundary.
	root, ok := pending.RootContinuation()
	if !ok || root.RunCreatedAt.UnixNano() == 0 || pending.CreatedAt.UnixNano() == 0 || len(pending.Interrupts) == 0 {
		return fmt.Errorf("sqlite: validate parked run %q: incomplete interrupt boundary", active.runID)
	}
	if pending.TurnID == "" {
		return fmt.Errorf("sqlite: validate parked run %q: turn id is required", active.runID)
	}
	if active.modelSelection != root.ModelSelection {
		return fmt.Errorf("sqlite: validate parked run %q: admission model %q/%q differs from interrupt model %q/%q", active.runID, active.modelSelection.Provider(), active.modelSelection.Model(), root.ModelSelection.Provider(), root.ModelSelection.Model())
	}
	if !active.createdAt.Equal(root.RunCreatedAt) {
		return fmt.Errorf("sqlite: validate parked run %q: run and interrupt creation times differ", active.runID)
	}
	return nil
}

func indexTranscriptItems(items []transcript.Item) map[string]transcript.Item {
	indexed := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		indexed[item.ID] = item
	}
	return indexed
}

func (active nonTerminalRun) validatePendingInterruptItems(interrupts []transcript.Interrupt, itemsByID map[string]transcript.Item) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(interrupts))
	for _, interrupt := range interrupts {
		if interrupt.ItemID == "" {
			return nil, fmt.Errorf("sqlite: validate parked run %q: interrupt item id is required", active.runID)
		}
		if _, duplicate := seen[interrupt.ItemID]; duplicate {
			return nil, fmt.Errorf("sqlite: validate parked run %q: duplicate interrupt item %q", active.runID, interrupt.ItemID)
		}
		seen[interrupt.ItemID] = struct{}{}
		item, found := itemsByID[interrupt.ItemID]
		if !found || item.RunID != active.runID || item.Status != transcript.ItemRunning {
			return nil, fmt.Errorf("sqlite: validate parked run %q: interrupt item %q is not running in the run", active.runID, interrupt.ItemID)
		}
		switch interrupt.Kind {
		case execution.ApprovalInterrupt:
			if interrupt.Approval == nil || interrupt.Question != nil || item.Kind != transcript.ToolCall || item.Tool == nil ||
				!reflect.DeepEqual(*item.Tool, interrupt.Approval.Tool) {
				return nil, fmt.Errorf("sqlite: validate parked run %q: malformed approval item %q", active.runID, interrupt.ItemID)
			}
		case execution.QuestionInterrupt:
			if interrupt.Question == nil || interrupt.Approval != nil || item.Kind != transcript.QuestionItem || item.Question == nil ||
				!reflect.DeepEqual(item.Question, interrupt.Question) {
				return nil, fmt.Errorf("sqlite: validate parked run %q: malformed question item %q", active.runID, interrupt.ItemID)
			}
		default:
			return nil, fmt.Errorf("sqlite: validate parked run %q: unknown interrupt kind %d", active.runID, interrupt.Kind)
		}
	}
	return seen, nil
}

func (active nonTerminalRun) validateRunningInterruptItems(items []transcript.Item, interruptItems map[string]struct{}) error {
	for _, item := range items {
		if item.RunID != active.runID || item.Status != transcript.ItemRunning {
			continue
		}
		if _, belongsToInterrupt := interruptItems[item.ID]; !belongsToInterrupt {
			return fmt.Errorf("sqlite: validate parked run %q: running item %q has no matching interrupt", active.runID, item.ID)
		}
	}
	return nil
}

func (active nonTerminalRun) validateDrainedTools(drainedTools []interrupts.DrainedTool, itemsByID map[string]transcript.Item, interruptItems map[string]struct{}) error {
	drainedSeen := make(map[string]struct{}, len(drainedTools))
	for _, drained := range drainedTools {
		item, found := itemsByID[drained.ItemID]
		_, overlapsInterrupt := interruptItems[drained.ItemID]
		_, duplicate := drainedSeen[drained.ItemID]
		if drained.ItemID == "" || drained.Name == "" || duplicate || overlapsInterrupt || !found || item.RunID != active.runID ||
			item.Kind != transcript.ToolCall || item.Status != transcript.ItemIncomplete || item.Tool == nil || item.Tool.Name != drained.Name {
			return fmt.Errorf("sqlite: validate parked run %q: malformed drained tool %q", active.runID, drained.ItemID)
		}
		drainedSeen[drained.ItemID] = struct{}{}
	}
	return nil
}
