package mongodb_test

import (
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/mongodb"
)

// TestVisitor_Conformance exercises every AST shape the filter DSL
// supports against the MongoDB visitor via the shared
// [storetest.VisitorConformance] suite. This is "no shape crashes"
// coverage; output equivalence stays in the per-test functions below.
func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.ParseAndAnalyze(src)
		if err != nil {
			return err
		}
		v := mongodb.NewVisitor("metadata")
		v.Visit(expr)
		return v.Error()
	})
}

// build is the test driver — parse src, visit, return (doc, err).
func build(t *testing.T, src string) (map[string]any, error) {
	t.Helper()
	expr, err := filter.ParseAndAnalyze(src)
	if err != nil {
		return nil, err
	}
	v := mongodb.NewVisitor("metadata")
	v.Visit(expr)
	if err := v.Error(); err != nil {
		return nil, err
	}
	return v.Result(), nil
}

func TestVisitor_IsNull(t *testing.T) {
	doc, err := build(t, `author is null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// {"metadata.author": {"$eq": nil}} — matches null or absent.
	inner, ok := doc["metadata.author"].(map[string]any)
	if !ok {
		t.Fatalf("doc=%v must key on metadata.author with a sub-document", doc)
	}
	val, present := inner["$eq"]
	if !present {
		t.Fatalf("doc=%v must contain $eq operator", doc)
	}
	if val != nil {
		t.Fatalf("IS NULL must match $eq: nil, got %v", val)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	doc, err := build(t, `author is not null`)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// NOT(author IS NULL) → {"$nor": [{"metadata.author": {"$eq": nil}}]}.
	nor, ok := doc["$nor"].([]any)
	if !ok {
		t.Fatalf("doc=%v must wrap IS NULL in $nor", doc)
	}
	if len(nor) != 1 {
		t.Fatalf("$nor must hold exactly one sub-filter, got %v", nor)
	}
	sub, ok := nor[0].(map[string]any)
	if !ok {
		t.Fatalf("$nor[0]=%v must be a sub-document", nor[0])
	}
	inner, ok := sub["metadata.author"].(map[string]any)
	if !ok {
		t.Fatalf("$nor[0]=%v must key on metadata.author", sub)
	}
	if inner["$eq"] != nil {
		t.Fatalf("negated IS NULL must wrap $eq: nil, got %v", inner["$eq"])
	}
}
