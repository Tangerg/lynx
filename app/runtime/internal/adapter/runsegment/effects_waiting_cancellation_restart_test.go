package runsegment

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestWaitingSubtreeCancellationSurvivesSQLiteRestart(t *testing.T) {
	tests := []struct {
		name              string
		survivingBoundary bool
		wantPostorder     []string
	}{
		{
			name:              "remaining waiting boundary",
			survivingBoundary: true,
			wantPostorder: []string{
				"run_grandchild",
				"run_child",
				"run_sibling",
				"run_root",
			},
		},
		{
			name:          "final waiting boundary",
			wantPostorder: []string{"run_grandchild", "run_child", "run_root"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "runtime.db")
			fixture := newWaitingCancellationSQLiteFixtureAt(
				t,
				path,
				test.survivingBoundary,
			)
			result, err := fixture.effects.CommitWaitingSubtreeCancellation(
				fixture.ctx,
				fixture.commit,
			)
			if err != nil {
				t.Fatalf("commit waiting cancellation: %v", err)
			}
			if err := fixture.db.Close(); err != nil {
				t.Fatalf("close first runtime database: %v", err)
			}

			db, err := sqlite.Open(path)
			if err != nil {
				t.Fatalf("reopen runtime database: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			runStore := sqlite.NewRunStore(db)
			interruptStore := sqlite.NewInterruptStore(db)
			transcriptStore := sqlite.NewTranscriptStore(db)
			processStore := sqlite.NewProcessStore(db)
			query := queries.New(queries.Dependencies{Runs: runStore})

			target := queryRun(t, query, fixture.childRun.ID)
			if !sameRunSnapshot(target, result.TargetRun) {
				t.Fatalf(
					"restarted target Run differs from command result:\ngot  %+v\nwant %+v",
					target,
					result.TargetRun,
				)
			}
			root := queryRun(t, query, fixture.rootRun.ID)
			if !sameRunSnapshot(root, result.RootRun) {
				t.Fatalf(
					"restarted root Run differs from command result:\ngot  %+v\nwant %+v",
					root,
					result.RootRun,
				)
			}

			treeRuns, err := runStore.RunTree(fixture.ctx, fixture.childRun.ID)
			if err != nil {
				t.Fatalf("read tree through canceled child ID: %v", err)
			}
			members := make([]execution.RunTreeMember, 0, len(treeRuns))
			runsByID := make(map[string]transcript.Run, len(treeRuns))
			for _, run := range treeRuns {
				members = append(members, execution.RunTreeMember{
					RunID:   run.ID,
					Lineage: run.Lineage(),
				})
				runsByID[run.ID] = run
			}
			topology, err := execution.NewRunTree(fixture.rootRun.ID, members)
			if err != nil {
				t.Fatalf("assemble restarted Run tree: %v", err)
			}
			if got := topology.Postorder(); !slices.Equal(got, test.wantPostorder) {
				t.Fatalf("restarted Run tree postorder = %v, want %v", got, test.wantPostorder)
			}
			for _, runID := range []string{
				fixture.grandchildRun.ID,
				fixture.childRun.ID,
			} {
				run := runsByID[runID]
				if run.State != execution.Canceled ||
					run.Outcome == nil ||
					*run.Outcome != execution.OutcomeCanceled {
					t.Fatalf("canceled subtree Run %q = %+v", runID, run)
				}
			}

			storedTree, checkpoint, err := processStore.LoadTree(
				fixture.ctx,
				fixture.replacementTree.RootID,
			)
			if err != nil {
				t.Fatalf("load restarted process tree: %v", err)
			}
			processTree := restoredProcessTree(t, storedTree)
			assertReplacementCheckpoint(
				t,
				processTree,
				checkpoint,
				fixture,
			)
			for _, processID := range []string{"process_child", "process_grandchild"} {
				if _, found := processSnapshotByID(processTree, processID); found {
					t.Fatalf("restarted process tree resurrected canceled process %q", processID)
				}
			}

			for _, replacement := range fixture.commit.TerminalItems {
				item, found, err := transcriptStore.Item(
					fixture.ctx,
					replacement.Expected.ID,
				)
				if err != nil ||
					!found ||
					!sameItemSnapshot(item, replacement.Replacement) {
					t.Fatalf(
						"restarted terminal Item %q = found:%t value:%+v err:%v, want %+v",
						replacement.Expected.ID,
						found,
						item,
						err,
						replacement.Replacement,
					)
				}
			}

			if test.survivingBoundary {
				assertRestartedWaitingBoundary(
					t,
					fixture,
					runStore,
					interruptStore,
					transcriptStore,
					processStore,
				)
				return
			}
			assertRestartedRunningBoundary(
				t,
				fixture,
				runStore,
				interruptStore,
			)
		})
	}
}

func sameRunSnapshot(left, right transcript.Run) bool {
	return reflect.DeepEqual(
		normalizeRunSnapshot(left),
		normalizeRunSnapshot(right),
	)
}

func normalizeRunSnapshot(run transcript.Run) transcript.Run {
	run.CreatedAt = timeFromUnixNano(run.CreatedAt)
	run.FinishedAt = timeFromUnixNano(run.FinishedAt)
	run.UpdatedAt = timeFromUnixNano(run.UpdatedAt)
	if len(run.Interrupts) == 0 {
		run.Interrupts = nil
	}
	run.ProtocolProfile = normalizeProtocolProfile(run.ProtocolProfile)
	if run.Metrics.Usage != nil && len(run.Metrics.Usage.ByModel) == 0 {
		usage := *run.Metrics.Usage
		usage.ByModel = nil
		run.Metrics.Usage = &usage
	}
	return run
}

func queryRun(
	t *testing.T,
	query *queries.Coordinator,
	runID string,
) transcript.Run {
	t.Helper()
	run, found, err := query.Run(t.Context(), runID)
	if err != nil || !found {
		t.Fatalf("query Run %q = found:%t value:%+v err:%v", runID, found, run, err)
	}
	return run
}

func assertReplacementCheckpoint(
	t *testing.T,
	tree core.ProcessSnapshotTree,
	checkpoint execution.ProcessCheckpoint,
	fixture waitingCancellationSQLiteFixture,
) {
	t.Helper()
	if !reflect.DeepEqual(
		normalizedProcessTree(tree),
		normalizedProcessTree(fixture.replacementTree),
	) {
		t.Fatalf(
			"restarted process tree differs from committed replacement:\ngot  %+v\nwant %+v",
			tree,
			fixture.replacementTree,
		)
	}
	if !reflect.DeepEqual(
		normalizedProcessCheckpoint(checkpoint),
		normalizedProcessCheckpoint(fixture.replacementCheckpoint),
	) {
		t.Fatalf(
			"restarted checkpoint differs from committed replacement:\ngot  %+v\nwant %+v",
			checkpoint,
			fixture.replacementCheckpoint,
		)
	}
}

func assertRestartedWaitingBoundary(
	t *testing.T,
	fixture waitingCancellationSQLiteFixture,
	runStore *sqlite.RunStore,
	interruptStore *sqlite.InterruptStore,
	transcriptStore *sqlite.TranscriptStore,
	processStore *sqlite.ProcessStore,
) {
	t.Helper()
	pending, found, err := interruptStore.Get(fixture.ctx, fixture.rootRun.ID)
	if err != nil ||
		!found ||
		fixture.commit.RemainingPending == nil ||
		!samePendingSnapshot(pending, *fixture.commit.RemainingPending) {
		t.Fatalf(
			"restarted reduced Pending = found:%t value:%+v err:%v, want %+v",
			found,
			pending,
			err,
			fixture.commit.RemainingPending,
		)
	}
	siblingQuestion := pending.Interrupts[0]
	item, found, err := transcriptStore.Item(fixture.ctx, siblingQuestion.ItemID)
	if err != nil || !found || item.Status != transcript.ItemRunning {
		t.Fatalf(
			"surviving interrupt Item after restart = found:%t value:%+v err:%v, want Running",
			found,
			item,
			err,
		)
	}

	recovered, err := runStore.ReconcileOrphans(
		fixture.ctx,
		func(ctx context.Context, processID string) (bool, error) {
			if processID != fixture.replacementTree.RootID {
				return false, fmt.Errorf(
					"snapshot validator received process %q, want %q",
					processID,
					fixture.replacementTree.RootID,
				)
			}
			storedTree, checkpoint, err := processStore.LoadTree(ctx, processID)
			if err != nil {
				return false, err
			}
			tree := restoredProcessTree(t, storedTree)
			if !reflect.DeepEqual(
				normalizedProcessTree(tree),
				normalizedProcessTree(fixture.replacementTree),
			) ||
				!reflect.DeepEqual(
					normalizedProcessCheckpoint(checkpoint),
					normalizedProcessCheckpoint(fixture.replacementCheckpoint),
				) {
				return false, errors.New("replacement checkpoint changed after restart")
			}
			return true, nil
		},
	)
	if err != nil || recovered != 0 {
		t.Fatalf("boot reconciliation = (%d, %v), want preserved tree", recovered, err)
	}
	for _, runID := range []string{"run_sibling", fixture.rootRun.ID} {
		run, found, err := runStore.Run(fixture.ctx, runID)
		if err != nil || !found || run.State != execution.Interrupted {
			t.Fatalf(
				"surviving Run %q after boot = found:%t value:%+v err:%v, want Interrupted",
				runID,
				found,
				run,
				err,
			)
		}
	}
}

func assertRestartedRunningBoundary(
	t *testing.T,
	fixture waitingCancellationSQLiteFixture,
	runStore *sqlite.RunStore,
	interruptStore *sqlite.InterruptStore,
) {
	t.Helper()
	if pending, found, err := interruptStore.Get(fixture.ctx, fixture.rootRun.ID); err != nil || found {
		t.Fatalf(
			"Pending after final-boundary restart = found:%t value:%+v err:%v, want none",
			found,
			pending,
			err,
		)
	}
	recovered, err := runStore.ReconcileOrphans(
		fixture.ctx,
		func(context.Context, string) (bool, error) {
			t.Fatal("boot inspected a snapshot for a Running tree")
			return false, nil
		},
	)
	if err != nil || recovered != 1 {
		t.Fatalf("boot reconciliation = (%d, %v), want one lost Running root", recovered, err)
	}
	root, found, err := runStore.Run(fixture.ctx, fixture.rootRun.ID)
	if err != nil ||
		!found ||
		root.State != execution.Failed ||
		root.Outcome == nil ||
		*root.Outcome != execution.OutcomeError ||
		root.Error == nil ||
		root.Error.Kind != transcript.RunLostProblem {
		t.Fatalf(
			"root after Running restart = found:%t value:%+v err:%v, want failed/run_lost",
			found,
			root,
			err,
		)
	}
	for _, runID := range []string{
		fixture.grandchildRun.ID,
		fixture.childRun.ID,
	} {
		run, found, err := runStore.Run(fixture.ctx, runID)
		if err != nil || !found || run.State != execution.Canceled {
			t.Fatalf(
				"canceled Run %q after root recovery = found:%t value:%+v err:%v",
				runID,
				found,
				run,
				err,
			)
		}
	}
}
