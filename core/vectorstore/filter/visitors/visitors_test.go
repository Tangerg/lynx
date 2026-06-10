package visitors_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter/parser"
	"github.com/Tangerg/lynx/core/vectorstore/filter/visitors"
)

// --- Analyzer ------------------------------------------------------------

func analyze(t *testing.T, input string) error {
	t.Helper()
	expr, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	return visitors.NewAnalyzer().Visit(expr)
}

func TestAnalyzer_HappyPath(t *testing.T) {
	if err := analyze(t, `category == 'tech' AND year >= 2020`); err != nil {
		t.Fatalf("clean expression must analyze without error: %v", err)
	}
}

func TestAnalyzer_OrderingRequiresNumber(t *testing.T) {
	if err := analyze(t, `name >= 'jay'`); err == nil {
		t.Fatal("string with >= must error")
	}
}

func TestAnalyzer_LikeRequiresString(t *testing.T) {
	if err := analyze(t, `name LIKE 42`); err == nil {
		t.Fatal("LIKE with non-string must error")
	}
}

func TestAnalyzer_LogicalRequiresComputed(t *testing.T) {
	// `1 AND a == 1` — left side is a bare literal, not computed.
	if err := analyze(t, `1 AND a == 1`); err == nil {
		t.Fatal("logical with literal LHS must error")
	}
}

func TestAnalyzer_EqualityNeedsLiteralRight(t *testing.T) {
	// `a == b` — right is identifier, not a literal.
	if err := analyze(t, `a == b`); err == nil {
		t.Fatal("equality with identifier RHS must error")
	}
}

func TestAnalyzer_NestedExpressions(t *testing.T) {
	if err := analyze(t, `(a == 1 OR b == 2) AND NOT (c == 3)`); err != nil {
		t.Fatalf("nested expression error: %v", err)
	}
}

func TestAnalyzer_IndexExpression(t *testing.T) {
	if err := analyze(t, `tags[0] == 'urgent'`); err != nil {
		t.Fatalf("index expression error: %v", err)
	}
}

// --- SQLLikeVisitor -----------------------------------------------------

func render(t *testing.T, input string) string {
	t.Helper()
	expr, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("parse %q: %v", input, err)
	}
	v := visitors.NewSQLLikeVisitor()
	if err := v.Visit(expr); err != nil {
		t.Fatalf("render error: %v", err)
	}
	return v.SQL()
}

func TestSQLLike_Equality(t *testing.T) {
	got := render(t, `name == 'john'`)
	if !strings.Contains(got, "name") || !strings.Contains(got, "'john'") {
		t.Fatalf("rendered = %q", got)
	}
}

func TestSQLLike_NumberLiteral(t *testing.T) {
	got := render(t, `age >= 18`)
	if !strings.Contains(got, "18") {
		t.Fatalf("rendered = %q, want it to contain 18", got)
	}
	// Numeric literals must NOT be quoted.
	if strings.Contains(got, "'18'") {
		t.Fatalf("rendered = %q — numeric literal must not be quoted", got)
	}
}

func TestSQLLike_BooleanLiteral(t *testing.T) {
	got := render(t, `active == true`)
	if !strings.Contains(got, "true") {
		t.Fatalf("rendered = %q", got)
	}
	if strings.Contains(got, "'true'") {
		t.Fatalf("rendered = %q — boolean literal must not be quoted", got)
	}
}

func TestSQLLike_InList(t *testing.T) {
	got := render(t, `status IN ('active', 'pending')`)
	if !strings.Contains(got, "'active'") || !strings.Contains(got, "'pending'") {
		t.Fatalf("rendered = %q", got)
	}
}

func TestSQLLike_LogicalChain(t *testing.T) {
	got := render(t, `a == 1 AND b == 2`)
	if !strings.Contains(strings.ToLower(got), "and") {
		t.Fatalf("rendered = %q, missing AND", got)
	}
}

func TestSQLLike_IndexExpression(t *testing.T) {
	got := render(t, `tags[0] == 'x'`)
	if !strings.Contains(got, "tags") || !strings.Contains(got, "[") || !strings.Contains(got, "]") {
		t.Fatalf("rendered = %q", got)
	}
}

func TestSQLLike_NotExpression(t *testing.T) {
	got := render(t, `NOT (a == 1)`)
	if !strings.Contains(strings.ToLower(got), "not") {
		t.Fatalf("rendered = %q, missing NOT", got)
	}
}

func TestSQLLike_Roundtrip(t *testing.T) {
	// Render, then parse the rendered output, then render again — the
	// two renderings should be identical (idempotent).
	first := render(t, `category == 'tech' AND year >= 2020`)
	second := render(t, first)
	if first != second {
		t.Fatalf("not idempotent:\n  first=%q\n  second=%q", first, second)
	}
}

func TestSQLLike_StringEscapeRoundtrip(t *testing.T) {
	// The lexer decodes \' \\ \n \t \r inside string literals; the
	// renderer must re-apply those escapes or an embedded quote breaks
	// out of the quoted form and the output no longer re-parses.
	first := render(t, `name == 'O\'Brien \\ line\nbreak'`)
	second := render(t, first)
	if first != second {
		t.Fatalf("escape round-trip not idempotent:\n  first=%q\n  second=%q", first, second)
	}
	if !strings.Contains(first, `'O\'Brien`) {
		t.Fatalf("rendered = %q — embedded quote must stay escaped", first)
	}
}
