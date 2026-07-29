package runsegment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

const checkpointBuildID = "test-build"

func waitingProcessSnapshot(id string, started, parked time.Time) core.ProcessSnapshot {
	return core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "chat", Digest: "digest"},
		StartedAt:     started,
		Status:        core.StatusWaiting,
		Suspension: &agent.Suspension{
			SchemaVersion: agent.SuspensionSchemaVersion,
			ID:            "suspension-" + id,
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			CreatedAt:     parked,
		},
	}
}

// TestCommitOpeningResumeRollsBackConsume proves the critical continuation
// write-set uses one real database transaction: even though Consume executes
// before validation fails, rollback leaves the interrupt open.
func TestCommitOpeningResumeRollsBackConsume(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ints := sqlite.NewInterruptStore(db)
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	ctx := context.Background()
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_actual", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := state.Suspend(ctx, parkedRunRecord("run_actual", "ses_1", time.Now().UTC())); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	stalePending := singleRunPending(
		t,
		"run_stale", "ses_1", "proc_stale", "susp_stale", "item_stale",
		time.Now().UTC(), time.Now().UTC(),
	)
	if err := ints.Put(ctx, stalePending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	resume := execution.TreeResumeDraft{
		RootRunID: "run_stale",
		SessionID: "ses_1",
		Runs:      []execution.RunResumeDraft{{RunID: "run_stale", SegmentID: "seg_next"}},
	}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{Resume: &resume, Events: []runs.EventCommit{{RunID: "run_stale", SessionID: "ses_1"}}})
	if err == nil {
		t.Fatal("CommitOpening must reject an interrupt that does not own the active run")
	}
	if _, found, getErr := ints.Get(ctx, "run_stale"); getErr != nil || !found {
		t.Fatalf("rolled-back interrupt found=%v err=%v, want still open", found, getErr)
	}
}

func TestCommitOpeningResumeCommitsWholeWriteSet(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ints := sqlite.NewInterruptStore(db)
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	ctx := context.Background()
	created := time.Now().UTC()
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := state.Suspend(ctx, parkedRunRecord("run_1", "ses_1", created)); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	pending := singleRunPending(
		t,
		"run_1", "ses_1", "proc_1", "susp_1", "item_question",
		created, time.Now().UTC(),
	)
	if err := ints.Put(ctx, pending); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	resume := execution.TreeResumeDraft{
		RootRunID: "run_1",
		SessionID: "ses_1",
		Runs:      []execution.RunResumeDraft{{RunID: "run_1", SegmentID: "seg_next"}},
	}
	opening := runs.OpeningCommit{
		Resume: &resume,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			Items: []transcript.Item{{
				SessionID: "ses_1", RunID: "run_1", ID: "item_resumed", CreatedAt: created,
				Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "go on"}},
			}},
		}},
	}
	err = effects.CommitOpening(ctx, opening)
	if err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if _, found, getErr := ints.Get(ctx, "run_1"); getErr != nil || found {
		t.Fatalf("interrupt found=%v err=%v, want consumed", found, getErr)
	}
	recorded, listErr := history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 1 {
		t.Fatalf("history items=%d err=%v, want the opening projection", len(recorded), listErr)
	}
	var stateName string
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, "run_1").Scan(&stateName); err != nil || stateName != "running" {
		t.Fatalf("run state=%q err=%v, want running", stateName, err)
	}

	if err := effects.CommitOpening(ctx, opening); err == nil {
		t.Fatal("duplicate CommitOpening succeeded after the interrupt was consumed")
	}
	recorded, listErr = history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 1 {
		t.Fatalf("history after duplicate items=%d err=%v, want unchanged", len(recorded), listErr)
	}
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, "run_1").Scan(&stateName); err != nil || stateName != "running" {
		t.Fatalf("run state after duplicate=%q err=%v, want running", stateName, err)
	}
}

func TestCommitOpeningResumesRunTreeAtomically(t *testing.T) {
	for _, test := range []struct {
		name        string
		suspendRoot bool
		wantError   bool
	}{
		{name: "complete tree", suspendRoot: true},
		{name: "rollback after descendant resumed", wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := sqlite.Open(":memory:")
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			t.Cleanup(func() { _ = db.Close() })
			ctx := t.Context()
			state := sqlite.NewRunStore(db)
			ints := sqlite.NewInterruptStore(db)
			createdAt := time.Now().UTC()
			if err := state.Admit(ctx, execution.RunDraft{
				RunID: "run_root", SessionID: "session_1", SegmentID: "segment_root", CreatedAt: createdAt,
			}); err != nil {
				t.Fatalf("admit root: %v", err)
			}
			lineage := execution.RunLineage{
				SpawnedByItemID: "item_spawn_child",
				ParentRunID:     "run_root",
				RootRunID:       "run_root",
			}
			if err := state.Admit(ctx, execution.RunDraft{
				RunID: "run_child", SessionID: "session_1", SegmentID: "segment_child",
				SpawnedByItemID: lineage.SpawnedByItemID,
				ParentRunID:     lineage.ParentRunID,
				RootRunID:       lineage.RootRunID,
				CreatedAt:       createdAt,
			}); err != nil {
				t.Fatalf("admit child: %v", err)
			}
			childRun := interruptedTreeRun(
				"run_child",
				"session_1",
				lineage,
				createdAt,
				[]transcript.Interrupt{treeQuestion("item_child", "run_child")},
			)
			if err := state.Suspend(ctx, childRun); err != nil {
				t.Fatalf("suspend child: %v", err)
			}
			if test.suspendRoot {
				rootRun := interruptedTreeRun(
					"run_root",
					"session_1",
					execution.RunLineage{},
					createdAt,
					nil,
				)
				if err := state.Suspend(ctx, rootRun); err != nil {
					t.Fatalf("suspend root: %v", err)
				}
			}
			pending := interrupts.Pending{
				RootRunID:  "run_root",
				SessionID:  "session_1",
				TurnID:     "turn_1",
				Interrupts: childRun.Interrupts,
				Suspensions: []interrupts.SuspensionBinding{{
					InterruptItemID: "item_child",
					ProcessID:       "process_child",
					SuspensionID:    "suspension_child",
				}},
				Continuations: []interrupts.Continuation{
					{
						RunID:           "run_child",
						ProcessID:       "process_child",
						ParentProcessID: "process_root",
						SpawnCallID:     "spawn_child",
						Lineage:         lineage,
						RunCreatedAt:    createdAt,
					},
					{
						RunID:        "run_root",
						ProcessID:    "process_root",
						RunCreatedAt: createdAt,
					},
				},
				CreatedAt: createdAt.Add(time.Second),
			}
			if err := ints.Put(ctx, pending); err != nil {
				t.Fatalf("put pending: %v", err)
			}
			effects := sqliteEffects(sqliteOpeningStores{interrupts: ints}, Config{
				RunState: state,
				Tx: func(ctx context.Context, fn func(context.Context) error) error {
					return sqlite.RunInTx(ctx, db, fn)
				},
			})
			resume := execution.TreeResumeDraft{
				RootRunID: "run_root",
				SessionID: "session_1",
				Runs: []execution.RunResumeDraft{
					{RunID: "run_child", SegmentID: "segment_child_resumed"},
					{RunID: "run_root", SegmentID: "segment_root_resumed"},
				},
			}
			err = effects.CommitOpening(ctx, runs.OpeningCommit{Resume: &resume})
			if test.wantError {
				if err == nil {
					t.Fatal("CommitOpening succeeded with a root Run that was not interrupted")
				}
				assertStoredRunState(t, db, "run_child", "interrupted")
				assertStoredRunState(t, db, "run_root", "running")
				if _, found, getErr := ints.Get(ctx, "run_root"); getErr != nil || !found {
					t.Fatalf("pending after rollback found=%v err=%v, want open", found, getErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CommitOpening: %v", err)
			}
			assertStoredRunState(t, db, "run_child", "running")
			assertStoredRunState(t, db, "run_root", "running")
			if _, found, getErr := ints.Get(ctx, "run_root"); getErr != nil || found {
				t.Fatalf("pending after commit found=%v err=%v, want consumed", found, getErr)
			}
		})
	}
}

func treeQuestion(itemID, runID string) transcript.Interrupt {
	return transcript.Interrupt{
		ItemID: itemID,
		RunID:  runID,
		Kind:   execution.QuestionInterrupt,
		Question: &transcript.Question{
			Prompt: "Continue?",
			Fields: []transcript.QuestionField{{
				Name: "answer", Kind: transcript.QuestionText, Required: true,
			}},
		},
	}
}

func interruptedTreeRun(
	runID string,
	sessionID string,
	lineage execution.RunLineage,
	createdAt time.Time,
	open []transcript.Interrupt,
) transcript.Run {
	return transcript.Run{
		ID:              runID,
		SessionID:       sessionID,
		SpawnedByItemID: lineage.SpawnedByItemID,
		ParentRunID:     lineage.ParentRunID,
		RootRunID:       lineage.RootRunID,
		State:           execution.Interrupted,
		Interrupts:      open,
		CreatedAt:       createdAt,
		MessageMark:     transcript.UnknownMessageMark,
	}
}

func assertStoredRunState(t testing.TB, db *sql.DB, runID, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(t.Context(), `SELECT state FROM runs WHERE run_id = ?`, runID).Scan(&got); err != nil {
		t.Fatalf("read Run %q state: %v", runID, err)
	}
	if got != want {
		t.Fatalf("Run %q state = %q, want %q", runID, got, want)
	}
}

// TestCommitOpeningRollsBackScheduledSession proves the occurrence-owned
// Session is not durable until every fresh-opening fact is accepted. In
// particular, a rejected occurrence must not leave a session that can later be
// mistaken for a user-created conversation.
func TestCommitOpeningRollsBackScheduledSession(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	sessions := sqlite.NewSessionStore(db)
	state := sqlite.NewRunStore(db)
	history := sqlite.NewTranscriptStore(db)
	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		Sessions:        sessions,
		ScheduleFirings: sqlite.NewScheduleStore(db),
		RunState:        state,
		Tx:              func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	created := time.Now().UTC()
	draft := execution.RunDraft{RunID: "run_scheduled", SessionID: "ses_scheduled", SegmentID: "seg_open", CreatedAt: created}
	scheduled := session.Session{ID: draft.SessionID, Title: "scheduled", Cwd: "/work", StartedAt: created, UpdatedAt: created, Revision: 1}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
		Admit:            &draft,
		ScheduledSession: &scheduled,
		// No firing is seeded: Accept fails after Ensure and Admit, so the
		// test exercises rollback rather than a preflight rejection.
		ScheduleFiring: "fire_missing",
		Events: []runs.EventCommit{{
			RunID: draft.RunID, SessionID: draft.SessionID,
			Run: &transcript.Run{ID: draft.RunID, SessionID: draft.SessionID, UpdatedAt: created},
		}},
	})
	if err == nil {
		t.Fatal("CommitOpening accepted an unknown scheduled occurrence")
	}
	if _, getErr := sessions.Get(ctx, draft.SessionID); !errors.Is(getErr, session.ErrNotFound) {
		t.Fatalf("scheduled session survived rejected opening: %v", getErr)
	}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("rejected opening left run admission behind: %v", err)
	}
}

// TestCommitEventRecordsGoalTurnWithTerminalRun proves budget accounting is a
// terminal Run fact, not a best-effort follow-up by the Goal driver. Both the
// Run state and the Goal aggregate must become visible together.
func TestCommitEventRecordsGoalTurnWithTerminalRun(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	created := time.Now().UTC()
	goals := sqlite.NewGoalStore(db)
	sessions := sqlite.NewSessionStore(db)
	if err := sessions.Restore(ctx, session.Session{ID: "ses_goal", StartedAt: created, UpdatedAt: created, Revision: 1}); err != nil {
		t.Fatalf("seed goal session: %v", err)
	}
	selection := mustEffectSelection(t, "provider", "model")
	g, err := goal.New("ses_goal", "finish", selection, goal.Budget{MaxTurns: 1}, "lease_goal", created)
	if err != nil {
		t.Fatalf("new goal: %v", err)
	}
	if _, saved, err := goals.Save(ctx, g, goal.Version{}); err != nil || !saved {
		t.Fatalf("seed goal saved=%v err=%v", saved, err)
	}
	state := sqlite.NewRunStore(db)
	draft := execution.RunDraft{RunID: "run_goal", SessionID: g.SessionID, SegmentID: "seg_open", CreatedAt: created}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit goal run: %v", err)
	}
	history := sqlite.NewTranscriptStore(db)
	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		GoalTurns: goals,
		RunState:  state,
		Tx:        func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	// The watermark is already resolved, so this commit needs no message store.
	finished := finishedRunRecord(draft.RunID, draft.SessionID, execution.OutcomeCompleted)
	finished.MessageMark = 0
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, State: runs.StateTerminalize,
		Outcome: execution.OutcomeCompleted,
		Run:     finished,
		GoalTurn: &goal.TurnRecord{
			SessionID: g.SessionID, LeaseID: g.LeaseID, RunID: draft.RunID,
			Outcome: execution.OutcomeCompleted, CostUSD: 0.25, Steps: 2, CompletedAt: created,
		},
	}); err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}
	got, found, err := goals.Get(ctx, g.SessionID)
	if err != nil || !found {
		t.Fatalf("goal after terminal found=%v err=%v", found, err)
	}
	if got.Used != (goal.Usage{Turns: 1, CostUSD: 0.25, Steps: 2}) || got.Status != goal.StatusBlocked || got.Reason.Cause != goal.ReasonTurnBudgetReached {
		t.Fatalf("goal after terminal = %+v", got)
	}
	var runState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, draft.RunID).Scan(&runState); err != nil || runState != "terminal" {
		t.Fatalf("run state=%q err=%v, want terminal", runState, err)
	}
}

// TestCommitTreeBarrierProducesBootResumableTriplet is the event boundary's
// evidence for parked_tree_has_exactly_one_open_interrupt_set: one tree commit
// lands the pending set, item, and suspended Run together. Boot recovery leaves
// the coherent triplet alone while a fresh admission is refused.
func TestCommitTreeBarrierProducesBootResumableTriplet(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ints := sqlite.NewInterruptStore(db)
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	parkedAt := time.Unix(2, 0).UTC()
	if err := state.Admit(ctx, execution.RunDraft{
		RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open",
		ModelSelection: mustEffectSelection(t, "anthropic", "claude"),
		CreatedAt:      createdAt,
	}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	snapshot := waitingProcessSnapshot("proc_1", createdAt, parkedAt)
	tree := core.ProcessSnapshotTree{
		RootID:    snapshot.ID,
		Snapshots: []core.ProcessSnapshot{snapshot},
	}
	if err := sqlite.NewProcessStore(db).SaveTree(ctx, tree, execution.ProcessCheckpoint{
		BuildID: checkpointBuildID,
		Scope:   execution.TurnScope{SessionID: "ses_1"},
		Usage:   accounting.Snapshot{},
	}); err != nil {
		t.Fatalf("save process snapshot: %v", err)
	}
	question := &transcript.Question{Prompt: "Continue?"}
	open := []transcript.Interrupt{{
		ItemID:   "item_question",
		RunID:    "run_1",
		Kind:     execution.QuestionInterrupt,
		Question: question,
	}}
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	pending := singleRunPending(
		t,
		"run_1", "ses_1", "proc_1", "suspension-proc_1", "item_question",
		createdAt, parkedAt,
	)
	if err := effects.CommitTreeBarrier(ctx, runs.TreeBarrierCommit{
		Pending: pending,
		Runs: []runs.EventCommit{{
			RunID: "run_1", SessionID: "ses_1", State: runs.StateSuspend,
			Items: []transcript.Item{{
				SessionID: "ses_1", ID: "item_question", RunID: "run_1",
				Status: transcript.ItemRunning, Kind: transcript.QuestionItem,
				Question: question, CreatedAt: parkedAt,
			}},
			Run: &transcript.Run{
				SessionID: "ses_1", ID: "run_1", State: execution.Interrupted,
				Interrupts: open, CreatedAt: createdAt, UpdatedAt: parkedAt, MessageMark: -1,
			},
		}},
	}); err != nil {
		t.Fatalf("park: %v", err)
	}

	if recovered, err := state.ReconcileOrphans(ctx, func(context.Context, string) (bool, error) { return true, nil }); err != nil || recovered != 0 {
		t.Fatalf("boot reconcile = (%d, %v), want intact resumable park", recovered, err)
	}
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_next", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: parkedAt}); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("admit after intact park = %v, want ErrSessionBusy", err)
	}
}

type sqliteOpeningStores struct {
	interrupts *sqlite.InterruptStore
	transcript *sqlite.TranscriptStore
}

func sqliteEffects(stores sqliteOpeningStores, cfg Config) *Effects {
	cfg.Interrupts = stores.interrupts
	cfg.Transcript = stores.transcript
	return New(cfg)
}

// TestCommitOpeningRefusesASecondOpenRun is the integration evidence that the
// opening transaction maintains session_has_at_most_one_open_run.
//
// The store's own Admit enforces the slot, but the invariant is about the whole
// opening write-set: an admission that loses the race must not leave the items it
// had already projected. Committing the projections and then failing the admit
// would produce a session whose history mentions a run that does not exist, and no
// later write supplies it.
func TestCommitOpeningRefusesASecondOpenRun(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	created := time.Now().UTC()
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}); err != nil {
		t.Fatalf("admit the first run: %v", err)
	}

	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	second := execution.RunDraft{RunID: "run_2", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: created}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
		Admit: &second,
		Events: []runs.EventCommit{{
			RunID:     "run_2",
			SessionID: "ses_1",
			Items: []transcript.Item{{
				SessionID: "ses_1", RunID: "run_2", ID: "item_second", CreatedAt: created,
				Status: transcript.ItemCompleted, Kind: transcript.UserMessage,
				Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "me too"}},
			}},
		}},
	})
	if !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("CommitOpening against a busy session = %v, want ErrSessionBusy", err)
	}
	recorded, listErr := history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 0 {
		t.Fatalf("history items=%d err=%v, want the refused opening to have written nothing", len(recorded), listErr)
	}
	var runRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM runs WHERE session_id = ?`, "ses_1").Scan(&runRows); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runRows != 1 {
		t.Fatalf("runs rows = %d, want only the run that holds the slot", runRows)
	}
}

// TestCommitEventPersistsTheTerminalRunsResult is the integration evidence that
// the event transaction maintains terminal_run_explains_how_it_ended.
//
// The store refuses a terminal row without an outcome, but that is one table's
// rule. What the invariant is about is the durable projection a client later reads:
// a run that says it ended must come back with the facts explaining how, because no
// later write will supply them. Reading it back through ListRuns is the only way to
// see whether the commit carried them or merely flipped a state column.
func TestCommitEventPersistsTheTerminalRunsResult(t *testing.T) {
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	history := sqlite.NewTranscriptStore(db)
	state := sqlite.NewRunStore(db)
	draft := execution.RunDraft{RunID: "run_1", SessionID: "ses_1", SegmentID: "seg_open", CreatedAt: time.Unix(1, 0).UTC()}
	if err := state.Admit(ctx, draft); err != nil {
		t.Fatalf("admit: %v", err)
	}

	effects := sqliteEffects(sqliteOpeningStores{transcript: history}, Config{
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	finished := finishedRunRecord(draft.RunID, draft.SessionID, execution.OutcomeError)
	finished.MessageMark = 0
	finished.Detail = "the provider rejected the request"
	finished.Metrics = transcript.RunMetrics{Steps: 3}
	finished.Error = &transcript.Problem{Kind: transcript.ProviderRejectedProblem, Detail: "rejected"}
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: draft.RunID, SessionID: draft.SessionID, State: runs.StateTerminalize,
		Outcome: execution.OutcomeError, Run: finished,
	}); err != nil {
		t.Fatalf("CommitEvent: %v", err)
	}

	recorded, err := state.ListRuns(ctx, draft.SessionID)
	if err != nil || len(recorded) != 1 {
		t.Fatalf("ListRuns = %d runs, %v", len(recorded), err)
	}
	run := recorded[0]
	switch {
	case run.State != execution.Failed:
		t.Errorf("run state = %v, want Failed", run.State)
	case run.Outcome == nil || *run.Outcome != execution.OutcomeError:
		t.Errorf("run outcome = %v, want the outcome it terminated with", run.Outcome)
	case run.Metrics.Steps != 3:
		t.Errorf("run metrics = %+v, want the accrual the terminal commit carried", run.Metrics)
	case run.Detail != "the provider rejected the request":
		t.Errorf("run detail = %q, want the explanation it ended with", run.Detail)
	case run.FinishedAt.IsZero():
		t.Error("terminal run has no finish time")
	}
}

// parkedRunRecord is the record a park hands to Suspend: the run moving to
// Interrupted with the interrupt it is parked on.
func parkedRunRecord(runID, sessionID string, createdAt time.Time) transcript.Run {
	return transcript.Run{
		SessionID: sessionID, ID: runID, State: execution.Interrupted,
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_" + runID, Kind: execution.QuestionInterrupt,
			Question: &transcript.Question{Prompt: "continue?"},
		}},
		CreatedAt: createdAt, MessageMark: transcript.UnknownMessageMark,
	}
}
