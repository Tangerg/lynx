package pgvector_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/pgvector"
)

// TestVisitor_Conformance exercises every AST shape the filter DSL
// supports against the pgvector visitor via the shared
// [storetest.VisitorConformance] suite. Output equivalence stays in
// the per-test functions below; this is "no shape crashes" coverage.
func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.ParseAndAnalyze(src)
		if err != nil {
			return err
		}
		v := pgvector.NewVisitor("metadata")
		v.Visit(expr)
		return v.Error()
	})
}

// build is the test driver — parse src, visit, return (sql, args, err).
func build(t *testing.T, src string) (string, []any, error) {
	t.Helper()
	expr, err := filter.ParseAndAnalyze(src)
	if err != nil {
		return "", nil, err
	}
	v := pgvector.NewVisitor("metadata")
	v.Visit(expr)
	if err := v.Error(); err != nil {
		return "", nil, err
	}
	sql, args := v.Result()
	return sql, args, nil
}

func TestVisitor_EqualityString(t *testing.T) {
	sql, args, err := build(t, `author == 'Alice'`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "metadata->>'author'") {
		t.Fatalf("sql=%q must address metadata->>'author'", sql)
	}
	if !strings.Contains(sql, "= $1") {
		t.Fatalf("sql=%q must contain '= $1'", sql)
	}
	if !reflect.DeepEqual(args, []any{"Alice"}) {
		t.Fatalf("args=%v, want [Alice]", args)
	}
}

func TestVisitor_EqualityNumberCastsNumeric(t *testing.T) {
	sql, args, err := build(t, `year == 2020`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "::numeric") {
		t.Fatalf("sql=%q must contain ::numeric cast for number compare", sql)
	}
	// Whole numbers come back as int64 from the literal converter.
	if !reflect.DeepEqual(args, []any{int64(2020)}) {
		t.Fatalf("args=%v, want [int64(2020)]", args)
	}
}

func TestVisitor_EqualityBoolCastsBoolean(t *testing.T) {
	sql, args, err := build(t, `published == true`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "::boolean") {
		t.Fatalf("sql=%q must contain ::boolean cast for bool compare", sql)
	}
	if !reflect.DeepEqual(args, []any{true}) {
		t.Fatalf("args=%v, want [true]", args)
	}
}

func TestVisitor_Ordering(t *testing.T) {
	sql, args, err := build(t, `year >= 2020`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, ">= $1") {
		t.Fatalf("sql=%q must contain '>= $1'", sql)
	}
	if !reflect.DeepEqual(args, []any{int64(2020)}) {
		t.Fatalf("args=%v, want [int64(2020)]", args)
	}
}

func TestVisitor_LogicalAnd(t *testing.T) {
	sql, args, err := build(t, `author == 'Alice' and year >= 2020`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, " AND ") {
		t.Fatalf("sql=%q must contain ' AND '", sql)
	}
	if len(args) != 2 {
		t.Fatalf("args=%v, want 2 placeholders", args)
	}
	// Placeholders must be numbered sequentially.
	if !strings.Contains(sql, "$1") || !strings.Contains(sql, "$2") {
		t.Fatalf("sql=%q must contain $1 and $2", sql)
	}
}

func TestVisitor_LogicalOr(t *testing.T) {
	sql, _, err := build(t, `a == 1 or b == 2`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, " OR ") {
		t.Fatalf("sql=%q must contain ' OR '", sql)
	}
}

func TestVisitor_Not(t *testing.T) {
	sql, _, err := build(t, `not (a == 1)`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "(NOT ") {
		t.Fatalf("sql=%q must contain '(NOT '", sql)
	}
}

func TestVisitor_InStrings(t *testing.T) {
	sql, args, err := build(t, `tag in ('rag', 'llm')`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "= ANY($1)") {
		t.Fatalf("sql=%q must contain '= ANY($1)'", sql)
	}
	if !reflect.DeepEqual(args, []any{[]string{"rag", "llm"}}) {
		t.Fatalf("args=%v, want [[rag llm]]", args)
	}
}

func TestVisitor_InNumbers(t *testing.T) {
	sql, args, err := build(t, `year in (2020, 2021, 2022)`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "::numeric") {
		t.Fatalf("sql=%q must cast left side to ::numeric for numeric IN", sql)
	}
	want := []any{[]float64{2020, 2021, 2022}}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args=%v, want %v", args, want)
	}
}

func TestVisitor_Like(t *testing.T) {
	sql, args, err := build(t, `author like '%Alice%'`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "ILIKE $1") {
		t.Fatalf("sql=%q must contain 'ILIKE $1'", sql)
	}
	if !reflect.DeepEqual(args, []any{"%Alice%"}) {
		t.Fatalf("args=%v, want [%%Alice%%]", args)
	}
}

func TestVisitor_NestedIndex(t *testing.T) {
	sql, _, err := build(t, `metadata['a']['b'] == 'x'`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Nested: intermediate hops use ->, final step uses ->>.
	if !strings.Contains(sql, "metadata->'a'->>'b'") {
		t.Fatalf("sql=%q must contain metadata->'a'->>'b'", sql)
	}
}

func TestVisitor_IndexedKeyStripsBase(t *testing.T) {
	sql, _, err := build(t, `metadata['author'] == 'Alice'`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "metadata->>'author'") {
		t.Fatalf("sql=%q must contain metadata->>'author'", sql)
	}
}

func TestVisitor_InRejectsScalar(t *testing.T) {
	// Parser unwraps single-element parens (a in (1)) to a bare
	// literal — that's a grammar choice, not a visitor bug — so this
	// path exercises the IN handler's "right side is not a list" branch.
	_, _, err := build(t, `a in (1)`)
	if err == nil {
		t.Fatal("expected error: IN with scalar right side")
	}
	if !strings.Contains(err.Error(), "IN") {
		t.Fatalf("err=%v should mention IN", err)
	}
}

func TestVisitor_EmptyMetadataColDefaults(t *testing.T) {
	expr, err := filter.ParseAndAnalyze(`a == 1`)
	if err != nil {
		t.Fatalf("ParseAndAnalyze: %v", err)
	}
	v := pgvector.NewVisitor("") // empty → defaults to "metadata"
	v.Visit(expr)
	if v.Error() != nil {
		t.Fatalf("visit: %v", v.Error())
	}
	sql, _ := v.Result()
	if !strings.Contains(sql, "metadata->>'a'") {
		t.Fatalf("sql=%q must default to metadata col", sql)
	}
}

func TestVisitor_NilExpression(t *testing.T) {
	v := pgvector.NewVisitor("metadata")
	v.Visit(nil)
	if v.Error() == nil {
		t.Fatal("nil expression must produce an error")
	}
}

func TestVisitor_IsNull(t *testing.T) {
	sql, args, err := build(t, `author is null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "metadata->>'author'") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must contain metadata->>'author' IS NULL", sql)
	}
	if len(args) != 0 {
		t.Fatalf("IS NULL takes no bound args, got %v", args)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	sql, _, err := build(t, `author is not null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// NOT(field IS NULL) — semantically IS NOT NULL.
	if !strings.Contains(sql, "NOT") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must wrap IS NULL in NOT", sql)
	}
}
