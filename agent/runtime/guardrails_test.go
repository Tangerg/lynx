package runtime

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestBuildChatGuardrailsUsesDefaultsAndAssemblesChain(t *testing.T) {
	t.Helper()

	guardrails, err := BuildChatGuardrails(ChatGuardrailsConfig{
		ToolLoop: ToolLoopPolicy{},
	})
	if err != nil {
		t.Fatalf("BuildChatGuardrails: unexpected error: %v", err)
	}
	if guardrails == nil {
		t.Fatal("BuildChatGuardrails: got nil guardrails")
	}

	if len(guardrails.CallMiddlewares) != 2 {
		t.Fatalf("BuildChatGuardrails: expected 2 call middlewares, got %d", len(guardrails.CallMiddlewares))
	}
	if len(guardrails.StreamMiddlewares) != 2 {
		t.Fatalf("BuildChatGuardrails: expected 2 stream middlewares, got %d", len(guardrails.StreamMiddlewares))
	}
}

func TestBuildToolLoopReturnsPair(t *testing.T) {
	callMW, streamMW := BuildToolLoop(ToolLoopPolicy{
		MaxIterations:           11,
		FeedbackOnEmptyResponse: true,
		BeforeRound:             func(_ context.Context) []chat.Message { return nil },
	})

	if callMW == nil {
		t.Fatal("BuildToolLoop: expected non-nil call middleware")
	}
	if streamMW == nil {
		t.Fatal("BuildToolLoop: expected non-nil stream middleware")
	}

	_ = callMW
	_ = streamMW
}
