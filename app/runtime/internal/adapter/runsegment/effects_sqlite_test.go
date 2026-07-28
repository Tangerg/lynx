package runsegment

import (
	"context"
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
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_actual", SessionID: "ses_1", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := state.Suspend(ctx, "ses_1", "run_actual"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{RunID: "run_stale", SessionID: "ses_1"}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	resume := execution.ResumeDraft{RunID: "run_stale", SessionID: "ses_1"}
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
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_1", SessionID: "ses_1", CreatedAt: created}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := state.Suspend(ctx, "ses_1", "run_1"); err != nil {
		t.Fatalf("suspend: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{RunID: "run_1", SessionID: "ses_1"}); err != nil {
		t.Fatalf("seed interrupt: %v", err)
	}
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	resume := execution.ResumeDraft{RunID: "run_1", SessionID: "ses_1"}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
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
	})
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
	draft := execution.RunDraft{RunID: "run_scheduled", SessionID: "ses_scheduled", CreatedAt: created}
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
	draft := execution.RunDraft{RunID: "run_goal", SessionID: g.SessionID, CreatedAt: created}
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

func TestCommitEventParkProducesBootResumableTriplet(t *testing.T) {
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
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_1", SessionID: "ses_1", CreatedAt: createdAt}); err != nil {
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
	open := []transcript.Interrupt{{ItemID: "item_question", Kind: transcript.QuestionInterrupt, Question: question}}
	effects := sqliteEffects(sqliteOpeningStores{interrupts: ints, transcript: history}, Config{
		Processes: fakeProcess{processID: "proc_1"},
		RunState:  state,
		Tx:        func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	if err := effects.CommitEvent(ctx, runs.EventCommit{
		RunID: "run_1", SessionID: "ses_1", State: runs.StateSuspend,
		Interrupt: &interrupts.Pending{
			RunID: "run_1", SessionID: "ses_1", TurnID: "turn_1",
			Interrupts: open, RunCreatedAt: createdAt, CreatedAt: parkedAt,
		},
		Items: []transcript.Item{{
			SessionID: "ses_1", ID: "item_question", RunID: "run_1",
			Status: transcript.ItemRunning, Kind: transcript.QuestionItem,
			Question: question, CreatedAt: parkedAt,
		}},
		Run: &transcript.Run{
			SessionID: "ses_1", ID: "run_1", State: execution.Interrupted,
			Interrupts: open, CreatedAt: createdAt, UpdatedAt: parkedAt, MessageMark: -1,
		},
	}); err != nil {
		t.Fatalf("park: %v", err)
	}

	if recovered, err := state.ReconcileOrphans(ctx, func(context.Context, string) (bool, error) { return true, nil }); err != nil || recovered != 0 {
		t.Fatalf("boot reconcile = (%d, %v), want intact resumable park", recovered, err)
	}
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_next", SessionID: "ses_1", CreatedAt: parkedAt}); !errors.Is(err, execution.ErrSessionBusy) {
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
