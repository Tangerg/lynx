package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

func newPromptContext(t *testing.T, model chat.Model) *core.ProcessContext {
	t.Helper()
	return core.NewProcessContext(core.ProcessContextConfig{
		RunInteraction: func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			response, err := model.Call(ctx, input.Request)
			if err != nil {
				return interaction.Result{}, err
			}
			final := interaction.Event{Kind: interaction.EventModelResponse, Round: 1, Final: true, Response: response}
			return interaction.Result{Final: &final}, nil
		},
	})
}

func TestPromptReturnsText(t *testing.T) {
	model := newStubModel("hello world")
	pc := newPromptContext(t, model)

	got, err := pc.Prompt(t.Context(), "say hi", core.PromptConfig{})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("Generate = %q, want %q", got, "hello world")
	}
	if !strings.Contains(model.gotPrompt, "say hi") {
		t.Fatalf("model didn't see the user prompt; got %q", model.gotPrompt)
	}
}

func TestPromptAcceptsSystemMessage(t *testing.T) {
	model := newStubModel("ok")
	pc := newPromptContext(t, model)

	_, err := pc.Prompt(t.Context(), "anything", core.PromptConfig{System: "You are terse."})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if model.gotPrompt == "" {
		t.Fatal("expected the user prompt to reach the model")
	}
}

func TestPromptRejectsMissingChatModel(t *testing.T) {
	pc := core.NewProcessContext(core.ProcessContextConfig{})

	_, err := pc.Prompt(t.Context(), "anything", core.PromptConfig{})
	if err == nil {
		t.Fatal("expected error when no chat model is configured")
	}
	if !strings.Contains(err.Error(), "managed interaction") {
		t.Fatalf("error %q should mention managed interaction", err.Error())
	}
}

func TestPromptReturnsModelError(t *testing.T) {
	wantErr := errors.New("boom")
	pc := newPromptContext(t, newStubErrModel(wantErr))

	_, err := pc.Prompt(t.Context(), "anything", core.PromptConfig{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want chain to include %v", err, wantErr)
	}
}

func TestPromptAcceptsTools(t *testing.T) {
	model := newStubModel("ok")
	pc := newPromptContext(t, model)

	got, err := pc.Prompt(t.Context(), "hi", core.PromptConfig{
		Tools: []tool.Tool{promptTool{name: "stub_tool"}},
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Generate = %q, want ok", got)
	}
}
