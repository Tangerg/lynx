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

func TestParseBinding(t *testing.T) {
	parsed := core.ParseBinding("topic:foo.Topic")
	if parsed.Name != "topic" || parsed.Type != "foo.Topic" {
		t.Fatalf("parsed: %+v", parsed)
	}
}
