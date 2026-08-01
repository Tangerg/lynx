package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func TestPromptDecodesManagedInteraction(t *testing.T) {
	type answer struct {
		Value string `json:"value"`
	}

	calls := 0
	process := core.NewProcessContext(core.ProcessContextConfig{
		RunInteraction: func(_ context.Context, input core.Interaction) (interaction.Result, error) {
			calls++
			prompt := input.Request.Messages[len(input.Request.Messages)-1].Text()
			if !strings.HasPrefix(prompt, "answer the question\n\n") {
				t.Fatalf("prompt = %q, want caller text followed by output instructions", prompt)
			}
			if !strings.Contains(prompt, "RFC 8259-compliant JSON") {
				t.Fatalf("prompt = %q, want JSON output instructions", prompt)
			}
			return finalTextResult(t, `{"value":"done"}`), nil
		},
	})

	got, err := agent.Prompt(t.Context(), process, "answer the question", agent.PromptConfig{}, chatclient.JSON[answer]())
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if calls != 1 {
		t.Fatalf("managed interactions = %d, want 1", calls)
	}
	if got.Value != "done" {
		t.Fatalf("Prompt = %+v, want decoded answer", got)
	}
}

func TestPromptValidatesOutputBeforeInteraction(t *testing.T) {
	called := false
	process := core.NewProcessContext(core.ProcessContextConfig{
		RunInteraction: func(context.Context, core.Interaction) (interaction.Result, error) {
			called = true
			return interaction.Result{}, nil
		},
	})

	_, err := agent.Prompt(t.Context(), process, "answer", agent.PromptConfig{}, chatclient.Output[string]{})
	if !errors.Is(err, chatclient.ErrInvalidOutput) {
		t.Fatalf("Prompt error = %v, want ErrInvalidOutput", err)
	}
	if called {
		t.Fatal("Prompt started an interaction for an invalid output")
	}
}

func TestPromptRejectsNilProcessContext(t *testing.T) {
	_, err := agent.Prompt(t.Context(), nil, "answer", agent.PromptConfig{}, chatclient.JSON[string]())
	if err == nil || !strings.Contains(err.Error(), "process context is nil") {
		t.Fatalf("Prompt error = %v, want nil process context diagnosis", err)
	}
}

func TestPromptPreservesDecodeErrorIdentity(t *testing.T) {
	process := core.NewProcessContext(core.ProcessContextConfig{
		RunInteraction: func(context.Context, core.Interaction) (interaction.Result, error) {
			return finalTextResult(t, "not JSON"), nil
		},
	})

	_, err := agent.Prompt(t.Context(), process, "answer", agent.PromptConfig{}, chatclient.JSON[map[string]string]())
	var syntaxError *json.SyntaxError
	if !errors.As(err, &syntaxError) {
		t.Fatalf("Prompt error = %v, want json.SyntaxError in chain", err)
	}
}

func TestPromptLeavesTextUnchangedForDecodeOnlyOutput(t *testing.T) {
	const original = "answer exactly"
	process := core.NewProcessContext(core.ProcessContextConfig{
		RunInteraction: func(_ context.Context, input core.Interaction) (interaction.Result, error) {
			if got := input.Request.Messages[len(input.Request.Messages)-1].Text(); got != original {
				t.Fatalf("prompt = %q, want %q", got, original)
			}
			return finalTextResult(t, "DONE"), nil
		},
	})
	output := chatclient.Output[string]{
		Decode: func(raw string) (string, error) { return strings.ToLower(raw), nil },
	}

	got, err := agent.Prompt(t.Context(), process, original, agent.PromptConfig{}, output)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "done" {
		t.Fatalf("Prompt = %q, want done", got)
	}
}

func finalTextResult(t *testing.T, text string) interaction.Result {
	t.Helper()
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	response, err := chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	})
	if err != nil {
		t.Fatalf("NewResponse: %v", err)
	}
	final := interaction.Event{
		Kind:     interaction.EventModelResponse,
		Round:    1,
		Final:    true,
		Response: response,
	}
	return interaction.Result{Final: &final}
}
