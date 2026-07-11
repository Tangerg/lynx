package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

type interruptRuntimeStore struct {
	sessionID string
	pending   []interrupts.Pending
}

var _ interruptStore = (*interruptRuntimeStore)(nil)

func (s *interruptRuntimeStore) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	s.sessionID = sessionID
	return s.pending, nil
}

func (*interruptRuntimeStore) Put(context.Context, interrupts.Pending) error { return nil }

func TestRuntimeListPendingInterrupts(t *testing.T) {
	store := &interruptRuntimeStore{pending: []interrupts.Pending{{ParentRunID: "run_waiting"}}}
	rt := &Runtime{interrupts: store}

	got, err := rt.ListPendingInterrupts(context.Background(), "ses_1")
	if err != nil {
		t.Fatalf("list pending interrupts: %v", err)
	}
	if store.sessionID != "ses_1" {
		t.Fatalf("store session = %q, want ses_1", store.sessionID)
	}
	if len(got) != 1 || got[0].ParentRunID != "run_waiting" {
		t.Fatalf("pending = %+v", got)
	}
}
