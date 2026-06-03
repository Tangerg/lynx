package storage_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
	"github.com/Tangerg/lynx/lyra/internal/storage"
)

func TestFileInterruptStore_PutGetListDelete(t *testing.T) {
	withTempHome(t)
	store, err := storage.NewFileInterruptStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	p := interrupts.Pending{
		ParentRunID: "run_1",
		SessionID:   "ses_a",
		TurnID:      "turn_1",
		Interrupts:  json.RawMessage(`[{"kind":"approval"}]`),
		CreatedAt:   time.Unix(0, 0).UTC(),
	}
	if err := store.Put(ctx, p); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, ok, err := store.Get(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.SessionID != "ses_a" || string(got.Interrupts) != `[{"kind":"approval"}]` {
		t.Fatalf("Get returned %+v", got)
	}

	if list, _ := store.List(ctx, "ses_a"); len(list) != 1 {
		t.Fatalf("List(ses_a) = %d, want 1", len(list))
	}
	if list, _ := store.List(ctx, "ses_other"); len(list) != 0 {
		t.Fatalf("List(ses_other) = %d, want 0", len(list))
	}

	if err := store.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := store.Get(ctx, "run_1"); ok {
		t.Fatal("Get after Delete: still present")
	}
}

// TestFileInterruptStore_Persistence verifies entries survive a reopen —
// the whole point of the durable backend.
func TestFileInterruptStore_Persistence(t *testing.T) {
	withTempHome(t)
	ctx := context.Background()

	s1, _ := storage.NewFileInterruptStore()
	if err := s1.Put(ctx, interrupts.Pending{ParentRunID: "run_x", SessionID: "s", CreatedAt: time.Unix(1, 0).UTC()}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	s2, err := storage.NewFileInterruptStore() // reopen — same LYRA_HOME
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok, _ := s2.Get(ctx, "run_x"); !ok {
		t.Fatal("entry did not survive reopen")
	}
}
