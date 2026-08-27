package runsegment

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/app/runtime/internal/testsupport/itemfixture"
	runfixture "github.com/Tangerg/scope/app/runtime/internal/testsupport/runfixture"
)

type failingWaitingCheckpointStore struct {
	ExecutorCheckpointStore
	err error
}

func (f failingWaitingCheckpointStore) SaveCheckpoint(context.Context, runs.ExecutorCheckpoint) error {
	return f.err
}

type failingWaitingItemReplacer struct {
	ItemReplacer
	failItemID string
	err        error
}

func (f failingWaitingItemReplacer) ReplaceItem(
	ctx context.Context,
	expected transcript.Item,
	replacement transcript.Item,
) error {
	if f.failItemID == "" || f.failItemID == expected.ID() {
		return f.err
	}
	return f.ItemReplacer.ReplaceItem(ctx, expected, replacement)
}

type failingWaitingRunWriter struct {
	RunStore
	resumeErr      error
	terminalizeErr error
	recordErr      error
}

func (f failingWaitingRunWriter) RecordRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	segmentID string,
	commitID string,
) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	return f.RunStore.RecordRunCommit(ctx, sessionID, runID, segmentID, commitID)
}

func (f failingWaitingRunWriter) RecordWaitingRunCommit(
	ctx context.Context,
	sessionID string,
	runID string,
	commitID string,
) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	return f.RunStore.RecordWaitingRunCommit(ctx, sessionID, runID, commitID)
}

func (f failingWaitingRunWriter) Resume(
	ctx context.Context,
	sessionID string,
	draft run.ResumeDraft,
	resumedAt time.Time,
) error {
	if f.resumeErr != nil {
		return f.resumeErr
	}
	return f.RunStore.Resume(ctx, sessionID, draft, resumedAt)
}

func (f failingWaitingRunWriter) Terminalize(
	ctx context.Context,
	run run.Run,
) error {
	if f.terminalizeErr != nil {
		return f.terminalizeErr
	}
	return f.RunStore.Terminalize(ctx, run)
}

type failingWaitingInterruptStore struct {
	InterruptStore
	putErr error
}

func (f failingWaitingInterruptStore) Open(
	ctx context.Context,
	pending runs.Pending,
) error {
	if f.putErr != nil {
		return f.putErr
	}
	return f.InterruptStore.Open(ctx, pending)
}

type failingWaitingTranscriptStore struct {
	TranscriptStore
	appendErr error
}

func (f failingWaitingTranscriptStore) AppendItem(
	ctx context.Context,
	item transcript.Item,
) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	return f.TranscriptStore.AppendItem(ctx, item)
}

func TestCommitWaitingSubtreeCancellationRejectsStalePendingWithoutMutation(t *testing.T) {
	fixture := newWaitingCancellationSQLiteFixture(t)
	changedPending := fixture.commit.ExpectedPending
	changedPending.ExecutorID = "turn_replaced"
	if _, found, err := fixture.interrupts.Consume(fixture.ctx, changedPending.SessionID, changedPending.RootRunID); err != nil || !found {
		t.Fatalf("consume original Pending fixture: found=%t err=%v", found, err)
	}
	if err := fixture.interrupts.Open(fixture.ctx, changedPending); err != nil {
		t.Fatalf("replace Pending fixture: %v", err)
	}

	_, err := fixture.effects.CommitWaitingSubtreeCancellation(
		fixture.ctx,
		fixture.commit,
	)
	if !errors.Is(err, runs.ErrSessionBusy) {
		t.Fatalf("stale Pending error = %v, want ErrSessionBusy", err)
	}
	if !strings.Contains(err.Error(), fixture.rootRun.ID()) {
		t.Fatalf("stale Pending error = %q, want root Run identity", err)
	}
	assertWaitingCancellationUnchanged(t, fixture, changedPending)
}

func TestCommitWaitingSubtreeCancellationRejectsMismatchedCheckpointBindingWithoutMutation(t *testing.T) {
	for name, mutate := range map[string]func(*runs.ExecutorCheckpoint){
		"root":             func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.RootMemberID = "other_root" },
		"session":          func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Scope.SessionID = "other_session" },
		"goal incarnation": func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Scope.GoalIncarnationID = "other_goal" },
		"limits":           func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.Limits.MaxTotalTokens++ },
		"provider": func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.ModelSelection, _ = modelref.New("openai", "model")
		},
		"model": func(checkpoint *runs.ExecutorCheckpoint) {
			checkpoint.ModelSelection, _ = modelref.New("anthropic", "other-model")
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newWaitingCancellationSQLiteFixture(t)
			mutate(&fixture.commit.Checkpoint)
			_, err := fixture.effects.CommitWaitingSubtreeCancellation(fixture.ctx, fixture.commit)
			if !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
				t.Fatalf("ownership error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
			assertWaitingCancellationUnchanged(t, fixture, fixture.commit.ExpectedPending)
		})
	}
}

// TestCommitWaitingSubtreeCancellationRejectsRunContinuationFactDriftWithoutMutation
// proves parked_continuation_matches_run_facts at the waiting-subtree transaction:
// a forged terminal projection cannot rewrite the canceled Run's admitted facts.
func TestCommitWaitingSubtreeCancellationRejectsRunContinuationFactDriftWithoutMutation(t *testing.T) {
	for name, mutate := range map[string]func(*runs.WaitingSubtreeCancellationCommit){
		"cumulative metrics": func(commit *runs.WaitingSubtreeCancellationCommit) {
			commit.TerminalRuns[0] = mutatedRun(commit.TerminalRuns[0], func(snapshot *run.Snapshot) {
				snapshot.Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Steps: snapshot.Metrics.Steps() + 1})
			})
		},
		"frozen limits": func(commit *runs.WaitingSubtreeCancellationCommit) {
			commit.TerminalRuns[0] = mutatedRun(commit.TerminalRuns[0], func(snapshot *run.Snapshot) {
				snapshot.Limits.MaxSteps++
			})
		},
		"root run capabilities": func(commit *runs.WaitingSubtreeCancellationCommit) {
			commit.RootRun = mutatedRun(commit.RootRun, func(snapshot *run.Snapshot) {
				snapshot.Capabilities.ChildRuns = false
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newWaitingCancellationSQLiteFixture(t)
			mutate(&fixture.commit)

			if _, err := fixture.effects.CommitWaitingSubtreeCancellation(fixture.ctx, fixture.commit); err == nil {
				t.Fatal("waiting subtree cancellation accepted contradictory Run and continuation facts")
			}
			assertWaitingCancellationUnchanged(t, fixture, fixture.commit.ExpectedPending)
		})
	}
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
				fixture.replaceEffects(func(config *Config) {
					config.ExecutorCheckpoints = failingWaitingCheckpointStore{
						ExecutorCheckpointStore: fixture.checkpoints,
						err:                     injected,
					}
				})
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
			name:      "terminal Run",
			operation: "terminalize canceled Run",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.State = failingWaitingRunWriter{
						RunStore:       fixture.runState,
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
					config.State = failingWaitingRunWriter{
						RunStore:  fixture.runState,
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
					RunID:     fixture.rootRun.ID(),
					SessionID: fixture.rootRun.SessionID(),
					SegmentID: fixture.commit.Resume.Runs[0].SegmentID,
					Items: []transcript.Item{itemfixture.MustRestore(itemfixture.Input{
						ID:         "item_root_continuation",
						SessionID:  fixture.rootRun.SessionID(),
						RunID:      fixture.rootRun.ID(),
						Status:     transcript.ItemCompleted,
						Kind:       transcript.UserMessage,
						OccurredAt: fixture.rootRun.UpdatedAt(),
						Content: []transcript.ContentBlock{{
							Kind: transcript.TextContent,
							Text: "continue",
						}},
					})},
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
			name:              "waiting command receipt",
			survivingBoundary: true,
			operation:         "record waiting cancellation commit receipt",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.State = failingWaitingRunWriter{
						RunStore:  fixture.runState,
						recordErr: injected,
					}
				})
			},
		},
		{
			name:      "resumed command receipt",
			operation: "record resumed waiting cancellation commit receipt",
			configure: func(fixture *waitingCancellationSQLiteFixture, injected error) {
				fixture.replaceEffects(func(config *Config) {
					config.State = failingWaitingRunWriter{
						RunStore:  fixture.runState,
						recordErr: injected,
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
				fixture.childRun.ID(),
				fixture.rootRun.ID(),
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

func (w *waitingCancellationSQLiteFixture) replaceEffects(
	configure func(*Config),
) {
	config := Config{
		Interrupts:          w.interrupts,
		Transcript:          w.transcript,
		ItemReplacer:        w.transcript,
		Conversation:        w.conversation,
		State:               w.runState,
		ExecutorCheckpoints: w.checkpoints,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, w.db, fn)
		},
	}
	configure(&config)
	w.effects = mustNewEffects(config)
}

func assertWaitingCancellationUnchanged(
	t *testing.T,
	fixture waitingCancellationSQLiteFixture,
	expectedPending runs.Pending,
) {
	t.Helper()
	pending, found, err := fixture.interrupts.Get(fixture.ctx, fixture.rootRun.ID())
	if err != nil || !found || !pending.Equal(expectedPending) {
		t.Fatalf(
			"Pending after rollback = found:%t value:%+v err:%v, want %+v",
			found,
			pending,
			err,
			expectedPending,
		)
	}

	items, err := fixture.transcript.List(fixture.ctx, fixture.rootRun.SessionID())
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
		itemsByID[item.ID()] = item
	}
	for _, expected := range fixture.originalItems {
		item, found := itemsByID[expected.ID()]
		if !found || !sameItemSnapshot(item, expected) {
			t.Fatalf(
				"Item %q after rollback = found:%t value:%+v, want %+v",
				expected.ID(),
				found,
				item,
				expected,
			)
		}
	}

	for _, continuation := range fixture.commit.ExpectedPending.Continuations {
		assertStoredRunState(t, fixture.db, continuation.RunID, "waiting")
	}

	checkpoint, err := fixture.checkpoints.LoadCheckpoint(
		fixture.ctx,
		fixture.originalCheckpoint.RootMemberID,
	)
	if err != nil {
		t.Fatalf("load executor checkpoint after rollback: %v", err)
	}
	if !reflect.DeepEqual(
		normalizedExecutorCheckpoint(checkpoint),
		normalizedExecutorCheckpoint(fixture.originalCheckpoint),
	) {
		t.Fatalf(
			"executor checkpoint changed after rollback:\ngot  %+v\nwant %+v",
			checkpoint,
			fixture.originalCheckpoint,
		)
	}

	var terminalRuns int
	if scanErr := fixture.db.QueryRowContext(
		fixture.ctx,
		`SELECT count(*) FROM runs WHERE session_id = ? AND state = 'terminal'`,
		fixture.rootRun.SessionID(),
	).Scan(&terminalRuns); scanErr != nil {
		t.Fatalf("count terminal Runs after rollback: %v", scanErr)
	}
	if terminalRuns != 0 {
		t.Fatalf("terminal Runs after rollback = %d, want 0", terminalRuns)
	}
	messages, err := fixture.conversation.Read(fixture.ctx, fixture.rootRun.SessionID())
	if err != nil || len(messages) != 0 {
		t.Fatalf("conversation after rollback = %+v err=%v, want empty", messages, err)
	}
}

func normalizedExecutorCheckpoint(
	checkpoint runs.ExecutorCheckpoint,
) runs.ExecutorCheckpoint {
	checkpoint.Usage.Models = slices.Clone(checkpoint.Usage.Models)
	if len(checkpoint.Usage.Models) == 0 {
		checkpoint.Usage.Models = nil
	}
	return checkpoint
}
