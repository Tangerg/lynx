package sqlite

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// ProcessSnapshotValidator asks the executor that owns processID whether its
// durable continuation is resumable. false, nil means the external state is
// unusable and the owning Run must be recovered lost; a non-nil error means the
// check itself failed and aborts the reconciliation transaction.
type ProcessSnapshotValidator func(context.Context, string) (bool, error)

// ReconcileOrphans repairs non-terminal Run trees abandoned by a process exit
// before any new Run is admitted. A completely interrupted tree with a coherent
// transcript, matching durable Pending, and resumable process snapshot survives.
// Every other non-terminal tree is lost in canonical postorder: its running
// transcript Items become incomplete, each Run becomes a terminal
// error(run_lost), and orphan Pending/process snapshots are removed. The complete
// cross-table repair commits in one transaction, so boot never exposes a
// half-reconciled lifecycle.
func (s *RunStore) ReconcileOrphans(ctx context.Context, validateSnapshot ProcessSnapshotValidator) (int, error) {
	if validateSnapshot == nil {
		return 0, errors.New("sqlite: process snapshot validator is required")
	}
	var reconciled int
	now := time.Now().UTC()
	err := RunInTx(ctx, s.db, func(ctx context.Context) error {
		active, err := s.nonTerminalRuns(ctx)
		if err != nil {
			return err
		}
		interruptStore := NewInterruptStore(s.db)
		pending, err := interruptStore.List(ctx, "")
		if err != nil {
			return fmt.Errorf("sqlite: reconcile orphan runs: %w", err)
		}
		pendingByRun := make(map[string]interrupts.Pending, len(pending))
		processOwners := make(map[string]string, len(pending))
		for _, interrupt := range pending {
			root, ok := interrupt.RootContinuation()
			if !ok {
				return fmt.Errorf("sqlite: interrupt %q has no root continuation", interrupt.RootRunID)
			}
			if owner, duplicate := processOwners[root.ProcessID]; duplicate {
				return fmt.Errorf("sqlite: process snapshot %q is owned by interrupts %q and %q", root.ProcessID, owner, interrupt.RootRunID)
			}
			processOwners[root.ProcessID] = interrupt.RootRunID
			pendingByRun[interrupt.RootRunID] = interrupt
		}

		trees, err := groupNonTerminalRunTrees(active)
		if err != nil {
			return err
		}
		preserved := make(map[string]struct{}, len(trees))
		rootRunIDs := make([]string, 0, len(trees))
		for rootRunID := range trees {
			rootRunIDs = append(rootRunIDs, rootRunID)
		}
		slices.Sort(rootRunIDs)
		for _, rootRunID := range rootRunIDs {
			tree := trees[rootRunID]
			pendingInterrupt, hasInterrupt := pendingByRun[rootRunID]
			if tree.root.State == execution.Interrupted &&
				hasInterrupt &&
				pendingInterrupt.SessionID == tree.root.SessionID {
				resumable, err := s.validateParkedTree(
					ctx,
					tree,
					pendingInterrupt,
					validateSnapshot,
				)
				if err != nil {
					return err
				}
				if resumable {
					preserved[rootRunID] = struct{}{}
					continue
				}
			}
			if err := s.recoverLostTree(ctx, tree, now); err != nil {
				return err
			}
			reconciled += len(tree.postorder)
		}
		for _, interrupt := range pending {
			if _, ok := preserved[interrupt.RootRunID]; ok {
				continue
			}
			root, ok := interrupt.RootContinuation()
			if ok {
				if err := NewProcessStore(s.db).DeleteTrees(ctx, []string{root.ProcessID}); err != nil {
					return fmt.Errorf("sqlite: reconcile orphan process snapshot: %w", err)
				}
			}
			if err := interruptStore.Delete(ctx, interrupt.RootRunID); err != nil {
				return fmt.Errorf("sqlite: reconcile orphan interrupt: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return reconciled, nil
}

func (s *RunStore) hasResumableProcessSnapshot(ctx context.Context, processID string, validateSnapshot ProcessSnapshotValidator) (bool, error) {
	resumable, err := validateSnapshot(ctx, processID)
	if err != nil {
		return false, fmt.Errorf("sqlite: validate process snapshot %q resumable state: %w", processID, err)
	}
	return resumable, nil
}

type nonTerminalRunTree struct {
	root      transcript.Run
	runsByID  map[string]transcript.Run
	postorder []string
}

// nonTerminalRuns reads complete durable Run aggregates, including accounting,
// limits, model selection, and the root-owned protocol profile. Recovery used to
// maintain a smaller parallel row decoder here; that made newly added Run facts
// disappear when the row was terminalized as lost. The recovery-specific scan
// now differs from ordinary reads only by allowing a missing pending join.
func (s *RunStore) nonTerminalRuns(ctx context.Context) ([]transcript.Run, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT `+runColumns+`
		   FROM runs AS r
		   `+runReadJoins+`
		  WHERE r.state != ?
		  ORDER BY r.started_at, r.run_id`,
		runStateTerminal)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal runs: %w", err)
	}
	defer rows.Close()

	var out []transcript.Run
	for rows.Next() {
		run, err := scanRunForRecovery(rows)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan non-terminal run: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal runs: %w", err)
	}
	return out, nil
}

func groupNonTerminalRunTrees(active []transcript.Run) (map[string]nonTerminalRunTree, error) {
	grouped := make(map[string][]transcript.Run)
	for _, run := range active {
		rootRunID := run.Lineage().TreeRootID(run.ID)
		grouped[rootRunID] = append(grouped[rootRunID], run)
	}

	trees := make(map[string]nonTerminalRunTree, len(grouped))
	for rootRunID, runs := range grouped {
		members := make([]execution.RunTreeMember, 0, len(runs))
		runsByID := make(map[string]transcript.Run, len(runs))
		for _, run := range runs {
			members = append(members, execution.RunTreeMember{
				RunID:   run.ID,
				Lineage: run.Lineage(),
			})
			runsByID[run.ID] = run
		}
		topology, err := execution.NewRunTree(rootRunID, members)
		if err != nil {
			return nil, fmt.Errorf(
				"sqlite: assemble non-terminal Run tree %q: %w",
				rootRunID,
				err,
			)
		}
		root, found := runsByID[rootRunID]
		if !found {
			return nil, fmt.Errorf(
				"sqlite: assemble non-terminal Run tree %q: root is missing",
				rootRunID,
			)
		}
		for _, run := range runs {
			if run.SessionID != root.SessionID {
				return nil, fmt.Errorf(
					"sqlite: non-terminal Run %q belongs to Session %q, want tree Session %q",
					run.ID,
					run.SessionID,
					root.SessionID,
				)
			}
		}
		trees[rootRunID] = nonTerminalRunTree{
			root:      root,
			runsByID:  runsByID,
			postorder: topology.Postorder(),
		}
	}
	return trees, nil
}

func (s *RunStore) recoverLostTree(
	ctx context.Context,
	tree nonTerminalRunTree,
	now time.Time,
) error {
	for _, runID := range tree.postorder {
		if err := s.recoverLostRun(ctx, tree.runsByID[runID], now); err != nil {
			return fmt.Errorf(
				"sqlite: recover lost Run tree %q member %q: %w",
				tree.root.ID,
				runID,
				err,
			)
		}
	}
	return nil
}

func (s *RunStore) recoverLostRun(ctx context.Context, active transcript.Run, now time.Time) error {
	transcripts := NewTranscriptStore(s.db)
	items, err := transcripts.List(ctx, active.SessionID)
	if err != nil {
		return fmt.Errorf("sqlite: reconcile lost run %q transcript: %w", active.ID, err)
	}

	for _, item := range items {
		if item.RunID != active.ID || item.Status != transcript.ItemRunning {
			continue
		}
		item.Status = transcript.ItemIncomplete
		if item.Kind == transcript.ToolCall {
			item.Error = &transcript.Problem{
				Kind: transcript.ToolFailedProblem, Scope: transcript.ToolProblem,
				Detail: "tool call interrupted because the run was lost on restart",
			}
		}
		if err := transcripts.AppendItem(ctx, item); err != nil {
			return fmt.Errorf("sqlite: reconcile lost run %q item %q: %w", active.ID, item.ID, err)
		}
	}

	messageMark, err := NewMessageStore(s.db).Count(ctx, active.SessionID)
	if err != nil {
		return fmt.Errorf("sqlite: reconcile lost run %q watermark: %w", active.ID, err)
	}
	next, ok := active.State.RecoverLost()
	if !ok {
		return fmt.Errorf("sqlite: reconcile lost run %q: state %s is not recoverable", active.ID, active.State)
	}
	outcome := execution.OutcomeError
	active.State = next
	active.ActiveSegmentID = ""
	active.Outcome = &outcome
	active.Detail = ""
	active.Error = &transcript.Problem{
		Kind: transcript.RunLostProblem, Scope: transcript.RunProblem,
		Detail: "run lost on restart",
	}
	active.Interrupts = nil
	active.FinishedAt = now
	active.UpdatedAt = now
	active.MessageMark = messageMark
	return s.RecoverLost(ctx, active)
}
