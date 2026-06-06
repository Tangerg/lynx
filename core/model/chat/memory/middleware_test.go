package memory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

// recordingHandler captures the messages of the last request it received
// (what the model would actually see) and returns a fixed assistant reply.
type recordingHandler struct {
	seen []chat.Message
	text string
}

func (h *recordingHandler) Call(_ context.Context, req *chat.Request) (*chat.Response, error) {
	h.seen = req.Messages
	res, err := chat.NewResult(chat.NewAssistantMessage(h.text), &chat.ResultMetadata{FinishReason: chat.FinishReasonStop})
	if err != nil {
		return nil, err
	}
	return chat.NewResponse(res, &chat.ResponseMetadata{})
}

func messageTypes(msgs []chat.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		parts = append(parts, string(m.Type()))
	}
	return strings.Join(parts, " → ")
}

// TestMemoryMiddleware_SystemFirstAndNeverPersisted locks the two memory
// invariants across two turns of the same conversation:
//   - the system message is regenerated each turn, never persisted, and
//     always the first message the model sees;
//   - stored history is loaded and spliced in front of the turn's new
//     non-system messages (load → splice → save), with no de-duplication
//     state involved.
func TestMemoryMiddleware_SystemFirstAndNeverPersisted(t *testing.T) {
	store := memory.NewInMemoryStore()
	callMW, _, err := memory.NewMiddleware(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	turn := func(system, user, reply string) []chat.Message {
		h := &recordingHandler{text: reply}
		handler := callMW(chat.CallHandlerFunc(h.Call))
		req, err := chat.NewRequest([]chat.Message{chat.NewSystemMessage(system), chat.NewUserMessage(user)})
		if err != nil {
			t.Fatal(err)
		}
		req.Set(memory.ConversationIDKey, "c1")
		if _, err := handler.Call(ctx, req); err != nil {
			t.Fatal(err)
		}
		return h.seen
	}

	// Turn 1: fresh conversation. The model sees [system, user]; history empty.
	seen1 := turn("sys-A", "hi", "a1")
	if len(seen1) == 0 || seen1[0].Type() != chat.MessageTypeSystem {
		t.Fatalf("turn1: model saw %s, want system first", messageTypes(seen1))
	}

	// Turn 2: a DIFFERENT system prompt. The model must see the FRESH system
	// (sys-B) first, exactly once, followed by the spliced history — which
	// carries NO system message.
	seen2 := turn("sys-B", "again", "a2")
	if len(seen2) == 0 || seen2[0].Type() != chat.MessageTypeSystem {
		t.Fatalf("turn2: model saw %s, want system first", messageTypes(seen2))
	}
	sysCount := 0
	for _, m := range seen2 {
		if m.Type() == chat.MessageTypeSystem {
			sysCount++
		}
	}
	if sysCount != 1 {
		t.Fatalf("turn2: %d system messages reached the model, want exactly 1 (the fresh one): %s", sysCount, messageTypes(seen2))
	}
	// spliced order: system(sys-B) → user(hi) → assistant(a1) → user(again)
	if want := "system → user → assistant → user"; messageTypes(seen2) != want {
		t.Fatalf("turn2 model view = %q, want %q", messageTypes(seen2), want)
	}

	// The store must never hold a system message, and must accumulate exactly
	// the non-system turn messages + replies in order.
	stored, _ := store.Read(ctx, "c1")
	for _, m := range stored {
		if m.Type() == chat.MessageTypeSystem {
			t.Fatalf("system message was persisted: %s", messageTypes(stored))
		}
	}
	if want := "user → assistant → user → assistant"; messageTypes(stored) != want {
		t.Fatalf("stored = %q, want %q", messageTypes(stored), want)
	}
}

// TestMemoryMiddleware_NoConversationIDPassesThrough verifies the middleware
// is a no-op (no load, no save) when no conversation id is set.
func TestMemoryMiddleware_NoConversationIDPassesThrough(t *testing.T) {
	store := memory.NewInMemoryStore()
	callMW, _, err := memory.NewMiddleware(store)
	if err != nil {
		t.Fatal(err)
	}

	h := &recordingHandler{text: "ok"}
	handler := callMW(chat.CallHandlerFunc(h.Call))
	req, err := chat.NewRequest([]chat.Message{chat.NewUserMessage("hi")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Call(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	ids, _ := store.Conversations(context.Background())
	if len(ids) != 0 {
		t.Fatalf("store wrote %v without a conversation id, want nothing", ids)
	}
}

// toolCallHandler returns an assistant message carrying a single tool call.
type toolCallHandler struct{ id, name string }

func (h toolCallHandler) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	am := chat.NewAssistantMessage([]*chat.ToolCallPart{{ID: h.id, Name: h.name, Arguments: "{}"}})
	res, err := chat.NewResult(am, &chat.ResultMetadata{FinishReason: chat.FinishReasonStop})
	if err != nil {
		return nil, err
	}
	return chat.NewResponse(res, &chat.ResponseMetadata{})
}

// TestMemoryMiddleware_SkipsUnpairedToolCallAssistant pins the fix for the
// dangling-tool_call bug: a model reply that REQUESTS tools is not persisted on
// its own (it has no answering tool message yet), so an interrupt or abort
// mid-round can never strand an unanswered assistant(tool_calls) in the store.
// The (assistant, tool) pair is written only when the tool-calling middleware
// re-presents them together as the next round's input.
func TestMemoryMiddleware_SkipsUnpairedToolCallAssistant(t *testing.T) {
	store := memory.NewInMemoryStore()
	callMW, _, err := memory.NewMiddleware(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Round 1: model asks for a tool. Memory must persist the user input but
	// NOT the unpaired tool-call assistant.
	h1 := callMW(chat.CallHandlerFunc(toolCallHandler{id: "call_1", name: "x"}.Call))
	req1, _ := chat.NewRequest([]chat.Message{chat.NewUserMessage("do it")})
	req1.Set(memory.ConversationIDKey, "c1")
	if _, err := h1.Call(ctx, req1); err != nil {
		t.Fatal(err)
	}
	stored, _ := store.Read(ctx, "c1")
	if got := messageTypes(stored); got != "user" {
		t.Fatalf("after a tool-call round, stored = %q, want just %q (no stranded assistant)", got, "user")
	}

	// Round 2: the tool middleware re-presents [assistant(tool_call), tool] as
	// input; the model now gives a final answer. The pair lands atomically.
	assistant := chat.NewAssistantMessage([]*chat.ToolCallPart{{ID: "call_1", Name: "x", Arguments: "{}"}})
	toolMsg, _ := chat.NewToolMessage([]*chat.ToolReturn{{ID: "call_1", Name: "x", Result: "ok"}})
	h2 := callMW(chat.CallHandlerFunc((&recordingHandler{text: "done"}).Call))
	req2, _ := chat.NewRequest([]chat.Message{assistant, toolMsg})
	req2.Set(memory.ConversationIDKey, "c1")
	if _, err := h2.Call(ctx, req2); err != nil {
		t.Fatal(err)
	}
	stored, _ = store.Read(ctx, "c1")
	if want := "user → assistant → tool → assistant"; messageTypes(stored) != want {
		t.Fatalf("stored = %q, want %q (the exchange paired atomically)", messageTypes(stored), want)
	}
}
