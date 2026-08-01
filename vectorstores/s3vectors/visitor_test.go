package s3vectors_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/s3vectors"
)

func TestVisitorCollectionMembershipUsesScalarEquality(t *testing.T) {
	visitor := s3vectors.NewVisitor()
	if err := visitor.Visit(filter.Has("visible_to", "user-42")); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"visible_to": map[string]any{"$eq": "user-42"}}
	if got := visitor.Result(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Result() = %#v, want %#v", got, want)
	}
}
