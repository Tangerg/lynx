package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

func TestBuildChatGuardrailsUsesTargetHistoryMiddleware(t *testing.T) {
	guardrails, err := NewChatGuardrails(ChatGuardrailsConfig{MaxToolRounds: 11})
	if err != nil {
		t.Fatal(err)
	}
	if len(guardrails.CallMiddlewares) != 1 || len(guardrails.StreamMiddlewares) != 1 {
		t.Fatalf("middleware counts = %d, %d; want 1, 1", len(guardrails.CallMiddlewares), len(guardrails.StreamMiddlewares))
	}
	if guardrails.MaxToolRounds != 11 {
		t.Fatalf("MaxToolRounds = %d, want 11", guardrails.MaxToolRounds)
	}
}

func TestBuildChatGuardrailsRejectsNegativeRounds(t *testing.T) {
	if _, err := NewChatGuardrails(ChatGuardrailsConfig{MaxToolRounds: -1}); err == nil {
		t.Fatal("negative MaxToolRounds must fail")
	}
}

func TestProcessChatBindsSessionToTargetHistoryMiddleware(t *testing.T) {
	store := chathistory.NewInMemoryStore()
	guardrails, err := NewChatGuardrails(ChatGuardrailsConfig{HistoryStore: store})
	if err != nil {
		t.Fatal(err)
	}
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		message := chat.NewAssistantMessage(chat.NewTextPart("answer"))
		return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	})
	client, err := chatclient.New(model)
	if err != nil {
		t.Fatal(err)
	}
	process := &Process{
		id:      "session-1",
		options: &processOptions{guardrails: guardrails},
	}
	scoped, err := process.scopeChat(core.ChatCapability{Model: client, Streamer: client})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if _, err := scoped.Model.Call(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	messages, err := store.Read(t.Context(), "session-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Text() != "question" || messages[1].Text() != "answer" {
		t.Fatalf("stored messages = %#v", messages)
	}
}
