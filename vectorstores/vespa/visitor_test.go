package vespa

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func TestVisitorCollectionMembershipUsesContains(t *testing.T) {
	visitor := newVisitor("")
	if err := filter.Has("visible_to", "user-42").Accept(visitor); err != nil {
		t.Fatal(err)
	}
	if got, want := visitor.snapshot(), `visible_to contains "user-42"`; got != want {
		t.Fatalf("Result() = %q, want %q", got, want)
	}
}
