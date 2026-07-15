package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
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
			name:      "null test",
			predicate: filter.IsNotNull("deleted_at"),
			expect:    `not (deleted_at is null)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var formatter filter.Formatter
			if err := filter.Visit(tt.predicate, &formatter); err != nil {
				t.Fatal(err)
			}
			if actual := formatter.String(); actual != tt.expect {
				t.Fatalf("Formatter.String() = %q, want %q", actual, tt.expect)
			}

			roundTrip, err := filter.Parse(formatter.String())
			if err != nil {
				t.Fatalf("Parse(Formatter.String()) = %v", err)
			}
			if !roundTrip.Equal(tt.predicate) {
				t.Fatalf("round trip = %#v, want %#v", roundTrip, tt.predicate)
			}
		})
	}
}

func TestFormatterLifecycle(t *testing.T) {
	var formatter filter.Formatter
	if formatter.String() != "" {
		t.Fatal("zero-value Formatter has a result")
	}
	if err := formatter.Visit(filter.EQ("first", 1)); err != nil {
		t.Fatal(err)
	}
	if formatter.String() != "first == 1" {
		t.Fatalf("first result = %q", formatter.String())
	}
	if err := formatter.Visit(&filter.BinaryExpr{}); err == nil {
		t.Fatal("Formatter accepted a malformed predicate")
	}
	if formatter.String() != "" {
		t.Fatalf("failed Visit retained %q", formatter.String())
	}
	if err := formatter.Visit(filter.EQ("second", true)); err != nil {
		t.Fatal(err)
	}
	if formatter.String() != "second == true" {
		t.Fatalf("reused result = %q", formatter.String())
	}
}

func TestFormatterNilReceiver(t *testing.T) {
	var formatter *filter.Formatter
	if formatter.String() != "" {
		t.Fatal("nil Formatter returned a result")
	}
	if err := formatter.Visit(filter.EQ("field", 1)); err == nil {
		t.Fatal("nil Formatter accepted Visit")
	}
}
