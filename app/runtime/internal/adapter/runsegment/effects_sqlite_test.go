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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func waitingProcessSnapshot(id string, started, captured time.Time) core.ProcessSnapshot {
	return core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "chat", Digest: "digest"},
		StartedAt:     started,
		CapturedAt:    captured,
		Status:        core.StatusWaiting,
		Suspension: &agent.Suspension{
			SchemaVersion: agent.SuspensionSchemaVersion,
			ID:            "suspension-" + id,
			Kind:          agent.SuspensionTool,
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			Payload:       json.RawMessage(`{"checkpoint":true}`),
			CreatedAt:     captured,
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
	state := sqlite.NewRunStateStore(db)
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
	effects := New(Config{
		Stores:   sqliteOpeningStores{interrupts: ints, transcript: history},
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
	state := sqlite.NewRunStateStore(db)
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
	effects := New(Config{
		Stores:   sqliteOpeningStores{interrupts: ints, transcript: history},
		RunState: state,
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) },
	})
	resume := execution.ResumeDraft{RunID: "run_1", SessionID: "ses_1"}
	err = effects.CommitOpening(ctx, runs.OpeningCommit{
		Resume: &resume,
		Events: []runs.EventCommit{{
			RunID:     "run_1",
			SessionID: "ses_1",
			Run:       &transcript.Run{SessionID: "ses_1", ID: "run_1", UpdatedAt: created},
		}},
	})
	if err != nil {
		t.Fatalf("CommitOpening: %v", err)
	}
	if _, found, getErr := ints.Get(ctx, "run_1"); getErr != nil || found {
		t.Fatalf("interrupt found=%v err=%v, want consumed", found, getErr)
	}
	_, recorded, listErr := history.List(ctx, "ses_1")
	if listErr != nil || len(recorded) != 1 {
		t.Fatalf("history runs=%d err=%v, want one opening projection", len(recorded), listErr)
	}
	var stateName string
	if err := db.QueryRowContext(ctx, `SELECT state FROM runs WHERE run_id = ?`, "run_1").Scan(&stateName); err != nil || stateName != "running" {
		t.Fatalf("run state=%q err=%v, want running", stateName, err)
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
	state := sqlite.NewRunStateStore(db)
	ctx := t.Context()
	createdAt := time.Unix(1, 0).UTC()
	parkedAt := time.Unix(2, 0).UTC()
	if err := state.Admit(ctx, execution.RunDraft{RunID: "run_1", SessionID: "ses_1", CreatedAt: createdAt}); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := sqlite.NewProcessStore(db).Save(ctx, []core.ProcessSnapshot{waitingProcessSnapshot("proc_1", createdAt, parkedAt)}); err != nil {
		t.Fatalf("save process snapshot: %v", err)
	}
	question := &transcript.Question{Prompt: "Continue?"}
	open := []transcript.Interrupt{{ItemID: "item_question", Kind: transcript.QuestionInterrupt, Question: question}}
	effects := New(Config{
		Stores:    sqliteOpeningStores{interrupts: ints, transcript: history},
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

func (s sqliteOpeningStores) Interrupts() InterruptStore                          { return s.interrupts }
func (sqliteOpeningStores) Session() SessionStore                                 { return nil }
func (s sqliteOpeningStores) Transcript() TranscriptStore                         { return s.transcript }
func (sqliteOpeningStores) ToolResults() ToolResultStore                          { return nil }
func (sqliteOpeningStores) MessageCount(context.Context, string) (int, error)     { return 0, nil }
func (sqliteOpeningStores) GenerateTitle(context.Context, string) (string, error) { return "", nil }
