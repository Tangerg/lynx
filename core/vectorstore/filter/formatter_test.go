package filter_test

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func TestFormatterCanonicalDSL(t *testing.T) {
	tests := []struct {
		name      string
		predicate filter.Predicate
		expect    string
	}{
		{
			name: "precedence",
			predicate: filter.And(
				filter.Or(filter.EQ("a", 1), filter.EQ("b", 2)),
				filter.EQ("c", 3),
			),
			expect: `(a == 1 or b == 2) and c == 3`,
		},
		{
			name: "right association",
			predicate: filter.Or(
				filter.EQ("a", 1),
				filter.Or(filter.EQ("b", 2), filter.EQ("c", 3)),
			),
			expect: `a == 1 or (b == 2 or c == 3)`,
		},
		{
			name: "escaped nested selector",
			predicate: filter.EQ(
				filter.Index(filter.Index("metadata", "a'b"), 2),
				"line\n\\tail",
			),
			expect: `metadata['a\'b'][2] == 'line\n\\tail'`,
		},
		{
			name:      "membership",
			predicate: filter.Not(filter.In("tags", []string{"go", "ai"})),
			expect:    `not (tags in ('go', 'ai'))`,
		},
		{
			name:      "collection membership",
			predicate: filter.Has("visible_to", "user-42"),
			expect:    `visible_to has 'user-42'`,
		},
		{
			name:      "null test",
			predicate: filter.IsNotNull("deleted_at"),
			expect:    `not (deleted_at is null)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if actual := tt.predicate.String(); actual != tt.expect {
				t.Fatalf("Predicate.String() = %q, want %q", actual, tt.expect)
			}

			roundTrip, err := filter.Parse(tt.predicate.String())
			if err != nil {
				t.Fatalf("Parse(Predicate.String()) = %v", err)
			}
			if !roundTrip.Equal(tt.predicate) {
				t.Fatalf("round trip = %#v, want %#v", roundTrip, tt.predicate)
			}
		})
	}
}

func TestMalformedPredicateHasNoStringRepresentation(t *testing.T) {
	if actual := (&filter.BinaryExpr{}).String(); actual != "" {
		t.Fatalf("malformed predicate String() = %q, want empty", actual)
	}
}
