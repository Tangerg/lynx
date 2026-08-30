package s3vectors

import (
	"reflect"
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func TestVisitorCollectionMembershipUsesScalarEquality(t *testing.T) {
	visitor := newVisitor()
	if err := filter.Has("visible_to", "user-42").Accept(visitor); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"visible_to": map[string]any{"$eq": "user-42"}}
	if got := visitor.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Result() = %#v, want %#v", got, want)
	}
}
