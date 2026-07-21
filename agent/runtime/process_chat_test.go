package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"
)

type conversationContextKey struct{}

func TestProcessChatProjectsConversationThroughHostBinder(t *testing.T) {
	const conversationID = "conversation-1"
	guardrails := &core.ChatGuardrails{
		BindConversation: func(ctx context.Context, id string) context.Context {
			return context.WithValue(ctx, conversationContextKey{}, id)
		},
	}
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		if got, _ := ctx.Value(conversationContextKey{}).(string); got != conversationID {
			return nil, errors.New("model call did not receive the conversation identity")
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("answer"))
		return chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	})
	process := &Process{
		id:      conversationID,
		options: &processOptions{guardrails: guardrails},
	}

	scoped, err := process.scopeChat(core.ChatCapability{Model: model})
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Model.Call(t.Context(), request); err != nil {
		t.Fatal(err)
	}
}

func TestProcessChatRejectsNilBoundContext(t *testing.T) {
	guardrails := &core.ChatGuardrails{
		BindConversation: func(context.Context, string) context.Context { return nil },
	}
	process := &Process{id: "conversation-1", options: &processOptions{guardrails: guardrails}}
	scoped, err := process.scopeChat(core.ChatCapability{Model: chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scoped.Model.Call(t.Context(), request); err == nil {
		t.Fatal("nil bound context was accepted")
	}
}
