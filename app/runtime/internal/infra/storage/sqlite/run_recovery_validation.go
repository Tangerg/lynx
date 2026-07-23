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
// the interrupted transcript Run, every referenced running item, and a usable
// process snapshot. An impossible partial transcript write means database
// corruption (the transcript park is one transaction), so startup fails loud;
// a missing/unusable process snapshot is an external-resource loss and returns
// resumable=false so reconciliation can terminalize the Run as run_lost.
func (s *RunStateStore) validateParkedRun(ctx context.Context, active nonTerminalRun, pending interrupts.Pending, validateSnapshot ProcessSnapshotValidator) (bool, error) {
	if err := active.validateParkedInterrupt(pending); err != nil {
		return false, err
	}
	items, runs, err := NewTranscriptStore(s.db).List(ctx, active.sessionID)
	if err != nil {
		return false, fmt.Errorf("sqlite: validate parked run %q transcript: %w", active.runID, err)
	}
	run, err := active.interruptedTranscriptRun(runs)
	if err != nil {
		return false, err
	}
	if err := active.validateParkedTranscript(*run, pending); err != nil {
		return false, err
	}
	itemsByID := indexTranscriptItems(items)
	interruptItems, err := active.validatePendingInterruptItems(pending.Interrupts, itemsByID)
	if err != nil {
		return false, err
	}
	if err := active.validateRunningInterruptItems(items, interruptItems); err != nil {
		return false, err
	}
	if err := active.validateDrainedTools(pending.DrainedTools, itemsByID, interruptItems); err != nil {
		return false, err
	}
	if pending.ProcessID == "" {
		return false, nil
	}
	return s.hasResumableProcessSnapshot(ctx, pending.ProcessID, validateSnapshot)
}

func (active nonTerminalRun) validateParkedInterrupt(pending interrupts.Pending) error {
	if pending.RunID != active.runID || pending.SessionID != active.sessionID {
		return fmt.Errorf("sqlite: validate parked run %q: interrupt identity is %q/%q, want %q/%q", active.runID, pending.SessionID, pending.RunID, active.sessionID, active.runID)
	}
	// These columns decode via time.Unix(0, ns), so the schema default 0 becomes the
	// 1970 epoch — whose time.IsZero() is false (Go's zero time is year 1). Test the
	// decoded nanos against 0 to actually detect an unset timestamp / incomplete boundary.
	if pending.RunCreatedAt.UnixNano() == 0 || pending.CreatedAt.UnixNano() == 0 || len(pending.Interrupts) == 0 {
		return fmt.Errorf("sqlite: validate parked run %q: incomplete interrupt boundary", active.runID)
	}
	if pending.TurnID == "" {
		return fmt.Errorf("sqlite: validate parked run %q: turn id is required", active.runID)
	}
	if (pending.Provider == "") != (pending.Model == "") {
		return fmt.Errorf("sqlite: validate parked run %q: provider and model must both be set or both be empty", active.runID)
	}
	if active.provider != pending.Provider || active.model != pending.Model {
		return fmt.Errorf("sqlite: validate parked run %q: admission model %q/%q differs from interrupt model %q/%q", active.runID, active.provider, active.model, pending.Provider, pending.Model)
	}
	return nil
}

func (active nonTerminalRun) interruptedTranscriptRun(runs []transcript.Run) (*transcript.Run, error) {
	for index := range runs {
		if runs[index].ID == active.runID {
			return &runs[index], nil
		}
	}
	return nil, fmt.Errorf("sqlite: validate parked run %q: transcript run not found", active.runID)
}

func (active nonTerminalRun) validateParkedTranscript(run transcript.Run, pending interrupts.Pending) error {
	if run.State != execution.Interrupted || run.Outcome != nil || run.Result != nil || !run.FinishedAt.IsZero() || run.MessageMark != -1 {
		return fmt.Errorf("sqlite: validate parked run %q: invalid interrupted transcript boundary", active.runID)
	}
	if run.Provider != pending.Provider || run.Model != pending.Model {
		return fmt.Errorf("sqlite: validate parked run %q: transcript model %q/%q differs from interrupt model %q/%q", active.runID, run.Provider, run.Model, pending.Provider, pending.Model)
	}
	if !run.CreatedAt.Equal(pending.RunCreatedAt) {
		return fmt.Errorf("sqlite: validate parked run %q: transcript and interrupt creation times differ", active.runID)
	}
	if !reflect.DeepEqual(run.Interrupts, pending.Interrupts) {
		return fmt.Errorf("sqlite: validate parked run %q: transcript and pending interrupts differ", active.runID)
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
		case transcript.ApprovalInterrupt:
			if interrupt.Approval == nil || interrupt.Question != nil || item.Kind != transcript.ToolCall || item.Tool == nil ||
				!reflect.DeepEqual(*item.Tool, interrupt.Approval.Tool) {
				return nil, fmt.Errorf("sqlite: validate parked run %q: malformed approval item %q", active.runID, interrupt.ItemID)
			}
		case transcript.QuestionInterrupt:
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
