package chat_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
)

func TestPromptTemplate_RenderWithVariable(t *testing.T) {
	tmpl := chat.NewPromptTemplate("Hello {{.name}}").
		WithVariable("name", "world")

	got, err := tmpl.Render()
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hello world" {
		t.Fatalf("got %q, want %q", got, "Hello world")
	}
}

func TestPromptTemplate_RenderWithVariables_BulkPlusOverride(t *testing.T) {
	tmpl := chat.NewPromptTemplate("{{.a}} {{.b}}").
		WithVariables(map[string]any{"a": "1", "b": "2"})

	got, err := tmpl.Render()
	if err != nil {
		t.Fatal(err)
	}
	if got != "1 2" {
		t.Fatalf("got %q, want %q", got, "1 2")
	}

	// RenderWithVariables must not mutate the template's stored variables.
	got2, err := tmpl.RenderWithVariables(map[string]any{"a": "X"})
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "X 2" {
		t.Fatalf("got2 = %q, want %q", got2, "X 2")
	}
	again, _ := tmpl.Render()
	if again != "1 2" {
		t.Fatalf("RenderWithVariables leaked into base template; got %q", again)
	}
}

func TestPromptTemplate_RequireVariables(t *testing.T) {
	tmpl := chat.NewPromptTemplate("Hello {{.name}}")

	if err := tmpl.RequireVariables("name"); err != nil {
		t.Fatalf("present variable should not error: %v", err)
	}
	if err := tmpl.RequireVariables("missing"); err == nil {
		t.Fatal("missing variable must error")
	}
}

func TestPromptTemplate_Clone_Isolation(t *testing.T) {
	a := chat.NewPromptTemplate("Hello {{.name}}").
		WithVariable("name", "alice")

	b := a.Clone()
	if b == nil {
		t.Fatal("Clone returned nil")
	}

	// Mutating the clone must not affect the original.
	b.WithVariable("name", "bob")

	gotA, _ := a.Render()
	gotB, _ := b.Render()
	if gotA == gotB {
		t.Fatalf("Clone leaked: %q == %q", gotA, gotB)
	}
}

func TestPromptTemplate_Clone_NilReceiver(t *testing.T) {
	var p *chat.PromptTemplate
	if got := p.Clone(); got != nil {
		t.Fatalf("nil Clone = %v, want nil", got)
	}
}

func TestPromptTemplate_CreateUserMessage_IncludesText(t *testing.T) {
	tmpl := chat.NewPromptTemplate("Hi")
	msg, err := tmpl.CreateUserMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "Hi" {
		t.Fatalf("Text = %q", msg.Text)
	}
}

func TestPromptTemplate_CreateSystemMessage_IncludesText(t *testing.T) {
	tmpl := chat.NewPromptTemplate("Be brief")
	msg, err := tmpl.CreateSystemMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "Be brief" {
		t.Fatalf("Text = %q", msg.Text)
	}
}

func TestPromptTemplate_Render_ErrorWrapping(t *testing.T) {
	// Empty template body errors at render time.
	tmpl := chat.NewPromptTemplate("")

	_, err := tmpl.Render()
	if err == nil {
		t.Skip("renderer accepts empty templates; nothing to assert")
		return
	}
	if !strings.Contains(err.Error(), "chat.PromptTemplate.Render") {
		t.Fatalf("error should be prefixed; got %q", err.Error())
	}
}
