package chat_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// stubEngine satisfies chat.Engine without touching the real
// platform / chat-memory / MCP wiring. Existence proves the chat
// service does not depend on *engine.Engine directly — only on
// the narrow interface.
type stubEngine struct {
	runChatCalls atomic.Int32
	runReply     string
}

func (s *stubEngine) RunChat(ctx context.Context, req engine.RunChatRequest) (engine.ChatOutput, error) {
	s.runChatCalls.Add(1)
	if req.Observer != nil {
		req.Observer.OnMessageDelta(s.runReply)
	}
	return engine.ChatOutput{Reply: s.runReply}, nil
}

func (s *stubEngine) GeneratePlan(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (s *stubEngine) InjectUserMessage(_ context.Context, _, _ string) error { return nil }

func (s *stubEngine) MaybeCompact(_ context.Context, _ string) (bool, error) { return false, nil }

func (s *stubEngine) MaybeExtract(_ context.Context, _ string) error { return nil }

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
// without ever returning normally.
type slowStubEngine struct{ stubEngine }

func (s *slowStubEngine) RunChat(ctx context.Context, _ engine.RunChatRequest) (engine.ChatOutput, error) {
	<-ctx.Done()
	return engine.ChatOutput{}, errors.New("canceled")
}
