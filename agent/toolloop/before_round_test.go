package toolloop

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

// TestToolMiddleware_BeforeRoundInjects proves the BeforeRound hook appends its
// messages to the CONTINUATION round (after the tool result), never before the
// first round — the seam for mid-run steering.
func TestToolMiddleware_BeforeRoundInjects(t *testing.T) {
	model := newFakeChatModel(t)
	var round2 []chat.Message
	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		if calls == 1 {
			return responseWithToolCall(t, "echo", `{"x":1}`), nil
		}
		round2 = req.Messages
		return responseWithText("done"), nil // final answer ends the loop
	}
	echoTool := mustNewCallable(t, "echo", false, func(context.Context, string) (string, error) {
		return "result", nil
	})

	injected := 0
	callMW, _ := NewMiddleware(Config{
		MaxIterations: 5,
		BeforeRound: func(context.Context) []chat.Message {
			injected++
			return []chat.Message{chat.NewUserMessage("STEER")}
		},
	})
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed")).WithTools(echoTool)
	if _, err := req.Call().Response(context.Background()); err != nil {
		t.Fatalf("call: %v", err)
	}

	// Fires once — before the single continuation round, not before round 1.
	if injected != 1 {
		t.Fatalf("BeforeRound called %d times, want 1 (continuation only)", injected)
	}
	// The steer is the LAST message of the continuation request — after the
	// assistant(tool_call) + tool result, so the model reads it as a fresh turn.
	if len(round2) == 0 {
		t.Fatal("no continuation-round messages captured")
	}
	last, ok := round2[len(round2)-1].(*chat.UserMessage)
	if !ok || last.Text != "STEER" {
		t.Fatalf("last continuation message = %#v, want UserMessage(STEER) after the tool result", round2[len(round2)-1])
	}
}

// TestToolMiddleware_BeforeRoundNilDefault confirms a nil hook (the default)
// leaves the loop untouched — no extra messages, no calls.
func TestToolMiddleware_BeforeRoundNilDefault(t *testing.T) {
	model := newFakeChatModel(t)
	calls := 0
	model.respond = func(req *chat.Request) (*chat.Response, error) {
		calls++
		if calls == 1 {
			return responseWithToolCall(t, "echo", `{}`), nil
		}
		// No injected user message in the continuation request.
		for _, m := range req.Messages {
			if u, ok := m.(*chat.UserMessage); ok && u.Text == "STEER" {
				t.Fatal("nil BeforeRound injected a message")
			}
		}
		return responseWithText("done"), nil
	}
	echoTool := mustNewCallable(t, "echo", false, func(context.Context, string) (string, error) {
		return "result", nil
	})
	callMW, _ := NewMiddleware(Config{MaxIterations: 5}) // BeforeRound unset
	req, _ := chat.NewClientRequest(model)
	req.WithMiddlewares(callMW).WithMessages(chat.NewUserMessage("seed")).WithTools(echoTool)
	if _, err := req.Call().Response(context.Background()); err != nil {
		t.Fatalf("call: %v", err)
	}
}
