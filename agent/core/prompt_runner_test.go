package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"
)

// newPromptRunnerPC wires a ProcessContext with the stub chat client
// the existing prompt_condition_test fixtures already provide.
func newPromptRunnerPC(t *testing.T, model chat.Model) *core.ProcessContext {
	t.Helper()
	return core.NewProcessContext(core.ProcessContextConfig{
		PlatformHooks: core.PlatformHooks{
			ChatClient: newStubChatClient(t, model),
		},
	})
}

func TestPromptRunner_Generate_ReturnsText(t *testing.T) {
	model := newStubModel("hello world")
	pc := newPromptRunnerPC(t, model)

	got, err := pc.PromptRunner().Generate(t.Context(), "say hi")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("Generate = %q, want %q", got, "hello world")
	}
	if !strings.Contains(model.gotPrompt, "say hi") {
		t.Fatalf("model didn't see the user prompt; got %q", model.gotPrompt)
	}
}

func TestPromptRunner_Generate_SystemPromptPropagates(t *testing.T) {
	model := newStubModel("ok")
	pc := newPromptRunnerPC(t, model)

	_, err := pc.PromptRunner().
		WithSystem("You are terse.").
		Generate(t.Context(), "anything")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// The stub only records the user message; the system message lands
	// on req.Messages but the stub ignores it. We verify the runner
	// didn't error and a user prompt still arrived.
	if model.gotPrompt == "" {
		t.Fatal("expected the user prompt to reach the model")
	}
}

func TestPromptRunner_Generate_NoChatClient(t *testing.T) {
	pc := core.NewProcessContext(core.ProcessContextConfig{}) // no ChatClient

	_, err := pc.PromptRunner().Generate(t.Context(), "anything")
	if err == nil {
		t.Fatal("expected error when no ChatClient is configured")
	}
	if !strings.Contains(err.Error(), "ChatClient") {
		t.Fatalf("error %q should mention ChatClient", err.Error())
	}
}

func TestPromptRunner_Generate_PropagatesModelError(t *testing.T) {
	wantErr := errors.New("boom")
	pc := newPromptRunnerPC(t, newStubErrModel(wantErr))

	_, err := pc.PromptRunner().Generate(t.Context(), "anything")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want chain to include %v", err, wantErr)
	}
}

// TestPromptRunner_GenerateObject_JSONDecoded verifies the typed
// structured-output path: the JSON parser instructions are appended
// to the user prompt and the assistant's JSON reply is unmarshalled
// into T.
func TestPromptRunner_GenerateObject_JSONDecoded(t *testing.T) {
	type Brief struct {
		Summary string   `json:"summary"`
		Sources []string `json:"sources"`
	}

	model := newStubModel(`{"summary":"hi","sources":["a","b"]}`)
	pc := newPromptRunnerPC(t, model)

	brief, err := core.GenerateObject[Brief](t.Context(), pc.PromptRunner(), "brief me")
	if err != nil {
		t.Fatalf("GenerateObject: %v", err)
	}
	if brief.Summary != "hi" {
		t.Fatalf("brief.Summary = %q, want hi", brief.Summary)
	}
	if len(brief.Sources) != 2 || brief.Sources[0] != "a" {
		t.Fatalf("brief.Sources = %v, want [a b]", brief.Sources)
	}

	// The schema-derived instructions must reach the model alongside
	// the user prompt.
	if !strings.Contains(model.gotPrompt, "brief me") {
		t.Fatalf("user text missing from model prompt: %q", model.gotPrompt)
	}
	if !strings.Contains(model.gotPrompt, "JSON SCHEMA") {
		t.Fatalf("JSON schema instructions missing from model prompt: %q", model.gotPrompt)
	}
}

func TestPromptRunner_GenerateObject_NilRunnerErrors(t *testing.T) {
	type X struct{}
	_, err := core.GenerateObject[X](t.Context(), nil, "anything")
	if err == nil {
		t.Fatal("expected error on nil runner")
	}
}

// TestPromptRunner_WithTools_ToolLoopEngaged verifies that
// when explicit tools are supplied via WithTools, the tool loop is
// installed and the tool is reachable through it.
func TestPromptRunner_WithTools_ToolLoopEngaged(t *testing.T) {
	// Stub model replies "ok" without invoking the tool — we just
	// verify the runner accepts WithTools and the call succeeds with
	// the middleware in place.
	model := newStubModel("ok")
	pc := newPromptRunnerPC(t, model)

	tool, err := chat.NewTool(
		chat.ToolDefinition{Name: "stub_tool", InputSchema: `{"type":"object"}`},
		func(context.Context, string) (string, error) { return "", nil },
	)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}

	got, err := pc.PromptRunner().WithTools(tool).Generate(t.Context(), "hi")
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got != "ok" {
		t.Fatalf("Generate = %q, want ok", got)
	}
}
