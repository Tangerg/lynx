package core_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type sample struct{ V int }

func TestNewBindingIncludesPackagePath(t *testing.T) {
	binding := core.NewBinding[sample]("input")
	if binding.Name != "input" {
		t.Fatalf("name: got %q want %q", binding.Name, "input")
	}
	// The fully-qualified type name should include the package path so
	// same-named types in other packages don't collide as planner keys.
	if !strings.Contains(binding.Type, "core_test.sample") {
		t.Fatalf("expected pkg-qualified name, got %q", binding.Type)
	}
}

func TestBindingDefaultName(t *testing.T) {
	binding := core.NewBinding[sample]("")
	if !binding.IsDefault() {
		t.Fatalf("empty name should be default")
	}
	if binding.String() != core.DefaultBindingName+":"+binding.Type {
		t.Fatalf("string: %q", binding.String())
	}
}

func TestBindingValidateRejectsAmbiguousIdentity(t *testing.T) {
	tests := []core.Binding{
		{Name: "topic:raw", Type: "example.Topic"},
		{Name: " topic", Type: "example.Topic"},
		{Name: "topic", Type: ""},
		{Name: "topic", Type: " example.Topic"},
	}
	for _, binding := range tests {
		if err := binding.Validate(); err == nil {
			t.Errorf("Binding%+v validated successfully", binding)
		}
	}
	if err := (core.Binding{Type: "example.Topic"}).Validate(); err != nil {
		t.Fatalf("default binding: %v", err)
	}
}

func TestBindingsZeroValue(t *testing.T) {
	var bindings core.Bindings
	if bindings.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", bindings.Len())
	}
	bindings.Set("topic", sample{V: 1})
	value, ok := bindings.Get("topic")
	if !ok || value != (sample{V: 1}) {
		t.Fatalf("Get(topic) = %#v, %t", value, ok)
	}
	bindings.Delete("topic")
	if _, ok := bindings.Get("topic"); ok {
		t.Fatal("Delete left topic present")
	}
}

func TestBindingsCloneOwnsContainer(t *testing.T) {
	bindings := core.Input(sample{V: 1})
	clone := bindings.Clone()
	bindings.Set(core.DefaultBindingName, sample{V: 2})
	clone.Set("extra", true)

	value, _ := clone.Get(core.DefaultBindingName)
	if value != (sample{V: 1}) {
		t.Fatalf("clone input = %#v, want original value", value)
	}
	if _, ok := bindings.Get("extra"); ok {
		t.Fatal("clone mutation leaked into source")
	}
}
