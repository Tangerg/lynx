package server

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/core/chat"

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

func (*blockingRunRuntime) TurnEvents(ctx context.Context, _ runs.TurnRef) (iter.Seq[runs.EngineEvent], error) {
	return func(func(runs.EngineEvent) bool) { <-ctx.Done() }, nil
}

func (*blockingRunRuntime) CancelTurn(context.Context, runs.TurnRef) error { return nil }

func (*blockingRunRuntime) PrepareStart(_ context.Context, req runs.StartTurn) (runs.TurnRef, error) {
	handle := turn.TurnHandle{SessionID: req.SessionID, TurnID: "turn_blocking"}
	return runs.TurnRef{SessionID: handle.SessionID, TurnID: handle.TurnID}, nil
}

func (*blockingRunRuntime) Activate(context.Context, runs.TurnRef) error { return nil }

func (*blockingRunRuntime) RunSegmentEffects(runsegment.Checkpoints, runsegment.FileChangePublisher) *runsegment.Effects {
	return runsegment.New(runsegment.Config{
		Transcript: blockingTranscript{},
		RunState:   stubRunState{},
		Tx:         func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
	})
}

type blockingTranscript struct{}

func (blockingTranscript) AppendItem(context.Context, transcript.Item) error { return nil }
func (blockingTranscript) PutRun(context.Context, transcript.Run) error      { return nil }

// startLiveRun starts a run that blocks forever (via a blockingRunRuntime the
// caller wired into the Server), waits until the coordinator has registered it,
// and schedules teardown. Use for tests that need a live run present.
func startLiveRun(t *testing.T, s *Server, cwd string) string {
	t.Helper()
	sess, err := s.sessions.CreateView(context.Background(), "", cwd)
	if err != nil {
		t.Fatalf("create live-run session: %v", err)
	}
	result, err := s.coordinator.Start(context.Background(), runs.StartCommand{
		SessionID: sess.ID,
		Input:     []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hold this run open"}},
	})
	if err != nil {
		t.Fatalf("start live run: %v", err)
	}
	probeCtx, cancel := context.WithCancel(context.Background())
	_, _, live := s.coordinator.SubscribeLive(probeCtx, result.RunID, "")
	cancel()
	if !live {
		t.Fatal("Start returned before the live run was registered")
	}
	t.Cleanup(s.Close)
	return result.RunID
}
