package azurecosmos

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestLikeCompilationPreservesPatternSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		pattern string
		want    string
	}{
		{pattern: "alpha", want: "c.metadata.name = @p1"},
		{pattern: "alpha%", want: "STARTSWITH(c.metadata.name, @p1)"},
		{pattern: "%alpha", want: "ENDSWITH(c.metadata.name, @p1)"},
		{pattern: "%alpha%", want: "CONTAINS(c.metadata.name, @p1)"},
	}

	for _, test := range tests {
		t.Run(test.pattern, func(t *testing.T) {
			t.Parallel()
			compiler := NewVisitor("c", "metadata")
			if err := compiler.Visit(filter.Like("name", test.pattern)); err != nil {
				t.Fatal(err)
			}
			if got, _ := compiler.Result(); got != test.want {
				t.Fatalf("Result() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestLikeCompilationRejectsUnsupportedPatterns(t *testing.T) {
	t.Parallel()

	for _, pattern := range []string{"a_b", "a%b", "%"} {
		compiler := NewVisitor("c", "metadata")
		if err := compiler.Visit(filter.Like("name", pattern)); err == nil {
			t.Errorf("Visit(LIKE %q) error = nil, want unsupported pattern", pattern)
		}
	}
}
