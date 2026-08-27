package azurecosmos

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
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
			if err := filter.Like("name", test.pattern).Accept(compiler); err != nil {
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
		if err := filter.Like("name", pattern).Accept(compiler); err == nil {
			t.Errorf("Visit(LIKE %q) error = nil, want unsupported pattern", pattern)
		}
	}
}

func TestCollectionMembershipUsesArrayContains(t *testing.T) {
	t.Parallel()

	visitor := NewVisitor("c", "metadata")
	if err := filter.Has("visible_to", "user-42").Accept(visitor); err != nil {
		t.Fatal(err)
	}
	query, params := visitor.Result()
	if want := "ARRAY_CONTAINS(c.metadata.visible_to, @p1)"; query != want {
		t.Fatalf("Result() = %q, want %q", query, want)
	}
	if len(params) != 1 || params[0].Name != "@p1" || params[0].Value != "user-42" {
		t.Fatalf("params = %#v", params)
	}
}
