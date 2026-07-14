package filterhelp_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/filterhelp"
)

// mustParseBinary parses src and asserts the result is a [*filter.BinaryExpr].
func mustParseBinary(t *testing.T, src string) *filter.BinaryExpr {
	t.Helper()
	expr, err := filter.Parse(src)
	if err != nil {
		t.Fatalf("filter.Parse(%q): %v", src, err)
	}
	be, ok := expr.(*filter.BinaryExpr)
	if !ok {
		t.Fatalf("expected *filter.BinaryExpr, got %T", expr)
	}
	return be
}

func TestDispatchBinary_Routes(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"and is logical", `a == 1 and b == 2`, "logical"},
		{"or is logical", `a == 1 or b == 2`, "logical"},
		{"eq is comparison", `a == 1`, "comparison"},
		{"ne is comparison", `a != 1`, "comparison"},
		{"lt is comparison", `a < 1`, "comparison"},
		{"gte is comparison", `a >= 1`, "comparison"},
		{"in is membership", `a in (1, 2)`, "in"},
		{"like is pattern", `a like '%foo%'`, "like"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := mustParseBinary(t, tc.src)
			got, err := filterhelp.DispatchBinary(
				e,
				func(*filter.BinaryExpr) (string, error) { return "logical", nil },
				func(*filter.BinaryExpr) (string, error) { return "comparison", nil },
				func(*filter.BinaryExpr) (string, error) { return "in", nil },
				func(*filter.BinaryExpr) (string, error) { return "like", nil },
			)
			if err != nil {
				t.Fatalf("DispatchBinary: %v", err)
			}
			if got != tc.want {
				t.Fatalf("routed to %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDispatchBinary_HandlerErrorPropagates(t *testing.T) {
	want := errors.New("boom")
	e := mustParseBinary(t, `a == 1`)
	_, err := filterhelp.DispatchBinary(
		e,
		nil, // unreachable for ==
		func(*filter.BinaryExpr) (string, error) { return "", want },
		nil, nil,
	)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want chain to %v", err, want)
	}
}

func TestDispatchUnary_NotOK(t *testing.T) {
	expr, err := filter.Parse(`not (a == 1)`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	u, ok := expr.(*filter.UnaryExpr)
	if !ok {
		t.Fatalf("expected *filter.UnaryExpr, got %T", expr)
	}
	got, err := filterhelp.DispatchUnary(u,
		func(*filter.UnaryExpr) (string, error) { return "not", nil },
	)
	if err != nil {
		t.Fatalf("DispatchUnary: %v", err)
	}
	if got != "not" {
		t.Fatalf("got %q, want not", got)
	}
}

func TestLogicalOpString(t *testing.T) {
	if op, _ := filterhelp.LogicalOpString(filter.OpAnd); op != "AND" {
		t.Fatalf("AND → %q, want AND", op)
	}
	if op, _ := filterhelp.LogicalOpString(filter.OpOr); op != "OR" {
		t.Fatalf("OR → %q, want OR", op)
	}
	if _, err := filterhelp.LogicalOpString(filter.OpEqual); err == nil {
		t.Fatal("non-logical kind must error")
	}
}

func TestRequireListLiteral_NonEmpty(t *testing.T) {
	e := mustParseBinary(t, `a in (1, 2, 3)`)
	list, err := filterhelp.RequireListLiteral(e)
	if err != nil {
		t.Fatalf("RequireListLiteral: %v", err)
	}
	if got := len(list.Values); got != 3 {
		t.Fatalf("len = %d, want 3", got)
	}
}

func TestRequireListLiteral_RejectsNonList(t *testing.T) {
	e := mustParseBinary(t, `a == 1`)
	_, err := filterhelp.RequireListLiteral(e)
	if err == nil || !strings.Contains(err.Error(), "list") {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestRequireStringPatternOnRight(t *testing.T) {
	e := mustParseBinary(t, `a like '%foo%'`)
	got, err := filterhelp.RequireStringPatternOnRight(e)
	if err != nil {
		t.Fatalf("RequireStringPatternOnRight: %v", err)
	}
	if got != "%foo%" {
		t.Fatalf("got %q, want %%foo%%", got)
	}

	bad := &filter.BinaryExpr{
		Left: filter.NewIdent("a"), Op: filter.OpLike, Right: filter.NewLiteral(42),
	}
	if _, err := filterhelp.RequireStringPatternOnRight(bad); err == nil {
		t.Fatal("non-string pattern must error")
	}
}

func TestConvertListLiteral_Strings(t *testing.T) {
	e := mustParseBinary(t, `a in ('x', 'y', 'z')`)
	list, _ := filterhelp.RequireListLiteral(e)

	slice, sample, err := filterhelp.ConvertListLiteral(list)
	if err != nil {
		t.Fatalf("ConvertListLiteral: %v", err)
	}
	xs, ok := slice.([]string)
	if !ok {
		t.Fatalf("slice type = %T, want []string", slice)
	}
	if len(xs) != 3 || xs[0] != "x" || xs[2] != "z" {
		t.Fatalf("xs = %v, want [x y z]", xs)
	}
	if s, _ := sample.(string); s != "x" {
		t.Fatalf("sample = %v, want x", sample)
	}
}

func TestConvertListLiteral_Numbers(t *testing.T) {
	e := mustParseBinary(t, `a in (1, 2, 3.5)`)
	list, _ := filterhelp.RequireListLiteral(e)

	slice, sample, err := filterhelp.ConvertListLiteral(list)
	if err != nil {
		t.Fatalf("ConvertListLiteral: %v", err)
	}
	ns, ok := slice.([]float64)
	if !ok {
		t.Fatalf("slice type = %T, want []float64", slice)
	}
	if len(ns) != 3 || ns[2] != 3.5 {
		t.Fatalf("ns = %v, want [1 2 3.5]", ns)
	}
	if n, _ := sample.(float64); n != 1.0 {
		t.Fatalf("sample = %v, want 1.0", sample)
	}
}

func TestConvertListLiteral_Bools(t *testing.T) {
	e := mustParseBinary(t, `a in (true, false, true)`)
	list, _ := filterhelp.RequireListLiteral(e)

	slice, _, err := filterhelp.ConvertListLiteral(list)
	if err != nil {
		t.Fatalf("ConvertListLiteral: %v", err)
	}
	bs, ok := slice.([]bool)
	if !ok {
		t.Fatalf("slice type = %T, want []bool", slice)
	}
	if len(bs) != 3 || bs[0] != true || bs[1] != false {
		t.Fatalf("bs = %v, want [true false true]", bs)
	}
}
