package runrecovery_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/runrecovery"
)

func TestRecoverReadsAFinishedRunAfterItsSegmentExpires(t *testing.T) {
	runtime := mock.New()
	runtime.Instant = true
	runtime.Script = completedScript
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "finish"}})
	if err != nil {
		t.Fatal(err)
	}
	consumeSegment(t, opened)
	if _, err := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{RunID: opened.RunID, SegmentID: opened.SegmentID}); !runrecovery.Required(err) {
		t.Fatalf("subscribe error = %v, want a cold-recovery condition", err)
	}
	recovered, err := runrecovery.Recover(t.Context(), runtime, session.ID, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.Status != agent.RunStatusFinished || recovered.Run.Outcome.Status != agent.OutcomeCompleted || recovered.Stream.Events != nil {
		t.Fatalf("recovered state = %+v", recovered)
	}
	if len(recovered.Snapshot.Transcript) != 2 {
		t.Fatalf("transcript = %+v, want user and assistant blocks", recovered.Snapshot.Transcript)
	}
}

func TestRecoverAttachesBeforeReadingALiveRun(t *testing.T) {
	runtime := mock.New()
	runtime.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "keep running"}})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := runrecovery.Recover(t.Context(), runtime, session.ID, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.Status != agent.RunStatusRunning || recovered.Stream.RunID != opened.RunID || recovered.Stream.SegmentID != opened.SegmentID || recovered.Stream.Events == nil {
		t.Fatalf("recovered state = %+v", recovered)
	}
	conversation := agent.NewConversation()
	if err := conversation.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
		t.Fatal(err)
	}
	if conversation.Checkpoint() != recovered.Stream.HeadEventID || conversation.Checkpoint() == "" {
		t.Fatalf("recovery checkpoint = %q, head = %q", conversation.Checkpoint(), recovered.Stream.HeadEventID)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID, Reason: "test complete"}); err != nil {
		t.Fatal(err)
	}
}

func TestAttachSessionPerformsTheHeadAttachmentBeforeItsAuthoritativeRead(t *testing.T) {
	runtime := mock.New()
	runtime.Script = func(string) mock.Script {
		return mock.Script{Prelude: []mock.Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), agent.StartRun{SessionID: session.ID, Message: agent.Message{Text: "keep running"}})
	if err != nil {
		t.Fatal(err)
	}
	observed := &orderedSource{Source: runtime}
	recovered, err := runrecovery.AttachSession(t.Context(), observed, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.ID != opened.RunID || recovered.Stream.Events == nil {
		t.Fatalf("attached state = %+v", recovered)
	}
	if got := observed.snapshot(); !slices.Equal(got, []string{"read", "attach", "read"}) {
		t.Fatalf("recovery operations = %v", got)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID, Reason: "test complete"}); err != nil {
		t.Fatal(err)
	}
}

func TestAttachSessionReturnsAuthoritativeStateWhenNoStreamIsRequired(t *testing.T) {
	t.Run("waiting", func(t *testing.T) {
		runtime := mock.New()
		runtime.Instant = true
		runtime.Script = func(string) mock.Script {
			return mock.Script{Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval_1", Title: "Run checks",
				Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning},
			}}}
		}
		session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		opened, err := runtime.StartRun(t.Context(), agent.StartRun{
			SessionID: session.ID, Message: agent.Message{Text: "wait for approval"},
		})
		if err != nil {
			t.Fatal(err)
		}
		consumeSegment(t, opened)

		recovered, err := runrecovery.AttachSession(t.Context(), runtime, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Run.ID != opened.RunID || recovered.Run.Status != agent.RunStatusWaiting ||
			recovered.Stream.Events != nil || len(recovered.Snapshot.Interactions) != 1 {
			t.Fatalf("waiting session attachment = %+v", recovered)
		}
	})

	t.Run("finished", func(t *testing.T) {
		runtime := mock.New()
		runtime.Instant = true
		runtime.Script = completedScript
		session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		opened, err := runtime.StartRun(t.Context(), agent.StartRun{
			SessionID: session.ID, Message: agent.Message{Text: "finish"},
		})
		if err != nil {
			t.Fatal(err)
		}
		consumeSegment(t, opened)

		recovered, err := runrecovery.AttachSession(t.Context(), runtime, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Run.ID != opened.RunID || recovered.Run.Status != agent.RunStatusFinished || recovered.Stream.Events != nil {
			t.Fatalf("finished session attachment = %+v", recovered)
		}
	})

	t.Run("empty", func(t *testing.T) {
		runtime := mock.New()
		session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := runrecovery.AttachSession(t.Context(), runtime, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Run.ID != "" || recovered.Stream.Events != nil || len(recovered.Snapshot.Runs) != 0 {
			t.Fatalf("empty session attachment = %+v", recovered)
		}
	})
}

func consumeSegment(t *testing.T, stream agent.SegmentStream) {
	t.Helper()
	for _, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
}

func TestRequiredRecognizesOnlyColdRecoveryConditions(t *testing.T) {
	for _, err := range []error{
		agent.ErrStaleSegment, agent.ErrRunWaiting, agent.ErrRunFinished,
		agent.ErrReplayCursorInvalid, agent.ErrReplayUnavailable,
	} {
		if !runrecovery.Required(errors.Join(errors.New("adapter"), err)) {
			t.Fatalf("Required(%v) = false", err)
		}
	}
	if runrecovery.Required(agent.ErrDisconnected) {
		t.Fatal("a transport disconnect was classified as cold recovery")
	}
}

func completedScript(string) mock.Script {
	return mock.Script{Prelude: []mock.Step{
		{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "done"}}},
		{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
	}}
}

type orderedSource struct {
	runrecovery.Source

	mu         sync.Mutex
	operations []string
}

func (s *orderedSource) GetSession(ctx context.Context, id string) (agent.SessionSnapshot, error) {
	s.record("read")
	return s.Source.GetSession(ctx, id)
}

func (s *orderedSource) SubscribeRun(ctx context.Context, request agent.SubscribeRun) (agent.SegmentStream, error) {
	s.record("attach")
	return s.Source.SubscribeRun(ctx, request)
}

func (s *orderedSource) record(operation string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operations = append(s.operations, operation)
}

func (s *orderedSource) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.operations)
}
