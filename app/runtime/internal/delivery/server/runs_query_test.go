package server

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
)

type interruptRuntime struct {
	stubRuntime
	sessionID string
	pending   []interrupts.Pending
}

func (r *interruptRuntime) ListPendingInterrupts(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	r.sessionID = sessionID
	return r.pending, nil
}

func TestListOpenInterruptsUsesRuntimeFacade(t *testing.T) {
	created := time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC)
	rt := &interruptRuntime{pending: []interrupts.Pending{
		{
			ParentRunID: "run_waiting",
			SessionID:   "ses_1",
			Interrupts:  []byte(`[{"itemId":"item_1","type":"approval"}]`),
			CreatedAt:   created,
		},
		{
			ParentRunID: "run_corrupt",
			SessionID:   "ses_1",
			Interrupts:  []byte(`{`),
			CreatedAt:   created,
		},
	}}
	s := newTestServer(rt)

	got, err := s.ListOpenInterrupts(context.Background(), protocol.ListOpenInterruptsRequest{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("list open interrupts: %v", err)
	}
	if rt.sessionID != "ses_1" {
		t.Fatalf("runtime session = %q, want ses_1", rt.sessionID)
	}
	if len(got.Data) != 1 {
		t.Fatalf("open interrupts = %+v, want only valid record", got.Data)
	}
	open := got.Data[0]
	if open.ParentRunID != "run_waiting" || open.SessionID != "ses_1" || !open.CreatedAt.Equal(created) || len(open.Interrupts) != 1 {
		t.Fatalf("wire open interrupt = %+v", open)
	}
}
