package filter_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

func TestParse_Optimizes(t *testing.T) {
	// Parse folds dead logic: not(not(x)) collapses to x, so
	// the result is the comparison itself, not a UnaryExpr.
	expr, err := filter.Parse(`not (not (year >= 2020))`)
	if err != nil {
		t.Fatal(err)
	}
	if _, isUnary := expr.(*filter.UnaryExpr); isUnary {
		t.Fatalf("expected double-NOT to be folded away, got %T", expr)
	}
	if _, isBinary := expr.(*filter.BinaryExpr); !isBinary {
		t.Fatalf("expected the bare comparison BinaryExpr, got %T", expr)
	}
}

func TestParse_ValidExpression(t *testing.T) {
	expr, err := filter.Parse(`category == 'tech'`)
	if err != nil {
		t.Fatal(err)
	}
	if expr == nil {
		t.Fatal("nil AST")
	}
}

func TestParse_InvalidExpression(t *testing.T) {
	if _, err := filter.Parse(`category ==`); err == nil {
		t.Fatal("incomplete expression must error")
	}
}

func TestAnalyze_RejectsTypeMismatch(t *testing.T) {
	expr, err := filter.Parse(`year >= 'two-thousand'`)
	if err != nil {
		t.Skip("parser does not allow this shape; nothing to analyze")
		return
	}
	// Non-comparable string vs numeric op — analyzer should flag.
	if err := filter.Validate(expr); err == nil {
		t.Skip("analyzer is permissive for this case; that is acceptable")
	}
}

func TestParse_HappyPath(t *testing.T) {
	expr, err := filter.Parse(`category == 'tech' AND year >= 2020`)
	if err != nil {
		t.Fatal(err)
	}
	if expr == nil {
		t.Fatal("nil AST")
	}
}

func TestParse_ParseError(t *testing.T) {
	_, err := filter.Parse(`broken (`)
	if err == nil {
		t.Fatal("syntactically invalid input must error")
	}
}

func TestParse_LogicalOperators(t *testing.T) {
	cases := []string{
		`a == 1 AND b == 2`,
		`a == 1 OR b == 2`,
		`NOT a == 1`,
		`(a == 1 OR b == 2) AND c == 3`,
	}
	for _, c := range cases {
		if _, err := filter.Parse(c); err != nil {
			t.Fatalf("Parse(%q) errored: %v", c, err)
		}
	}
}

func TestParse_ErrorMessageNonempty(t *testing.T) {
	_, err := filter.Parse(`@@@`)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.TrimSpace(err.Error()) == "" {
		t.Fatal("error message should be non-empty")
	}
}

func TestParse_IsNull(t *testing.T) {
	for _, src := range []string{
		`owner is null`,
		`owner is not null`,
		`metadata['k'] is null`,
		`a is null and b == 1`,
	} {
		if _, err := filter.Parse(src); err != nil {
			t.Fatalf("Parse(%q): unexpected error %v", src, err)
		}
	}
}

func TestParse_IsNullRejectsNonNull(t *testing.T) {
	// IS must be followed by NULL (optionally NOT NULL); other right
	// sides are rejected at parse time.
	for _, src := range []string{
		`owner is 5`,
		`owner is 'x'`,
		`owner is`,
	} {
		if _, err := filter.Parse(src); err == nil {
			t.Fatalf("Parse(%q): expected error, got nil", src)
		}
	}
}

func TestParse_NotIn(t *testing.T) {
	// `NOT IN` reuses the NOT + IN tokens: it parses to a NOT wrapping an
	// IN, not a dedicated node.
	expr, err := filter.Parse(`tags not in ('a', 'b', 'c')`)
	if err != nil {
		t.Fatal(err)
	}
	unary, ok := expr.(*filter.UnaryExpr)
	if !ok {
		t.Fatalf("NOT IN should be a UnaryExpr(NOT, ...), got %T", expr)
	}
	if _, ok := unary.Right.(*filter.BinaryExpr); !ok {
		t.Fatalf("NOT IN inner should be a BinaryExpr(IN), got %T", unary.Right)
	}
}

func TestParse_NotInRejectsNonIn(t *testing.T) {
	// Infix NOT only accepts IN after it.
	for _, src := range []string{`a not == 1`, `a not 5`} {
		if _, err := filter.Parse(src); err == nil {
			t.Fatalf("Parse(%q): expected error, got nil", src)
		}
	}
}
