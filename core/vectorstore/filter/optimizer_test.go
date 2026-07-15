package filter_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestParseOptimizerBooleanIdentities(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect filter.Predicate
	}{
		{
			name:   "triple negation",
			input:  `not not not active == true`,
			expect: filter.Not(filter.EQ("active", true)),
		},
		{
			name:   "and idempotence",
			input:  `a == 1 and a == 1`,
			expect: filter.EQ("a", 1),
		},
		{
			name:   "or idempotence",
			input:  `a == 1 or a == 1`,
			expect: filter.EQ("a", 1),
		},
		{
			name:   "and absorption",
			input:  `a == 1 and (a == 1 or b == 2)`,
			expect: filter.EQ("a", 1),
		},
		{
			name:   "or absorption reversed",
			input:  `(b == 2 and a == 1) or a == 1`,
			expect: filter.EQ("a", 1),
		},
		{
			name:  "associative deduplication",
			input: `(a == 1 and b == 2) and a == 1`,
			expect: filter.And(
				filter.EQ("a", 1),
				filter.EQ("b", 2),
			),
		},
		{
			name:   "deep absorption",
			input:  `a == 1 and (b == 2 or (c == 3 or a == 1))`,
			expect: filter.EQ("a", 1),
		},
		{
			name:  "commutative clause absorption",
			input: `(a == 1 or b == 2) and (b == 2 or a == 1 or c == 3)`,
			expect: filter.Or(
				filter.EQ("a", 1),
				filter.EQ("b", 2),
			),
		},
		{
			name:  "factor conjunction",
			input: `(a == 1 and b == 2) or (a == 1 and c == 3)`,
			expect: filter.And(
				filter.EQ("a", 1),
				filter.Or(filter.EQ("b", 2), filter.EQ("c", 3)),
			),
		},
		{
			name:  "factor disjunction",
			input: `(a == 1 or b == 2) and (a == 1 or c == 3)`,
			expect: filter.Or(
				filter.EQ("a", 1),
				filter.And(filter.EQ("b", 2), filter.EQ("c", 3)),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := filter.Parse(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if !actual.Equal(tt.expect) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tt.input, actual, tt.expect)
			}
		})
	}
}

func TestValidateDoesNotRewriteProgrammaticPredicate(t *testing.T) {
	comparison := filter.EQ("a", 1)
	predicate := filter.And(comparison, comparison)

	if err := filter.Validate(predicate); err != nil {
		t.Fatal(err)
	}
	if predicate.Op != filter.OpAnd || predicate.Left != comparison || predicate.Right != comparison {
		t.Fatal("Validate rewrote the caller-owned predicate")
	}
}

func TestParseOptimizerPreservesMembershipOperands(t *testing.T) {
	predicate, err := filter.Parse(`status in ('active', 'active', 'paused')`)
	if err != nil {
		t.Fatal(err)
	}
	list := predicate.(*filter.BinaryExpr).Right.(*filter.ListLiteral)
	if len(list.Values) != 3 || list.Values[0].Value != "active" || list.Values[1].Value != "active" {
		t.Fatalf("membership values = %#v, want source order and duplicates preserved", list.Values)
	}
}
