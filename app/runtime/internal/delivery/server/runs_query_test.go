package server

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/queries"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// fakeInterruptReader backs the query coordinator's interrupt read for the
// ListOpenInterrupts wire-projection test.
type fakeInterruptReader struct {
	sessionID string
	pending   []interrupts.Pending
}

func (r *fakeInterruptReader) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	r.sessionID = sessionID
	return r.pending, nil
}

func TestListOpenInterruptsProjectsToWire(t *testing.T) {
	created := time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC)
	reader := &fakeInterruptReader{pending: []interrupts.Pending{
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
	s := &Server{queries: queries.New(queries.Dependencies{Interrupts: reader})}

	got, err := s.ListOpenInterrupts(context.Background(), protocol.ListOpenInterruptsRequest{SessionID: "ses_1"})
	if err != nil {
		t.Fatalf("list open interrupts: %v", err)
	}
	if reader.sessionID != "ses_1" {
		t.Fatalf("read session = %q, want ses_1", reader.sessionID)
	}
	if len(got.Data) != 1 {
		t.Fatalf("open interrupts = %+v, want only valid record", got.Data)
	}
	open := got.Data[0]
	if open.ParentRunID != "run_waiting" || open.SessionID != "ses_1" || !open.CreatedAt.Equal(created) || len(open.Interrupts) != 1 {
		t.Fatalf("wire open interrupt = %+v", open)
	}
}
