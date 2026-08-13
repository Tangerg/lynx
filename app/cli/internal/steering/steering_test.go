package steering

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type steerRuntimeStub struct {
	requests []agent.SteerRun
	err      error
}

func (runtime *steerRuntimeStub) SteerRun(_ context.Context, request agent.SteerRun) error {
	runtime.requests = append(runtime.requests, request.Clone())
	return runtime.err
}

func TestRecoverReplaysAndAcknowledgesTheExactDurableSteer(t *testing.T) {
	store, pending, window := stagedSteer(t)
	runtime := new(steerRuntimeStub)
	window.Now = func() time.Time { return pending.StagedAt.Add(time.Minute) }
	if err := Recover(t.Context(), runtime, store, window, retry.Backoff{}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.requests) != 1 || !runtime.requests[0].Equal(pending.Command) {
		t.Fatalf("replayed requests = %+v", runtime.requests)
	}
	if _, found := store.PendingSteer(pending.SessionID); found {
		t.Fatal("acknowledged steer remains pending")
	}
	history := store.History()
	if len(history) != 1 || !history[0].Equal(pending.Command.Message) {
		t.Fatalf("accepted steer history = %+v", history)
	}
}

func TestRecoverReturnsAttachmentsAfterAReplayableRefusal(t *testing.T) {
	store, pending, window := stagedSteer(t)
	runtime := &steerRuntimeStub{err: agent.ErrStaleSegment}
	window.Now = func() time.Time { return pending.StagedAt.Add(time.Minute) }
	if err := Recover(t.Context(), runtime, store, window, retry.Backoff{}); err != nil {
		t.Fatal(err)
	}
	if _, found := store.PendingSteer(pending.SessionID); found {
		t.Fatal("rejected steer remains pending")
	}
	draft, found, err := store.Draft(pending.SessionID)
	if err != nil || !found || len(draft.Attachments) != 1 ||
		draft.Attachments[0] != pending.Command.Message.Attachments[0] {
		t.Fatalf("recovered draft = %+v, found %t, error %v", draft, found, err)
	}
}

func TestRecoverRefusesToGuessAtOrAfterTheReplayDeadline(t *testing.T) {
	for _, offset := range []time.Duration{0, time.Nanosecond} {
		t.Run(offset.String(), func(t *testing.T) {
			store, pending, window := stagedSteer(t)
			runtime := new(steerRuntimeStub)
			window.Now = func() time.Time { return pending.ReplayUntil.Add(offset) }
			err := Recover(t.Context(), runtime, store, window, retry.Backoff{})
			if err == nil {
				t.Fatal("expired replay unexpectedly succeeded")
			}
			if len(runtime.requests) != 0 {
				t.Fatalf("expired replay reached runtime: %+v", runtime.requests)
			}
			if durable, found := store.PendingSteer(pending.SessionID); !found || !durable.Command.Equal(pending.Command) {
				t.Fatalf("expired pending steer = %+v, found %t", durable, found)
			}
		})
	}
}

func TestDeliverPreservesACommandRejectedByAnotherRuntimeStore(t *testing.T) {
	_, pending, _ := stagedSteer(t)
	runtime := &steerRuntimeStub{err: agent.ErrCommandStoreMismatch}
	result, err := Deliver(t.Context(), runtime, pending, retry.Backoff{})
	if !errors.Is(err, agent.ErrCommandStoreMismatch) || result.Outcome != Unknown {
		t.Fatalf("store mismatch settlement = outcome %v, error %v", result.Outcome, err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("store mismatch attempts = %+v", runtime.requests)
	}
}

func stagedSteer(t *testing.T) (*workbench.Store, workbench.PendingSteer, ReplayWindow) {
	t.Helper()
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	stagedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	window := ReplayWindow{
		Namespace: "runtime-test", Retention: time.Hour,
		Now: func() time.Time { return stagedAt },
	}
	attachment := agent.Attachment{
		ID: "att_notes", Kind: agent.AttachmentText, Name: "notes.txt",
		Path: filepath.Join(t.TempDir(), "notes.txt"), MimeType: "text/plain", Size: 5,
	}
	request := agent.SteerRun{
		CommandID: "cli_22222222222222222222222222222222",
		RunID:     "run_1", SegmentID: "seg_1",
		Message: agent.Message{Text: "inspect the parser", Attachments: []agent.Attachment{attachment}},
	}
	source := agent.Message{Text: "/steer inspect the parser", Attachments: []agent.Attachment{attachment}}
	if err := store.SaveDraft("ses_1", source); err != nil {
		t.Fatal(err)
	}
	pending, err := Stage(store, "ses_1", request, source, window)
	if err != nil {
		t.Fatal(err)
	}
	return store, pending, window
}
