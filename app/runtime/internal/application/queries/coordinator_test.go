package queries

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"

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

func (f *fakeTranscript) ListRuns(_ context.Context, sessionID string) ([]transcript.Run, error) {
	f.session = sessionID
	return f.runs, nil
}

type fakeHistory struct {
	msgs    []chat.Message
	session string
	err     error
}

func (f *fakeHistory) Read(_ context.Context, sessionID string) ([]chat.Message, error) {
	f.session = sessionID
	return f.msgs, f.err
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
		items: []transcript.Item{{ItemID: "it_1"}},
		runs:  []transcript.Run{{RunID: "run_1"}},
	}
	hist := &fakeHistory{msgs: []chat.Message{chat.NewUserMessage("hi")}}
	ints := &fakeInterrupts{pending: []interrupts.Pending{{RunID: "run_1"}}}
	c := New(Dependencies{Transcript: tx, History: hist, Interrupts: ints})

	items, runs, err := c.ListTranscript(ctx, "ses_1")
	if err != nil || len(items) != 1 || len(runs) != 1 || tx.session != "ses_1" {
		t.Fatalf("ListTranscript items=%d runs=%d session=%q err=%v", len(items), len(runs), tx.session, err)
	}

	gotRuns, err := c.ListTranscriptRuns(ctx, "ses_2")
	if err != nil || len(gotRuns) != 1 || tx.session != "ses_2" {
		t.Fatalf("ListTranscriptRuns runs=%d session=%q err=%v", len(gotRuns), tx.session, err)
	}

	msgs, err := c.ReadHistory(ctx, "ses_3")
	if err != nil || len(msgs) != 1 || hist.session != "ses_3" {
		t.Fatalf("ReadHistory msgs=%d session=%q err=%v", len(msgs), hist.session, err)
	}

	pending, err := c.ListPendingInterrupts(ctx, "ses_4")
	if err != nil || len(pending) != 1 || ints.session != "ses_4" {
		t.Fatalf("ListPendingInterrupts pending=%d session=%q err=%v", len(pending), ints.session, err)
	}
}

// TestCoordinatorSurfacesReadError: a projection read error is returned verbatim.
func TestCoordinatorSurfacesReadError(t *testing.T) {
	boom := errors.New("store down")
	c := New(Dependencies{History: &fakeHistory{err: boom}})
	if _, err := c.ReadHistory(context.Background(), "ses_1"); !errors.Is(err, boom) {
		t.Fatalf("ReadHistory err = %v, want %v", err, boom)
	}
}
