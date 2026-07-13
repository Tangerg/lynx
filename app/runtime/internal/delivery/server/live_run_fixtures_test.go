package server

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

// blockingRunRuntime is a stub whose turn never emits or finishes, so a run
// started through it stays live in the coordinator until its context is
// canceled — the seam delivery tests use to hold a live run present (the run
// registry lives inside the Coordinator now, so tests can no longer inject a
// bare record; they start a real, blocking run instead).
type blockingRunRuntime struct {
	stubRuntime
}

func newBlockingServer(t *testing.T) *Server {
	t.Helper()
	db, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open blocking runtime store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return newTestServer(&blockingRunRuntime{stubRuntime: stubRuntime{
		sess:       sqlite.NewSessionStore(db),
		hist:       sqlite.NewTranscriptStore(db),
		interrupts: sqlite.NewInterruptStore(db),
		history:    map[string][]chat.Message{},
	}})
}

func (*blockingRunRuntime) SessionByID(context.Context, string) (session.Session, error) {
	return session.Session{ID: "ses_1", Cwd: "/work"}, nil
}

func (*blockingRunRuntime) TurnEvents(ctx context.Context, _ runs.Handle) (iter.Seq[runs.EngineEvent], error) {
	return func(func(runs.EngineEvent) bool) { <-ctx.Done() }, nil
}

func (*blockingRunRuntime) CancelTurn(context.Context, runs.Handle) error { return nil }

func (*blockingRunRuntime) Start(_ context.Context, req runs.StartTurn) (runs.Turn, error) {
	handle := turn.TurnHandle{SessionID: req.SessionID, TurnID: "turn_blocking"}
	return runs.Turn{SessionID: handle.SessionID, TurnID: handle.TurnID, Handle: handle}, nil
}

func (*blockingRunRuntime) RunSegmentEffects(runsegment.Checkpoints, runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Stores:   blockingRunStores{},
		RunState: stubRunState{},
		Tx:       func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
	})
}

type blockingRunStores struct{}

func (blockingRunStores) Interrupts() runsegment.InterruptStore                 { return nil }
func (blockingRunStores) Session() runsegment.SessionStore                      { return nil }
func (blockingRunStores) Transcript() runsegment.TranscriptStore                { return blockingTranscript{} }
func (blockingRunStores) MessageCount(context.Context, string) (int, error)     { return 0, nil }
func (blockingRunStores) GenerateTitle(context.Context, string) (string, error) { return "", nil }

type blockingTranscript struct{}

func (blockingTranscript) AppendItem(context.Context, transcript.Item) error { return nil }
func (blockingTranscript) PutRun(context.Context, transcript.Run) error      { return nil }

// startLiveRun starts a run that blocks forever (via a blockingRunRuntime the
// caller wired into the Server), waits until the coordinator has registered it,
// and schedules teardown. Use for tests that need a live run present.
func startLiveRun(t *testing.T, s *Server, cwd string) string {
	t.Helper()
	sess, err := s.sessions.Create(context.Background(), "", cwd)
	if err != nil {
		t.Fatalf("create live-run session: %v", err)
	}
	result, err := s.coordinator.Start(context.Background(), runs.StartCommand{
		SessionID:       sess.ID,
		Message:         "hold this run open",
		OpeningUserText: "hold this run open",
		NewProjector:    s.segmentProjector(nil),
	})
	if err != nil {
		t.Fatalf("start live run: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for !s.coordinator.Contains(result.RunID) {
		if time.Now().After(deadline) {
			t.Fatal("live run was not registered")
		}
		time.Sleep(time.Millisecond)
	}
	t.Cleanup(s.Close)
	return result.RunID
}
