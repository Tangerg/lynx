package core_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type sample struct{ V int }

func TestNewIOBindingPackagePath(t *testing.T) {
	b := core.NewIOBinding[sample]("input")
	if b.Name != "input" {
		t.Fatalf("name: got %q want %q", b.Name, "input")
	}
	// The fully-qualified type name should include the package path so
	// same-named types in other packages don't collide as planner keys.
	if !strings.Contains(b.Type, "core_test.sample") {
		t.Fatalf("expected pkg-qualified name, got %q", b.Type)
	}
}

func TestIOBindingDefaultName(t *testing.T) {
	b := core.NewIOBinding[sample]("")
	if !b.IsDefault() {
		t.Fatalf("empty name should be default")
	}
	if b.String() != core.DefaultBinding+":"+b.Type {
		t.Fatalf("string: %q", b.String())
	}
}

func TestParseIOBinding(t *testing.T) {
	parsed := core.ParseIOBinding("topic:foo.Topic")
	if parsed.Name != "topic" || parsed.Type != "foo.Topic" {
		t.Fatalf("parsed: %+v", parsed)
	}
}
