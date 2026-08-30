package couchbase

import (
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
	"github.com/Tangerg/scope/core/vectorstore/storetest"
)

// TestVisitor_Conformance exercises every AST shape the filter DSL
// supports against the couchbase visitor via the shared
// [storetest.VisitorConformance] suite. This is "no shape crashes"
// coverage; output equivalence stays in the per-test functions below.
func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.Parse(src)
		if err != nil {
			return err
		}
		v := newVisitor("metadata")
		return expr.Accept(v)
	})
}

// build is the test driver — parse src, visit, return (sql, err).
func build(t *testing.T, src string) (string, error) {
	t.Helper()
	expr, err := filter.Parse(src)
	if err != nil {
		return "", err
	}
	v := newVisitor("metadata")
	if err := expr.Accept(v); err != nil {
		return "", err
	}
	return v.snapshot(), nil
}

func TestVisitor_IsNull(t *testing.T) {
	sql, err := build(t, `author is null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(sql, "metadata.`author`") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must contain metadata.`author` IS NULL", sql)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	sql, err := build(t, `author is not null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// NOT(field IS NULL) — semantically IS NOT NULL.
	if !strings.Contains(sql, "NOT") || !strings.Contains(sql, "IS NULL") {
		t.Fatalf("sql=%q must wrap IS NULL in NOT", sql)
	}
}

func TestVisitor_CollectionMembership(t *testing.T) {
	sql, err := build(t, `visible_to has 'user-42'`)
	if err != nil {
		t.Fatal(err)
	}
	want := `ANY element IN metadata.` + "`visible_to`" + ` SATISFIES element = "user-42" END`
	if sql != want {
		t.Fatalf("sql = %q, want %q", sql, want)
	}
}
