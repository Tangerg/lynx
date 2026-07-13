package runsegment

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

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
	if err := state.Suspend(ctx, "ses_1"); err != nil {
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
	err = effects.CommitOpening(ctx, runs.OpeningCommit{Resume: &resume, Events: []execution.EventCommit{{SessionID: "ses_1"}}})
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
	if err := state.Suspend(ctx, "ses_1"); err != nil {
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
		Events: []execution.EventCommit{{
			SessionID: "ses_1",
			Run:       &transcript.Run{SessionID: "ses_1", RunID: "run_1", Blob: []byte(`{"id":"run_1"}`), UpdatedAt: created},
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

type sqliteOpeningStores struct {
	interrupts *sqlite.InterruptStore
	transcript *sqlite.TranscriptStore
}

func (s sqliteOpeningStores) Interrupts() InterruptStore   { return s.interrupts }
func (sqliteOpeningStores) Session() SessionStore          { return nil }
func (s sqliteOpeningStores) Transcript() TranscriptStore { return s.transcript }
func (sqliteOpeningStores) MessageCount(context.Context, string) (int, error) { return 0, nil }
func (sqliteOpeningStores) GenerateTitle(context.Context, string) (string, error) { return "", nil }
