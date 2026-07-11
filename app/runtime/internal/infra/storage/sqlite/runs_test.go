package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newRunStores(t *testing.T) (*sqlite.RunStateStore, *sqlite.InterruptStore) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewRunStateStore(db), sqlite.NewInterruptStore(db)
}

func runDraft(runID, sessionID string) execution.RunDraft {
	return execution.RunDraft{RunID: runID, SessionID: sessionID, CreatedAt: time.Unix(0, 0)}
}

// TestRunAdmitEnforcesOneActivePerSession proves the durable §8.2 guarantee: the
// partial unique index rejects a second non-terminal run for the same session,
// a different session is independent, and terminalizing frees the slot.
func TestRunAdmitEnforcesOneActivePerSession(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("first admit: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_2", "ses_A")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("second admit err = %v, want ErrSessionBusy", err)
	}
	if err := store.Admit(ctx, runDraft("run_3", "ses_B")); err != nil {
		t.Fatalf("other-session admit: %v", err)
	}
	if err := store.Terminalize(ctx, "ses_A", execution.OutcomeCompleted.String()); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_4", "ses_A")); err != nil {
		t.Fatalf("re-admit after terminal: %v", err)
	}
}

// TestTerminalizeIsIdempotent: terminalizing a session with no non-terminal run
// (already terminal / never admitted) is a no-op, not an error.
func TestTerminalizeIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store, _ := newRunStores(t)

	if err := store.Terminalize(ctx, "ses_unknown", execution.OutcomeCompleted.String()); err != nil {
		t.Fatalf("terminalize unknown: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_1", "ses_A")); err != nil {
		t.Fatalf("admit: %v", err)
	}
	if err := store.Terminalize(ctx, "ses_A", execution.OutcomeCompleted.String()); err != nil {
		t.Fatalf("terminalize: %v", err)
	}
	if err := store.Terminalize(ctx, "ses_A", execution.OutcomeCompleted.String()); err != nil {
		t.Fatalf("second terminalize (idempotent): %v", err)
	}
}

// TestReconcileOrphansSweepsCrashedButPreservesParked: a boot sweep terminalizes
// a running run whose process is gone with no open interrupt (a crash), but
// leaves a running run whose session has a parked interrupt (resumable) — so a
// crash frees its session while parked runs stay resumable across restart.
func TestReconcileOrphansSweepsCrashedButPreservesParked(t *testing.T) {
	ctx := context.Background()
	store, ints := newRunStores(t)

	if err := store.Admit(ctx, runDraft("run_crash", "ses_crash")); err != nil {
		t.Fatalf("admit crash: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_park", "ses_park")); err != nil {
		t.Fatalf("admit park: %v", err)
	}
	if err := ints.Put(ctx, interrupts.Pending{
		ParentRunID: "run_park",
		SessionID:   "ses_park",
		Interrupts:  json.RawMessage("[]"),
		CreatedAt:   time.Unix(0, 0),
	}); err != nil {
		t.Fatalf("put interrupt: %v", err)
	}

	swept, err := store.ReconcileOrphans(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if swept != 1 {
		t.Fatalf("swept = %d, want 1 (only the crashed orphan)", swept)
	}
	if err := store.Admit(ctx, runDraft("run_crash2", "ses_crash")); err != nil {
		t.Fatalf("re-admit swept session: %v", err)
	}
	if err := store.Admit(ctx, runDraft("run_park2", "ses_park")); !errors.Is(err, execution.ErrSessionBusy) {
		t.Fatalf("parked-session admit err = %v, want ErrSessionBusy (preserved)", err)
	}
}
