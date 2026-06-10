package tidb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/tidb"
)

// build is the test driver — parse src, visit, return (sql, args, err).
func build(t *testing.T, src string) (string, []any, error) {
	t.Helper()
	expr, err := filter.ParseAndAnalyze(src)
	if err != nil {
		return "", nil, err
	}
	v := tidb.NewVisitor("metadata")
	if err := v.Visit(expr); err != nil {
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
	if !strings.Contains(sql, "JSON_VALUE(metadata, '$.author')") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must contain JSON_VALUE(metadata, '$.author') IS NULL", sql)
	}
	if len(args) != 0 {
		t.Fatalf("IS NULL takes no bound args, got %v", args)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	sql, args, err := build(t, `author is not null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// NOT(field IS NULL) — semantically IS NOT NULL.
	if !strings.Contains(sql, "NOT") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must wrap IS NULL in NOT", sql)
	}
	if len(args) != 0 {
		t.Fatalf("IS NOT NULL takes no bound args, got %v", args)
	}
}
