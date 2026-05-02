package core_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
)

type sample struct{ V int }

func TestNewIoBindingPackagePath(t *testing.T) {
	b := core.NewIoBinding[sample]("input")
	if b.Name != "input" {
		t.Fatalf("name: got %q want %q", b.Name, "input")
	}
	// The fully-qualified type name should include the package path so
	// same-named types in other packages don't collide as planner keys.
	if !strings.Contains(b.Type, "core_test.sample") {
		t.Fatalf("expected pkg-qualified name, got %q", b.Type)
	}
}

func TestIoBindingDefaultName(t *testing.T) {
	b := core.NewIoBinding[sample]("")
	if !b.IsDefault() {
		t.Fatalf("empty name should be default")
	}
	if b.String() != core.DefaultBinding+":"+b.Type {
		t.Fatalf("string: %q", b.String())
	}
}

func TestParseIoBinding(t *testing.T) {
	parsed := core.ParseIoBinding("topic:foo.Topic")
	if parsed.Name != "topic" || parsed.Type != "foo.Topic" {
		t.Fatalf("parsed: %+v", parsed)
	}
}
