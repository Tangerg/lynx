package queries

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type fakeTranscript struct {
	items   []transcript.Item
	runs    []transcript.Run
	session string
}

func (f *fakeTranscript) List(_ context.Context, sessionID string) ([]transcript.Item, []transcript.Run, error) {
	f.session = sessionID
	return f.items, f.runs, nil
}

type fakeInterrupts struct {
	pending []interrupts.Pending
	session string
}

func (f *fakeInterrupts) List(_ context.Context, sessionID string) ([]interrupts.Pending, error) {
	f.session = sessionID
	return f.pending, nil
}

// TestCoordinatorReadsDelegateToProjections: each query reads its projection
// store directly, forwarding the session key and the rows unchanged.
func TestCoordinatorReadsDelegateToProjections(t *testing.T) {
	ctx := context.Background()
	tx := &fakeTranscript{
		items: []transcript.Item{{ID: "it_1"}},
		runs:  []transcript.Run{{ID: "run_1"}},
	}
	ints := &fakeInterrupts{pending: []interrupts.Pending{{RunID: "run_1"}}}
	c := New(Dependencies{Transcript: tx, Interrupts: ints})

	items, runs, err := c.ListTranscript(ctx, "ses_1")
	if err != nil || len(items) != 1 || len(runs) != 1 || tx.session != "ses_1" {
		t.Fatalf("ListTranscript items=%d runs=%d session=%q err=%v", len(items), len(runs), tx.session, err)
	}

	pending, err := c.ListPendingInterrupts(ctx, "ses_2")
	if err != nil || len(pending) != 1 || ints.session != "ses_2" {
		t.Fatalf("ListPendingInterrupts pending=%d session=%q err=%v", len(pending), ints.session, err)
	}
}
