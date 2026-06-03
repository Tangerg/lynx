package clickhouse_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/clickhouse"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
)

// TestVisitor_Conformance exercises every AST shape the filter DSL
// supports against the ClickHouse visitor via the shared
// [storetest.VisitorConformance] suite. Output equivalence stays in the
// per-test functions below; this is "no shape crashes" coverage.
func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.ParseAndAnalyze(src)
		if err != nil {
			return err
		}
		v := clickhouse.NewVisitor("metadata")
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
	v := clickhouse.NewVisitor("metadata")
	v.Visit(expr)
	if err := v.Error(); err != nil {
		return "", nil, err
	}
	sql, args := v.Result()
	return sql, args, nil
}

func TestVisitor_IsNull(t *testing.T) {
	sql, args, err := build(t, `author is null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "mapContains(metadata, 'author')") {
		t.Fatalf("sql=%q must test key presence with mapContains(metadata, 'author')", sql)
	}
	if !strings.Contains(sql, "NOT") {
		t.Fatalf("sql=%q must negate mapContains for an IS NULL test", sql)
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
	// NOT(field IS NULL) — semantically IS NOT NULL. The IS NULL renders
	// as NOT mapContains(...), then the IS NOT NULL NOT wrapper negates it
	// again, yielding NOT (NOT mapContains(...)).
	if !strings.Contains(sql, "mapContains(metadata, 'author')") {
		t.Fatalf("sql=%q must test key presence with mapContains(metadata, 'author')", sql)
	}
	if strings.Count(sql, "NOT") < 2 {
		t.Fatalf("sql=%q must wrap the IS NULL (NOT mapContains) in an outer NOT", sql)
	}
}
