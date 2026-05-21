package chat_test

import (
	"context"
	"iter"
	"strings"
	"testing"

	chatmodel "github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/lyra/internal/service/chat"
)

// TestService_StartTurn_EmitsExpectedEvents drives a full turn
// against a stub LLM that asks for `bash` (echo lyra). The service
// must emit the canonical sequence:
//
//	TurnStart → ToolCallStart → ToolCallEnd → MessageDelta → TurnEnd
//
// and the channel must close cleanly. This is the M1+M2 contract:
// transport adapters built later only need to forward whatever this
// channel yields.
func TestService_StartTurn_EmitsExpectedEvents(t *testing.T) {
	svc, _ := buildService(t)

	handle, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "sess-1",
		Message:   "say lyra via bash",
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	events, err := svc.Events(context.Background(), handle)
	if err != nil {
		t.Fatalf("Events: %v", err)
	}

	got := drainEvents(events)

	wantOrder := []string{"TurnStart", "ToolCallStart", "ToolCallEnd", "MessageDelta", "TurnEnd"}
	if names := eventNames(got); !sliceEqual(names, wantOrder) {
		t.Fatalf("event order mismatch:\n  got  %v\n  want %v", names, wantOrder)
	}

	// Spot-check each event's content.
	for _, ev := range got {
		switch e := ev.(type) {
		case chat.TurnStart:
			if e.SessionID != "sess-1" {
				t.Errorf("TurnStart.SessionID = %q, want sess-1", e.SessionID)
			}
			if e.TurnID == "" {
				t.Error("TurnStart.TurnID is empty")
			}
		case chat.ToolCallStart:
			if e.ToolName != "bash" {
				t.Errorf("ToolCallStart.ToolName = %q, want bash", e.ToolName)
			}
			if !strings.Contains(e.Arguments, "echo lyra") {
				t.Errorf("ToolCallStart.Arguments missing command: %q", e.Arguments)
			}
		case chat.ToolCallEnd:
			if e.Err != "" {
				t.Errorf("ToolCallEnd.Err = %q, want empty", e.Err)
			}
			if !strings.Contains(e.Output, "lyra") {
				t.Errorf("ToolCallEnd.Output missing 'lyra': %q", e.Output)
			}
		case chat.MessageDelta:
			if !strings.Contains(e.Text, "lyra") {
				t.Errorf("MessageDelta.Text missing 'lyra': %q", e.Text)
			}
		case chat.TurnEnd:
			if e.Reason != chat.TurnEndCompleted {
				t.Errorf("TurnEnd.Reason = %s, want completed", e.Reason)
			}
		}
	}

	// After the turn ends Events / Cancel should report ErrTurnNotFound —
	// the impl cleans up turnState on close.
	if _, err := svc.Events(context.Background(), handle); err == nil {
		t.Error("Events after TurnEnd should error")
	}
	if err := svc.Cancel(context.Background(), handle); err == nil {
		t.Error("Cancel after TurnEnd should error")
	}
}

// TestService_SeqMonotone verifies every event in a turn carries a
// strictly increasing Seq starting at 1 — transport adapters rely
// on monotonicity for resume-from-seq semantics.
func TestService_SeqMonotone(t *testing.T) {
	svc, _ := buildService(t)
	handle, _ := svc.StartTurn(context.Background(), chat.StartTurnRequest{
		SessionID: "s", Message: "hi",
	})
	events, _ := svc.Events(context.Background(), handle)
	got := drainEvents(events)

	var prev uint64
	for i, ev := range got {
		seq := baseSeq(ev)
		if seq != prev+1 {
			t.Errorf("event[%d] seq = %d, want %d (%T)", i, seq, prev+1, ev)
		}
		prev = seq
	}
}

// TestService_StartTurn_Validation rejects empty SessionID / Message.
func TestService_StartTurn_Validation(t *testing.T) {
	svc, _ := buildService(t)

	if _, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{Message: "x"}); err == nil {
		t.Error("missing SessionID should error")
	}
	if _, err := svc.StartTurn(context.Background(), chat.StartTurnRequest{SessionID: "s"}); err == nil {
		t.Error("missing Message should error")
	}
}

// ------------------------------------------------------------------
// Helpers
// ------------------------------------------------------------------

func buildService(t *testing.T) (chat.Service, *engine.Engine) {
	t.Helper()

	client, err := chatmodel.NewClient(newStubChatModel())
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng, err := engine.New(engine.Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	return chat.New(eng), eng
}

func drainEvents(events <-chan chat.Event) []chat.Event {
	var out []chat.Event
	for ev := range events {
		out = append(out, ev)
	}
	return out
}

func eventNames(events []chat.Event) []string {
	out := make([]string, len(events))
	for i, ev := range events {
		switch ev.(type) {
		case chat.TurnStart:
			out[i] = "TurnStart"
		case chat.MessageDelta:
			out[i] = "MessageDelta"
		case chat.ToolCallStart:
			out[i] = "ToolCallStart"
		case chat.ToolCallEnd:
			out[i] = "ToolCallEnd"
		case chat.TurnEnd:
			out[i] = "TurnEnd"
		case chat.ErrorEvent:
			out[i] = "ErrorEvent"
		default:
			out[i] = "?"
		}
	}
	return out
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func baseSeq(ev chat.Event) uint64 {
	switch e := ev.(type) {
	case chat.TurnStart:
		return e.Seq
	case chat.MessageDelta:
		return e.Seq
	case chat.ToolCallStart:
		return e.Seq
	case chat.ToolCallEnd:
		return e.Seq
	case chat.TurnEnd:
		return e.Seq
	case chat.ErrorEvent:
		return e.Seq
	}
	return 0
}

// ------------------------------------------------------------------
// Stub model (duplicated from engine package because that one's
// test-scope; this test lives in a different package).
// ------------------------------------------------------------------

type stubChatModel struct{ defaults *chatmodel.Options }

func newStubChatModel() *stubChatModel {
	opts, _ := chatmodel.NewOptions("stub-model")
	return &stubChatModel{defaults: opts}
}

func (m *stubChatModel) DefaultOptions() chatmodel.Options    { return *m.defaults }
func (m *stubChatModel) Metadata() chatmodel.ModelMetadata    { return chatmodel.ModelMetadata{Provider: "stub"} }

func (m *stubChatModel) Call(_ context.Context, req *chatmodel.Request) (*chatmodel.Response, error) {
	if hasToolMsg(req.Messages) {
		return makeText("I ran echo and got lyra.")
	}
	return makeToolCall("bash", `{"command":"echo lyra"}`)
}

func (m *stubChatModel) Stream(ctx context.Context, req *chatmodel.Request) iter.Seq2[*chatmodel.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chatmodel.Response, error) bool) { yield(resp, err) }
}

func hasToolMsg(messages []chatmodel.Message) bool {
	for _, msg := range messages {
		if msg.Type() == chatmodel.MessageTypeTool {
			return true
		}
	}
	return false
}

func makeText(text string) (*chatmodel.Response, error) {
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(text),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonStop},
		},
		&chatmodel.ResponseMetadata{},
	)
}

func makeToolCall(name, args string) (*chatmodel.Response, error) {
	calls := []*chatmodel.ToolCallPart{{ID: "c1", Name: name, Arguments: args}}
	return chatmodel.NewResponse(
		&chatmodel.Result{
			AssistantMessage: chatmodel.NewAssistantMessage(calls),
			Metadata:         &chatmodel.ResultMetadata{FinishReason: chatmodel.FinishReasonToolCalls},
		},
		&chatmodel.ResponseMetadata{},
	)
}
