package runsegment

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/persistence"
	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
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

			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatalf("reopen runtime database: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			runStore := sqlite.NewRunStore(db)
			interruptStore := persistence.NewInterruptStore(sqlite.NewInterruptStore(db))
			transcriptStore := sqlite.NewTranscriptStore(db)
			checkpointStore := persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
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

			treeRuns, err := runStore.Tree(fixture.ctx, fixture.childRun.ID)
			if err != nil {
				t.Fatalf("read tree through canceled child ID: %v", err)
			}
			members := make([]run.TreeMember, 0, len(treeRuns))
			runsByID := make(map[string]transcript.Run, len(treeRuns))
			for _, record := range treeRuns {
				members = append(members, run.TreeMember{
					RunID:   record.ID,
					Lineage: record.Lineage(),
				})
				runsByID[record.ID] = record
			}
			topology, err := run.NewTree(fixture.rootRun.ID, members)
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
				record := runsByID[runID]
				if record.State != run.Canceled ||
					record.Outcome == nil ||
					*record.Outcome != run.OutcomeCanceled {
					t.Fatalf("canceled subtree Run %q = %+v", runID, record)
				}
			}

			checkpoint, err := checkpointStore.LoadCheckpoint(
				fixture.ctx,
				fixture.replacementTree.RootID,
			)
			if err != nil {
				t.Fatalf("load restarted process tree: %v", err)
			}
			processTree := restoredExecutorTree(t, checkpoint)
			assertReplacementCheckpoint(
				t,
				processTree,
				checkpoint,
				fixture,
			)
			for _, processID := range []string{"process_child", "process_grandchild"} {
				if _, found := executorMemberByID(processTree, processID); found {
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
					checkpointStore,
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
	run.Capabilities = normalizeCapabilities(run.Capabilities)
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
	tree executorTreeFixture,
	checkpoint runs.ExecutorCheckpoint,
	fixture waitingCancellationSQLiteFixture,
) {
	t.Helper()
	if !reflect.DeepEqual(
		normalizedExecutorTree(tree),
		normalizedExecutorTree(fixture.replacementTree),
	) {
		t.Fatalf(
			"restarted process tree differs from committed replacement:\ngot  %+v\nwant %+v",
			tree,
			fixture.replacementTree,
		)
	}
	if !reflect.DeepEqual(
		normalizedExecutorCheckpoint(checkpoint),
		normalizedExecutorCheckpoint(fixture.replacementCheckpoint),
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
	interruptStore *persistence.InterruptStore,
	transcriptStore *sqlite.TranscriptStore,
	checkpointStore *persistence.ExecutorCheckpointStore,
) {
	t.Helper()
	pending, found, err := interruptStore.Get(fixture.ctx, fixture.rootRun.ID)
	if err != nil ||
		!found ||
		fixture.commit.RemainingPending == nil ||
		!pending.Equal(*fixture.commit.RemainingPending) {
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

	checkpoint, err := checkpointStore.LoadCheckpoint(fixture.ctx, fixture.replacementTree.RootID)
	if err != nil || !reflect.DeepEqual(
		normalizedExecutorCheckpoint(checkpoint),
		normalizedExecutorCheckpoint(fixture.replacementCheckpoint),
	) {
		t.Fatalf("restarted executor checkpoint = (%+v, %v), want committed replacement", checkpoint, err)
	}
	for _, runID := range []string{"run_sibling", fixture.rootRun.ID} {
		record, found, err := runStore.Run(fixture.ctx, runID)
		if err != nil || !found || record.State != run.Waiting {
			t.Fatalf(
				"surviving Run %q after boot = found:%t value:%+v err:%v, want Waiting",
				runID,
				found,
				record,
				err,
			)
		}
	}
}

func assertRestartedRunningBoundary(
	t *testing.T,
	fixture waitingCancellationSQLiteFixture,
	runStore *sqlite.RunStore,
	interruptStore *persistence.InterruptStore,
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
	root, found, err := runStore.Run(fixture.ctx, fixture.rootRun.ID)
	if err != nil ||
		!found ||
		root.State != run.Running ||
		root.Outcome != nil ||
		root.Error != nil {
		t.Fatalf(
			"root after restart = found:%t value:%+v err:%v, want persisted Running",
			found,
			root,
			err,
		)
	}
	for _, runID := range []string{
		fixture.grandchildRun.ID,
		fixture.childRun.ID,
	} {
		record, found, err := runStore.Run(fixture.ctx, runID)
		if err != nil || !found || record.State != run.Canceled {
			t.Fatalf(
				"canceled Run %q after root recovery = found:%t value:%+v err:%v",
				runID,
				found,
				record,
				err,
			)
		}
	}
}
