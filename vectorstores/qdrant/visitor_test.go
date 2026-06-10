package qdrant_test

import (
	"testing"

	qdrantclient "github.com/qdrant/go-client/qdrant"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/qdrant"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.ParseAndAnalyze(src)
		if err != nil {
			return err
		}
		v := qdrant.NewVisitor()
		return v.Visit(expr)
	})
}

func toFilter(t *testing.T, src string) *qdrantclient.Filter {
	t.Helper()
	expr, err := filter.ParseAndAnalyze(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	v := qdrant.NewVisitor()
	if err := v.Visit(expr); err != nil {
		t.Fatalf("visit %q: %v", src, err)
	}
	return v.Filter()
}

// isNullKey returns the key of an IsNull condition, or "" if cond is not one.
func isNullKey(cond *qdrantclient.Condition) string {
	in, ok := cond.GetConditionOneOf().(*qdrantclient.Condition_IsNull)
	if !ok || in.IsNull == nil {
		return ""
	}
	return in.IsNull.GetKey()
}

func TestVisitor_IsNull(t *testing.T) {
	f := toFilter(t, "author is null")

	if len(f.GetMust()) != 1 {
		t.Fatalf("expected 1 Must condition, got %d", len(f.GetMust()))
	}
	if len(f.GetMustNot()) != 0 {
		t.Fatalf("expected 0 MustNot conditions, got %d", len(f.GetMustNot()))
	}
	if key := isNullKey(f.GetMust()[0]); key != "author" {
		t.Fatalf("expected IsNull condition on key %q, got %q", "author", key)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	f := toFilter(t, "author is not null")

	// IS NOT NULL = NOT(author IS NULL): the IsNull condition is wrapped in
	// a nested filter under MustNot.
	if len(f.GetMustNot()) != 1 {
		t.Fatalf("expected 1 MustNot condition, got %d", len(f.GetMustNot()))
	}
	if len(f.GetMust()) != 0 {
		t.Fatalf("expected 0 Must conditions, got %d", len(f.GetMust()))
	}

	nested, ok := f.GetMustNot()[0].GetConditionOneOf().(*qdrantclient.Condition_Filter)
	if !ok {
		t.Fatalf("expected nested filter condition under MustNot, got %T",
			f.GetMustNot()[0].GetConditionOneOf())
	}
	inner := nested.Filter.GetMust()
	if len(inner) != 1 {
		t.Fatalf("expected 1 condition inside nested filter, got %d", len(inner))
	}
	if key := isNullKey(inner[0]); key != "author" {
		t.Fatalf("expected nested IsNull condition on key %q, got %q", "author", key)
	}
}
