package pinecone_test

import (
	"reflect"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/pinecone"
	"github.com/Tangerg/lynx/vectorstores/storetest"
)

// TestVisitor_Conformance runs the shared visitor suite. Pinecone
// metadata filters don't support LIKE — that conformance case is
// declared via [storetest.Options.Unsupported].
func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t,
		func(src string) error {
			expr, err := filter.Parse(src)
			if err != nil {
				return err
			}
			v := pinecone.NewVisitor()
			return v.Visit(expr)
		},
		storetest.Options{
			// Pinecone metadata filters have no LIKE operator.
			Unsupported: []string{"like"},
		},
	)
}

func TestVisitor_CollectionMembershipUsesScalarEquality(t *testing.T) {
	result, err := pinecone.ToFilter(filter.Has("visible_to", "user-42"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"visible_to": map[string]any{"$eq": "user-42"}}
	if got := result.AsMap(); !reflect.DeepEqual(got, want) {
		t.Fatalf("filter = %#v, want %#v", got, want)
	}
	if _, err := pinecone.ToFilter(filter.Has("visible_to", 42)); err == nil {
		t.Fatal("Pinecone accepted a non-string list member")
	}
}

func TestVisitor_NotUsesOnlyPineconeOperators(t *testing.T) {
	tests := []struct {
		name      string
		predicate filter.Predicate
		want      map[string]any
	}{
		{
			name:      "comparison inverse",
			predicate: filter.Not(filter.EQ("status", "draft")),
			want:      map[string]any{"status": map[string]any{"$ne": "draft"}},
		},
		{
			name:      "not in",
			predicate: filter.Not(filter.In("status", []string{"draft", "deleted"})),
			want:      map[string]any{"status": map[string]any{"$nin": []any{"draft", "deleted"}}},
		},
		{
			name:      "not has",
			predicate: filter.Not(filter.Has("visible_to", "user-42")),
			want:      map[string]any{"visible_to": map[string]any{"$ne": "user-42"}},
		},
		{
			name:      "de morgan",
			predicate: filter.Not(filter.And(filter.EQ("a", 1), filter.EQ("b", 2))),
			want: map[string]any{"$or": []any{
				map[string]any{"a": map[string]any{"$ne": float64(1)}},
				map[string]any{"b": map[string]any{"$ne": float64(2)}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := pinecone.ToFilter(test.predicate)
			if err != nil {
				t.Fatal(err)
			}
			if got := result.AsMap(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("filter = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestVisitor_RejectsIntegerThatStructPBCannotRepresent(t *testing.T) {
	_, err := pinecone.ToFilter(filter.EQ("id", uint64(1<<53+1)))
	if err == nil {
		t.Fatal("Pinecone silently rounded a large integer")
	}
}
