package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newInterruptStore(t *testing.T) *sqlite.InterruptStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewInterruptStore(db)
}

func TestInterruptStore_PutGetListDelete(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)

	p := interrupts.Pending{
		RunID:     "run_1",
		SessionID: "ses_a",
		TurnID:    "turn_1",
		Provider:  "anthropic",
		Model:     "claude-opus-4-8",
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_question", Kind: transcript.QuestionInterrupt,
			Question: &transcript.Question{Prompt: "Choose"},
		}},
		CreatedAt: time.Unix(5, 0).UTC(),
	}
	if err := store.Put(ctx, p); err != nil {
		t.Fatalf("Put: %v", err)
	}
	// UPSERT overwrite.
	p.SessionID = "ses_b"
	if err := store.Put(ctx, p); err != nil {
		t.Fatalf("Put overwrite: %v", err)
	}

	got, ok, err := store.Get(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.SessionID != "ses_b" || len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "item_question" || !got.CreatedAt.Equal(time.Unix(5, 0).UTC()) {
		t.Fatalf("Get returned %+v", got)
	}
	// Per-run model selection round-trips (T1.4 — cross-restart rehydrate rebuilds
	// the SAME model client instead of dropping to the default).
	if got.Provider != "anthropic" || got.Model != "claude-opus-4-8" {
		t.Fatalf("Get provider/model = %q/%q, want anthropic/claude-opus-4-8", got.Provider, got.Model)
	}

	if list, _ := store.List(ctx, "ses_b"); len(list) != 1 {
		t.Fatalf("List(ses_b) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, ""); len(list) != 1 {
		t.Fatalf("List(all) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, "nope"); len(list) != 0 {
		t.Fatalf("List(nope) = %d, want 0", len(list))
	}

	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete not idempotent: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "run_1"); ok {
		t.Fatal("Get after Delete: still present")
	}
}

// TestInterruptStore_ConsumeIsAtomic pins the resume-idempotency fix: Consume
// reads AND deletes the pending interrupt in one statement, so two concurrent
// resumes can't both claim it (the second gets ok=false and backs off, instead
// of both rebuilding the parked process and re-firing the approved tool). Also
// exercises that modernc SQLite supports DELETE ... RETURNING.
func TestInterruptStore_ConsumeIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := newInterruptStore(t)

	// Nothing recorded → ok=false.
	if _, ok, err := store.Consume(ctx, "run_x"); err != nil || ok {
		t.Fatalf("Consume(empty) = ok=%v err=%v, want ok=false", ok, err)
	}

	if err := store.Put(ctx, interrupts.Pending{
		RunID:     "run_1",
		SessionID: "ses_a",
		ProcessID: "proc_1",
		Interrupts: []transcript.Interrupt{{
			ItemID: "item_approval", Kind: transcript.ApprovalInterrupt,
			Approval: &transcript.Approval{Risk: tool.RiskHigh},
		}},
		CreatedAt: time.Unix(7, 0).UTC(),
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// First consume returns the full record.
	got, ok, err := store.Consume(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Consume: ok=%v err=%v", ok, err)
	}
	if got.ProcessID != "proc_1" || len(got.Interrupts) != 1 || got.Interrupts[0].ItemID != "item_approval" ||
		got.Interrupts[0].Approval == nil || got.Interrupts[0].Approval.Risk != tool.RiskHigh {
		t.Fatalf("Consume returned %+v", got)
	}

	// Second consume finds nothing — the record was removed atomically with
	// the read, so a racing resume can't re-fire the tool.
	if _, ok, err := store.Consume(ctx, "run_1"); err != nil || ok {
		t.Fatalf("second Consume = ok=%v err=%v, want ok=false — record must be gone", ok, err)
	}
}

func TestInterruptStore_ProcessSnapshotHasOneOwner(t *testing.T) {
	store := newInterruptStore(t)
	ctx := t.Context()
	for _, runID := range []string{"run_1", "run_2"} {
		err := store.Put(ctx, interrupts.Pending{
			RunID: runID, SessionID: "ses_" + runID, TurnID: "turn_" + runID,
			ProcessID: "proc_shared", CreatedAt: time.Unix(1, 0),
		})
		if runID == "run_1" && err != nil {
			t.Fatalf("first Put: %v", err)
		}
		if runID == "run_2" && err == nil {
			t.Fatal("second Put reused a process snapshot owned by another interrupt")
		}
	}
}
