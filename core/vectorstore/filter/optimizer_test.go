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
