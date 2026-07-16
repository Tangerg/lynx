package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

func newPromptContext(t *testing.T, model chat.Model) *core.ProcessContext {
	t.Helper()
	return core.NewProcessContext(core.ProcessContextConfig{
		Chat: func() (core.ChatCapability, error) {
			streamer, _ := model.(chat.Streamer)
			return core.ChatCapability{Model: model, Streamer: streamer}, nil
		},
		RunInteraction: func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			response, err := input.Model.Call(ctx, input.Request)
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
	if !strings.Contains(err.Error(), "chat model") {
		t.Fatalf("error %q should mention chat model", err.Error())
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

func TestPromptJSONDecodesResponse(t *testing.T) {
	type Brief struct {
		Summary string   `json:"summary"`
		Sources []string `json:"sources"`
	}

	model := newStubModel(`{"summary":"hi","sources":["a","b"]}`)
	pc := newPromptContext(t, model)

	brief, err := core.PromptJSON[Brief](t.Context(), pc, "brief me", core.PromptConfig{})
	if err != nil {
		t.Fatalf("PromptJSON: %v", err)
	}
	if brief.Summary != "hi" {
		t.Fatalf("brief.Summary = %q, want hi", brief.Summary)
	}
	if len(brief.Sources) != 2 || brief.Sources[0] != "a" {
		t.Fatalf("brief.Sources = %v, want [a b]", brief.Sources)
	}

	if !strings.Contains(model.gotPrompt, "brief me") {
		t.Fatalf("user text missing from model prompt: %q", model.gotPrompt)
	}
	if !strings.Contains(model.gotPrompt, "JSON SCHEMA") {
		t.Fatalf("JSON schema instructions missing from model prompt: %q", model.gotPrompt)
	}
}

func TestPromptJSONRejectsNilContext(t *testing.T) {
	type X struct{}
	_, err := core.PromptJSON[X](t.Context(), nil, "anything", core.PromptConfig{})
	if err == nil {
		t.Fatal("expected error on nil runner")
	}
}

func TestPromptAcceptsTools(t *testing.T) {
	model := newStubModel("ok")
	pc := newPromptContext(t, model)

	tool, err := tools.New[struct{}, string](
		tools.Config{Name: "stub_tool"},
		func(context.Context, struct{}) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	got, err := pc.Prompt(t.Context(), "hi", core.PromptConfig{Tools: []tools.Tool{tool}})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Generate = %q, want ok", got)
	}
}
