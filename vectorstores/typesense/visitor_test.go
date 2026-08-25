package typesense_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/typesense"
)

func TestVisitorCollectionMembershipUsesExactArrayMatch(t *testing.T) {
	visitor := typesense.NewVisitor("metadata")
	if err := filter.Has("visible_to", "user-42").Accept(visitor); err != nil {
		t.Fatal(err)
	}
	if got, want := visitor.Result(), "metadata.visible_to:= user-42"; got != want {
		t.Fatalf("Result() = %q, want %q", got, want)
	}
}
