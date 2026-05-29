package chat_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// stubChatProcess fakes the [engine.ChatProcess] handle without
// touching the real platform. The done channel is pre-fired so
// runTurn receives immediately; status / output / cancel return
// the values the test wired.
type stubChatProcess struct {
	id       string
	status   atomic.Int32 // core.AgentProcessStatus
	failure  error
	output   engine.ChatOutput
	done     chan error
	onCancel func(reason string)
}

func newStubChatProcess(id string, output engine.ChatOutput) *stubChatProcess {
	cp := &stubChatProcess{
		id:     id,
		output: output,
		done:   make(chan error, 1),
	}
	cp.status.Store(int32(core.StatusCompleted))
	cp.done <- nil
	close(cp.done)
	return cp
}

func (cp *stubChatProcess) ID() string { return cp.id }
func (cp *stubChatProcess) Status() core.AgentProcessStatus {
	return core.AgentProcessStatus(cp.status.Load())
}
func (cp *stubChatProcess) Failure() error     { return cp.failure }
func (cp *stubChatProcess) Done() <-chan error { return cp.done }
func (cp *stubChatProcess) Output() (engine.ChatOutput, error) {
	return cp.output, nil
}
func (cp *stubChatProcess) Cancel(reason string) error {
	cp.status.Store(int32(core.StatusKilled))
	if cp.onCancel != nil {
		cp.onCancel(reason)
	}
	return nil
}

// stubEngine satisfies chat.Engine without touching the real
// platform / chat-memory / MCP wiring. Existence proves the chat
// service does not depend on *engine.Engine directly — only on
// the narrow interface.
type stubEngine struct {
	runChatCalls atomic.Int32
	runReply     string
	stopOnBudget bool // when true the produced ChatOutput sets StoppedOnBudget
}

func (s *stubEngine) StartChat(_ context.Context, req engine.RunChatRequest) engine.ChatProcess {
	s.runChatCalls.Add(1)
	if req.Observer != nil {
		req.Observer.OnMessageDelta(s.runReply)
	}
	return newStubChatProcess("stub-proc-"+req.SessionID, engine.ChatOutput{
		Reply:           s.runReply,
		StoppedOnBudget: s.stopOnBudget,
	})
}

func (s *stubEngine) GeneratePlan(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (s *stubEngine) InjectUserMessage(_ context.Context, _, _ string) error { return nil }

func (s *stubEngine) MaybeCompact(_ context.Context, _ string) (engine.CompactionResult, error) {
	return engine.CompactionResult{}, nil
}

func (s *stubEngine) MaybeExtract(_ context.Context, _ string) (engine.ExtractionResult, error) {
	return engine.ExtractionResult{}, nil
}

// TestStubEngineDrivesTurn — confirms the chat service runs a full
// turn against a stub engine, no real platform involved. If chat
// ever regrows a hard *engine.Engine dependency, this test stops
// compiling.
func TestStubEngineDrivesTurn(t *testing.T) {
	stub := &stubEngine{runReply: "hello from stub"}

	svc := chat.New(stub, nil)
	handle, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-1",
		Message:   "hi",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	events, err := svc.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	var sawDelta, sawEnd bool
	deadline := time.After(2 * time.Second)
	for !sawEnd {
		select {
		case ev, ok := <-events:
			if !ok {
				sawEnd = true
				break
			}
			switch ev.(type) {
			case chat.MessageDelta:
				sawDelta = true
			case chat.TurnEnd:
				sawEnd = true
			}
		case <-deadline:
			t.Fatalf("timed out; sawDelta=%v sawEnd=%v", sawDelta, sawEnd)
		}
	}

	if !sawDelta {
		t.Errorf("expected at least one MessageDelta event")
	}
	if got := stub.runChatCalls.Load(); got != 1 {
		t.Errorf("RunChat called %d times, want 1", got)
	}
}

// TestStubEngineBudgetStop — a turn whose process reports
// StoppedOnBudget ends with Reason=TurnEndBudgetExceeded, not a plain
// completion, so clients can tell "stopped at the ceiling" apart from
// "model finished".
func TestStubEngineBudgetStop(t *testing.T) {
	stub := &stubEngine{runReply: "partial answer", stopOnBudget: true}
	svc := chat.New(stub, nil)

	handle, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "s",
		Message:   "go",
		MaxBudget: 1,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	events, _ := svc.Events(context.Background(), handle)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				t.Fatal("channel closed without a TurnEnd")
			}
			if end, ok := ev.(chat.TurnEnd); ok {
				if end.Reason != chat.TurnEndBudgetExceeded {
					t.Fatalf("TurnEnd reason = %v, want budget_exceeded", end.Reason)
				}
				return
			}
		case <-deadline:
			t.Fatal("no TurnEnd within 2s")
		}
	}
}

// TestStubEngineCancelsCleanly — confirms Cancel propagates to the
// turn without needing a real engine.
func TestStubEngineCancelsCleanly(t *testing.T) {
	stub := &slowStubEngine{}
	svc := chat.New(stub, nil)

	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "s",
		Message:   "m",
	})
	if err := svc.Cancel(context.Background(), handle); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	events, _ := svc.Events(context.Background(), handle)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return // channel closed = turn done
			}
			if end, ok := ev.(chat.TurnEnd); ok && end.Reason == chat.TurnEndCancelled {
				return
			}
		case <-deadline:
			t.Fatalf("turn did not cancel within 2s")
		}
	}
}

// slowStubEngine simulates an engine that respects ctx cancellation
// without ever returning normally — the stub ChatProcess holds a
// done channel that fires only when ctx is canceled, mirroring how
// the real platform reacts to KillProcess / ctx cancel.
type slowStubEngine struct{ stubEngine }

func (s *slowStubEngine) StartChat(ctx context.Context, _ engine.RunChatRequest) engine.ChatProcess {
	cp := &stubChatProcess{
		id:   "slow-stub-proc",
		done: make(chan error, 1),
	}
	cp.status.Store(int32(core.StatusRunning))
	cp.onCancel = func(_ string) {
		select {
		case cp.done <- errors.New("canceled"):
		default:
		}
	}
	go func() {
		<-ctx.Done()
		select {
		case cp.done <- errors.New("canceled"):
		default:
		}
	}()
	return cp
}
