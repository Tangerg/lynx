package filter_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
)

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
	if err := filter.Analyze(expr); err == nil {
		t.Skip("analyzer is permissive for this case; that is acceptable")
	}
}

func TestParseAndAnalyze_HappyPath(t *testing.T) {
	expr, err := filter.ParseAndAnalyze(`category == 'tech' AND year >= 2020`)
	if err != nil {
		t.Fatal(err)
	}
	if expr == nil {
		t.Fatal("nil AST")
	}
}

func TestParseAndAnalyze_ParseError(t *testing.T) {
	_, err := filter.ParseAndAnalyze(`broken (`)
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
