package vespa_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/vespa"
)

func TestVisitorCollectionMembershipUsesContains(t *testing.T) {
	visitor := vespa.NewVisitor("")
	if err := filter.Has("visible_to", "user-42").Accept(visitor); err != nil {
		t.Fatal(err)
	}
	if got, want := visitor.Result(), `visible_to contains "user-42"`; got != want {
		t.Fatalf("Result() = %q, want %q", got, want)
	}
}
