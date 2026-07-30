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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
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
			if tree.root.state == execution.Interrupted &&
				hasInterrupt &&
				pendingInterrupt.SessionID == tree.root.sessionID {
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

// nonTerminalRun is a Run still holding its session's admission slot, read
// WITHOUT composing the interrupt it may be parked on: reconciliation exists to
// repair parks whose interrupt record is missing or unusable, so it cannot
// require one to read the row.
type nonTerminalRun struct {
	runID          string
	sessionID      string
	lineage        execution.RunLineage
	modelSelection modelref.Selection
	state          execution.RunState
	createdAt      time.Time
}

type nonTerminalRunTree struct {
	root      nonTerminalRun
	runsByID  map[string]nonTerminalRun
	postorder []string
}

func (s *RunStore) nonTerminalRuns(ctx context.Context) ([]nonTerminalRun, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT run_id, session_id, spawned_by_item_id, parent_run_id, root_run_id,
		        provider, model, state, started_at
		   FROM runs
		  WHERE state != ?
		  ORDER BY started_at, run_id`,
		runStateTerminal)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal runs: %w", err)
	}
	defer rows.Close()

	var out []nonTerminalRun
	for rows.Next() {
		var run nonTerminalRun
		var provider, model, coarse string
		var startedAt int64
		if err := rows.Scan(
			&run.runID,
			&run.sessionID,
			&run.lineage.SpawnedByItemID,
			&run.lineage.ParentRunID,
			&run.lineage.RootRunID,
			&provider,
			&model,
			&coarse,
			&startedAt,
		); err != nil {
			return nil, fmt.Errorf("sqlite: scan non-terminal run: %w", err)
		}
		if err := run.lineage.Validate(run.runID); err != nil {
			return nil, fmt.Errorf(
				"sqlite: decode non-terminal run %q lineage: %w",
				run.runID,
				err,
			)
		}
		run.createdAt = time.Unix(0, startedAt).UTC()
		selection, err := modelref.New(provider, model)
		if err != nil {
			return nil, fmt.Errorf("sqlite: decode non-terminal run %q model selection: %w", run.runID, err)
		}
		run.modelSelection = selection
		switch coarse {
		case runStateRunning:
			run.state = execution.Running
		case runStateInterrupted:
			run.state = execution.Interrupted
		default:
			return nil, fmt.Errorf("sqlite: scan non-terminal run: unknown state %q", coarse)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list non-terminal runs: %w", err)
	}
	return out, nil
}

func groupNonTerminalRunTrees(active []nonTerminalRun) (map[string]nonTerminalRunTree, error) {
	grouped := make(map[string][]nonTerminalRun)
	for _, run := range active {
		rootRunID := run.lineage.TreeRootID(run.runID)
		grouped[rootRunID] = append(grouped[rootRunID], run)
	}

	trees := make(map[string]nonTerminalRunTree, len(grouped))
	for rootRunID, runs := range grouped {
		members := make([]execution.RunTreeMember, 0, len(runs))
		runsByID := make(map[string]nonTerminalRun, len(runs))
		for _, run := range runs {
			members = append(members, execution.RunTreeMember{
				RunID:   run.runID,
				Lineage: run.lineage,
			})
			runsByID[run.runID] = run
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
			if run.sessionID != root.sessionID {
				return nil, fmt.Errorf(
					"sqlite: non-terminal Run %q belongs to Session %q, want tree Session %q",
					run.runID,
					run.sessionID,
					root.sessionID,
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
				tree.root.runID,
				runID,
				err,
			)
		}
	}
	return nil
}

func (s *RunStore) recoverLostRun(ctx context.Context, active nonTerminalRun, now time.Time) error {
	transcripts := NewTranscriptStore(s.db)
	items, err := transcripts.List(ctx, active.sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: reconcile lost run %q transcript: %w", active.runID, err)
	}

	for _, item := range items {
		if item.RunID != active.runID || item.Status != transcript.ItemRunning {
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
			return fmt.Errorf("sqlite: reconcile lost run %q item %q: %w", active.runID, item.ID, err)
		}
	}

	messageMark, err := NewMessageStore(s.db).Count(ctx, active.sessionID)
	if err != nil {
		return fmt.Errorf("sqlite: reconcile lost run %q watermark: %w", active.runID, err)
	}
	next, ok := active.state.RecoverLost()
	if !ok {
		return fmt.Errorf("sqlite: reconcile lost run %q: state %s is not recoverable", active.runID, active.state)
	}
	outcome := execution.OutcomeError
	lost := transcript.Run{
		SessionID:       active.sessionID,
		ID:              active.runID,
		SpawnedByItemID: active.lineage.SpawnedByItemID,
		ParentRunID:     active.lineage.ParentRunID,
		RootRunID:       active.lineage.RootRunID,
		ModelSelection:  active.modelSelection,
		State:           next,
		Outcome:         &outcome,
		Error: &transcript.Problem{
			Kind: transcript.RunLostProblem, Scope: transcript.RunProblem,
			Detail: "run lost on restart",
		},
		CreatedAt:   active.createdAt,
		FinishedAt:  now,
		UpdatedAt:   now,
		MessageMark: messageMark,
	}
	return s.RecoverLost(ctx, lost)
}
