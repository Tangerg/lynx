package runsegment

import (
	"cmp"
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

type failingWaitingCheckpoint struct {
	err error
}

func (checkpoint failingWaitingCheckpoint) PersistCheckpoint(context.Context) error {
	return checkpoint.err
}

type failingWaitingItemReplacer struct {
	ItemReplacer
	failItemID string
	err        error
}

func (replacer failingWaitingItemReplacer) ReplaceItem(
	ctx context.Context,
	expected transcript.Item,
	replacement transcript.Item,
) error {
	if replacer.failItemID == "" || replacer.failItemID == expected.ID {
		return replacer.err
	}
	return replacer.ItemReplacer.ReplaceItem(ctx, expected, replacement)
}

type failingWaitingRunWriter struct {
	RunWriter
	resumeErr      error
	terminalizeErr error
}

func (writer failingWaitingRunWriter) Resume(
	ctx context.Context,
	sessionID string,
	draft execution.RunResumeDraft,
	resumedAt time.Time,
) error {
	if writer.resumeErr != nil {
		return writer.resumeErr
	}
	return writer.RunWriter.Resume(ctx, sessionID, draft, resumedAt)
}

func (writer failingWaitingRunWriter) Terminalize(
	ctx context.Context,
	run transcript.Run,
) error {
	if writer.terminalizeErr != nil {
		return writer.terminalizeErr
	}
	return writer.RunWriter.Terminalize(ctx, run)
}

type failingWaitingInterruptStore struct {
	InterruptStore
	putErr error
}

func (store failingWaitingInterruptStore) Put(
	ctx context.Context,
	pending interrupts.Pending,
) error {
	if store.putErr != nil {
		return store.putErr
	}
	return store.InterruptStore.Put(ctx, pending)
}

type failingWaitingTranscriptStore struct {
	TranscriptStore
	appendErr error
}

func (store failingWaitingTranscriptStore) AppendItem(
	ctx context.Context,
	item transcript.Item,
) error {
	if store.appendErr != nil {
		return store.appendErr
	}
	return store.TranscriptStore.AppendItem(ctx, item)
}

func TestCommitWaitingSubtreeCancellationRejectsStalePendingWithoutMutation(t *testing.T) {
	fixture := newWaitingCancellationSQLiteFixture(t)
	changedPending := fixture.commit.ExpectedPending
	changedPending.TurnID = "turn_replaced"
	if err := fixture.interrupts.Put(fixture.ctx, changedPending); err != nil {
		t.Fatalf("replace Pending fixture: %v", err)
	}

	_, err := fixture.effects.CommitWaitingSubtreeCancellation(
		fixture.ctx,
		fixture.commit,
	)
	if !errors.Is(err, runs.ErrSessionBusy) {
		t.Fatalf("stale Pending error = %v, want ErrSessionBusy", err)
	}
	if !strings.Contains(err.Error(), fixture.rootRun.ID) {
		t.Fatalf("stale Pending error = %q, want root Run identity", err)
	}
	assertWaitingCancellationUnchanged(t, fixture, changedPending)
}

func TestCommitWaitingSubtreeCancellationRollsBackEveryPreCommitFailure(t *testing.T) {
	tests := []struct {
		name              string
		survivingBoundary bool
		operation         string
		configure         func(*waitingCancellationSQLiteFixture, error)
	}{
		{
			name:      "replacement checkpoint",
			operation: "persist checkpoint",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.commit.Checkpoint = failingWaitingCheckpoint{err: injected}
			},
		},
		{
			name:      "parent Item",
			operation: "replace spawning Item",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.ItemReplacer = failingWaitingItemReplacer{
						ItemReplacer: fixture.transcript,
						err:          injected,
					}
				})
			},
		},
		{
			name:      "terminal interrupt Item",
			operation: "settle interrupted Item",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.ItemReplacer = failingWaitingItemReplacer{
						ItemReplacer: fixture.transcript,
						failItemID:   fixture.commit.TerminalItems[0].Expected.ID,
						err:          injected,
					}
				})
			},
		},
		{
			name:      "terminal Run",
			operation: "terminalize canceled Run",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.RunState = failingWaitingRunWriter{
						RunWriter:      fixture.runState,
						terminalizeErr: injected,
					}
				})
			},
		},
		{
			name:              "reduced Pending",
			survivingBoundary: true,
			operation:         "persist reduced interrupt",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.Interrupts = failingWaitingInterruptStore{
						InterruptStore: fixture.interrupts,
						putErr:         injected,
					}
				})
			},
		},
		{
			name:      "tree resume",
			operation: "resume surviving Run",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.RunState = failingWaitingRunWriter{
						RunWriter: fixture.runState,
						resumeErr: injected,
					}
				})
			},
		},
		{
			name:      "opening Item",
			operation: "persist opening projection",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.commit.OpeningEvents = []runs.EventCommit{{
					RunID:     fixture.rootRun.ID,
					SessionID: fixture.rootRun.SessionID,
					Items: []transcript.Item{{
						ID:        "item_root_continuation",
						SessionID: fixture.rootRun.SessionID,
						RunID:     fixture.rootRun.ID,
						Status:    transcript.ItemCompleted,
						Kind:      transcript.UserMessage,
						CreatedAt: fixture.rootRun.UpdatedAt,
						Content: []transcript.ContentBlock{{
							Kind: transcript.TextContent,
							Text: "continue",
						}},
					}},
				}}
				fixture.replaceEffects(func(config *Config) {
					config.Transcript = failingWaitingTranscriptStore{
						TranscriptStore: fixture.transcript,
						appendErr:       injected,
					}
				})
			},
		},
		{
			name:      "transaction completion",
			operation: "commit waiting child Run",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.Tx = func(ctx context.Context, fn func(context.Context) error) error {
						return sqlite.RunInTx(ctx, fixture.db, func(txCtx context.Context) error {
							if err := fn(txCtx); err != nil {
								return err
							}
							return injected
						})
					}
				})
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWaitingCancellationSQLiteFixtureWithSurvivingBoundary(
				t,
				test.survivingBoundary,
			)
			injected := errors.New("injected " + test.name + " failure")
			test.configure(&fixture, injected)

			_, err := fixture.effects.CommitWaitingSubtreeCancellation(
				fixture.ctx,
				fixture.commit,
			)
			if !errors.Is(err, injected) {
				t.Fatalf("commit error = %v, want injected cause", err)
			}
			for _, expected := range []string{
				test.operation,
				fixture.childRun.ID,
				fixture.rootRun.ID,
			} {
				if !strings.Contains(err.Error(), expected) {
					t.Fatalf("commit error = %q, want %q", err, expected)
				}
			}
			assertWaitingCancellationUnchanged(
				t,
				fixture,
				fixture.commit.ExpectedPending,
			)
		})
	}
}

func (fixture *waitingCancellationSQLiteFixture) replaceEffects(
	configure func(*Config),
) {
	config := Config{
		Interrupts:   fixture.interrupts,
		Transcript:   fixture.transcript,
		ItemReplacer: fixture.transcript,
		RunState:     fixture.runState,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, fixture.db, fn)
		},
	}
	configure(&config)
	fixture.effects = New(config)
}

func assertWaitingCancellationUnchanged(
	t *testing.T,
	fixture waitingCancellationSQLiteFixture,
	expectedPending interrupts.Pending,
) {
	t.Helper()
	pending, found, err := fixture.interrupts.Get(fixture.ctx, fixture.rootRun.ID)
	if err != nil || !found || !samePendingSnapshot(pending, expectedPending) {
		t.Fatalf(
			"Pending after rollback = found:%t value:%+v err:%v, want %+v",
			found,
			pending,
			err,
			expectedPending,
		)
	}

	items, err := fixture.transcript.List(fixture.ctx, fixture.rootRun.SessionID)
	if err != nil {
		t.Fatalf("list transcript after rollback: %v", err)
	}
	if len(items) != len(fixture.originalItems) {
		t.Fatalf(
			"transcript after rollback has %d Items, want %d: %+v",
			len(items),
			len(fixture.originalItems),
			items,
		)
	}
	itemsByID := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		itemsByID[item.ID] = item
	}
	for _, expected := range fixture.originalItems {
		item, found := itemsByID[expected.ID]
		if !found || !sameItemSnapshot(item, expected) {
			t.Fatalf(
				"Item %q after rollback = found:%t value:%+v, want %+v",
				expected.ID,
				found,
				item,
				expected,
			)
		}
	}

	for _, continuation := range fixture.commit.ExpectedPending.Continuations {
		assertStoredRunState(t, fixture.db, continuation.RunID, "interrupted")
	}

	storedTree, checkpoint, err := fixture.processes.LoadTree(fixture.ctx, fixture.originalTree.RootID)
	if err != nil {
		t.Fatalf("load process tree after rollback: %v", err)
	}
	tree := restoredProcessTree(t, storedTree)
	if !reflect.DeepEqual(
		normalizedProcessTree(tree),
		normalizedProcessTree(fixture.originalTree),
	) {
		t.Fatalf("process tree changed after rollback:\ngot  %+v\nwant %+v", tree, fixture.originalTree)
	}
	if !reflect.DeepEqual(
		normalizedProcessCheckpoint(checkpoint),
		normalizedProcessCheckpoint(fixture.originalCheckpoint),
	) {
		t.Fatalf(
			"process checkpoint changed after rollback:\ngot  %+v\nwant %+v",
			checkpoint,
			fixture.originalCheckpoint,
		)
	}

	var terminalRuns int
	if err := fixture.db.QueryRowContext(
		fixture.ctx,
		`SELECT count(*) FROM runs WHERE session_id = ? AND state = 'terminal'`,
		fixture.rootRun.SessionID,
	).Scan(&terminalRuns); err != nil {
		t.Fatalf("count terminal Runs after rollback: %v", err)
	}
	if terminalRuns != 0 {
		t.Fatalf("terminal Runs after rollback = %d, want 0", terminalRuns)
	}
}

func normalizedProcessTree(tree core.ProcessSnapshotTree) core.ProcessSnapshotTree {
	tree.Snapshots = slices.Clone(tree.Snapshots)
	slices.SortFunc(tree.Snapshots, func(left, right core.ProcessSnapshot) int {
		return cmp.Compare(left.ID, right.ID)
	})
	return tree
}

func normalizedProcessCheckpoint(
	checkpoint execution.ProcessCheckpoint,
) execution.ProcessCheckpoint {
	checkpoint.Usage.Models = slices.Clone(checkpoint.Usage.Models)
	if len(checkpoint.Usage.Models) == 0 {
		checkpoint.Usage.Models = nil
	}
	return checkpoint
}
