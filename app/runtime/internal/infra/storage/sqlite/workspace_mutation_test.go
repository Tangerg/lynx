package sqlite

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

// TestWorkspaceMutationLogRoundTrip: a recorded intent surfaces in ListPending
// and clears on Complete — the record/recover/clear cycle §8.5 boots from.
func TestWorkspaceMutationLogRoundTrip(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewWorkspaceMutationStore(db)
	ctx := context.Background()

	if err := store.Record(ctx, execution.WorkspaceMutation{
		SessionID: "ses_1", Cwd: "/repo", ToRunID: "run_1", RestoreHistory: true,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := store.Record(ctx, execution.WorkspaceMutation{SessionID: "ses_2", Cwd: "/repo2", ToRunID: "run_9"}); err != nil {
		t.Fatalf("record 2: %v", err)
	}

	pending, err := store.ListPending(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	if pending[0] != (execution.WorkspaceMutation{
		SessionID: "ses_1", Cwd: "/repo", ToRunID: "run_1", RestoreHistory: true,
	}) {
		t.Fatalf("pending[0] = %+v, want the ses_1 intent verbatim", pending[0])
	}

	if err := store.Complete(ctx, "ses_1"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// Completing an already-cleared row is a no-op (boot recovery may re-complete).
	if err := store.Complete(ctx, "ses_1"); err != nil {
		t.Fatalf("re-complete: %v", err)
	}

	pending, _ = store.ListPending(ctx)
	if len(pending) != 1 || pending[0].SessionID != "ses_2" {
		t.Fatalf("pending after complete = %+v, want only ses_2", pending)
	}
}

// TestWorkspaceMutationReRecordReplaces: re-recording for the same session
// overwrites rather than duplicating (the mutation slot admits one per session).
func TestWorkspaceMutationReRecordReplaces(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewWorkspaceMutationStore(db)
	ctx := context.Background()

	_ = store.Record(ctx, execution.WorkspaceMutation{SessionID: "ses_1", Cwd: "/a", ToRunID: "run_1"})
	_ = store.Record(ctx, execution.WorkspaceMutation{SessionID: "ses_1", Cwd: "/b", ToRunID: "run_2"})

	pending, _ := store.ListPending(ctx)
	if len(pending) != 1 || pending[0].Cwd != "/b" || pending[0].ToRunID != "run_2" {
		t.Fatalf("pending = %+v, want one ses_1 row with the latest intent", pending)
	}
}
