package server

import (
	"context"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/lifecycle"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/runsegment"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/turn"
)

func TestRunHandleCancelLinearizesAfterInterruptCommit(t *testing.T) {
	commitStarted := make(chan struct{})
	releaseCommit := make(chan struct{})
	canceled := make(chan struct{})
	live := &runHandle{cancel: func() { close(canceled) }}

	commitDone := make(chan struct{})
	go func() {
		defer close(commitDone)
		committed, err := live.commitInterrupt(func() error {
			close(commitStarted)
			<-releaseCommit
			return nil
		})
		if err != nil || !committed {
			t.Errorf("commitInterrupt = committed:%v err:%v, want committed", committed, err)
		}
	}()
	<-commitStarted

	cancelDone := make(chan struct{})
	go func() {
		live.requestCancel("user canceled")
		close(cancelDone)
	}()
	select {
	case <-cancelDone:
		t.Fatal("cancel crossed an in-flight interrupt commit")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseCommit)
	select {
	case <-commitDone:
	case <-time.After(time.Second):
		t.Fatal("interrupt commit did not finish")
	}
	select {
	case <-cancelDone:
	case <-time.After(time.Second):
		t.Fatal("cancel did not continue after interrupt commit")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("run context was not canceled")
	}
	if got := live.reason(); got != "user canceled" {
		t.Fatalf("cancel reason = %q", got)
	}

	called := false
	committed, err := live.commitInterrupt(func() error {
		called = true
		return nil
	})
	if err != nil || committed || called {
		t.Fatalf("post-cancel commit = committed:%v called:%v err:%v", committed, called, err)
	}
}

func TestCancelRunCleanupSurvivesRequestCancellation(t *testing.T) {
	rt := &cancelBindingRuntime{observed: make(chan error, 1)}
	s := newTestServer(rt)
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	ownerCtx, release, ok := s.tasks.Attach(requestCtx)
	if !ok {
		t.Fatal("owner attach rejected")
	}
	live := &runHandle{owner: ownerCtx, cancel: func() {}}
	s.runs.Open(runs.Record{ID: "run_1", SessionID: "ses_1", TurnID: "turn_1"}, live)
	cancelRequest()

	if err := s.CancelRun(requestCtx, protocol.CancelRunRequest{RunID: "run_1"}); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if err := <-rt.observed; err != nil {
		t.Fatalf("CancelRunBinding context = %v, want cleanup context alive", err)
	}
	release()
	s.Close()
}

type cancelBindingRuntime struct {
	stubRuntime
	observed chan error
}

func (r *cancelBindingRuntime) CancelRunBinding(ctx context.Context, _ lifecycle.RunTurnBinding) error {
	r.observed <- ctx.Err()
	return nil
}

func TestServerCloseCancelsAndJoinsRunPersistence(t *testing.T) {
	store := &blockingRunTranscript{started: make(chan struct{})}
	rt := &blockingPumpRuntime{}
	rt.effects = runsegment.New(runsegment.Config{Stores: blockingRunStores{transcript: store}})
	s := newTestServer(rt)
	handle := turn.TurnHandle{SessionID: "ses_1", TurnID: "run_1"}

	_, events, err := s.openSegment(context.Background(), handle.TurnID, "", handle, handle.SessionID, nil, nil, "", "")
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	select {
	case <-store.started:
	case <-time.After(time.Second):
		t.Fatal("run pump did not enter durable persistence")
	}

	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Server.Close did not cancel and join blocked persistence")
	}
	select {
	case _, ok := <-events:
		if ok {
			for range events {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("run event stream was not closed")
	}
}

func TestInterruptPersistenceFailureAbortsParkedTurn(t *testing.T) {
	rt := &blockingPumpRuntime{canceled: make(chan struct{})}
	rt.effects = runsegment.New(runsegment.Config{})
	rt.events = func(yield func(turn.Event) bool) {
		yield(turn.TurnInterrupted{Interrupts: []turn.Interrupt{{Kind: "question"}}})
	}
	s := newTestServer(rt)
	handle := turn.TurnHandle{SessionID: "ses_1", TurnID: "run_1"}

	_, events, err := s.openSegment(context.Background(), handle.TurnID, "", handle, handle.SessionID, nil, nil, "", "")
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	var outcome string
	for ev := range events {
		if ev.Event.Outcome != nil {
			outcome = string(ev.Event.Outcome.Type)
		}
	}
	if outcome != "error" {
		t.Fatalf("terminal outcome = %q, want error", outcome)
	}
	select {
	case <-rt.canceled:
	case <-time.After(time.Second):
		t.Fatal("parked turn was not canceled after interrupt persistence failed")
	}
	s.Close()
}

type blockingPumpRuntime struct {
	stubRuntime
	effects    *runsegment.Effects
	events     iter.Seq[turn.Event]
	canceled   chan struct{}
	cancelOnce sync.Once
}

func (*blockingPumpRuntime) SessionByID(context.Context, string) (session.Session, error) {
	return session.Session{ID: "ses_1", Cwd: "/work"}, nil
}

func (r *blockingPumpRuntime) TurnEvents(ctx context.Context, _ turn.TurnHandle) (iter.Seq[turn.Event], error) {
	if r.events != nil {
		return r.events, nil
	}
	return func(func(turn.Event) bool) { <-ctx.Done() }, nil
}

func (r *blockingPumpRuntime) CancelTurn(context.Context, turn.TurnHandle) error {
	if r.canceled != nil {
		r.cancelOnce.Do(func() { close(r.canceled) })
	}
	return nil
}

func (r *blockingPumpRuntime) RunSegmentEffects(runsegment.Checkpoints, runsegment.FileChangePublisher) *runsegment.Effects {
	return r.effects
}

type blockingRunStores struct {
	transcript *blockingRunTranscript
}

func (blockingRunStores) Interrupts() runsegment.InterruptStore { return nil }
func (blockingRunStores) Session() runsegment.SessionStore      { return nil }
func (s blockingRunStores) Transcript() runsegment.TranscriptStore {
	return s.transcript
}
func (blockingRunStores) MessageCount(context.Context, string) (int, error) { return 0, nil }
func (blockingRunStores) GenerateTitle(context.Context, string) (string, error) {
	return "", nil
}

type blockingRunTranscript struct {
	once    sync.Once
	started chan struct{}
}

func (s *blockingRunTranscript) AppendItem(ctx context.Context, _ transcript.Item) error {
	return ctx.Err()
}

func (s *blockingRunTranscript) PutRun(ctx context.Context, _ transcript.Run) error {
	s.once.Do(func() { close(s.started) })
	<-ctx.Done()
	return ctx.Err()
}
