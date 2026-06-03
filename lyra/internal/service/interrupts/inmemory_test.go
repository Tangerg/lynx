package interrupts_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/service/interrupts"
)

func TestInMemory_PutListGetDelete(t *testing.T) {
	ctx := context.Background()
	s := interrupts.NewInMemory()

	p := interrupts.Pending{
		ParentRunID: "run_1",
		SessionID:   "ses_a",
		TurnID:      "run_1",
		Interrupts:  json.RawMessage(`[{"kind":"approval"}]`),
		CreatedAt:   time.Unix(0, 0).UTC(),
	}
	if err := s.Put(ctx, p); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, ok, err := s.Get(ctx, "run_1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.TurnID != "run_1" || got.SessionID != "ses_a" {
		t.Fatalf("Get returned %+v", got)
	}

	// Session filter: matching + non-matching.
	if list, _ := s.List(ctx, "ses_a"); len(list) != 1 {
		t.Fatalf("List(ses_a) = %d, want 1", len(list))
	}
	if list, _ := s.List(ctx, "ses_other"); len(list) != 0 {
		t.Fatalf("List(ses_other) = %d, want 0", len(list))
	}
	if list, _ := s.List(ctx, ""); len(list) != 1 {
		t.Fatalf("List(all) = %d, want 1", len(list))
	}

	if err := s.Delete(ctx, "run_1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, _ := s.Get(ctx, "run_1"); ok {
		t.Fatalf("Get after Delete: still present")
	}
}
