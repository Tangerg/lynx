package weaviate_test

import (
	"testing"

	"github.com/weaviate/weaviate-go-client/v5/weaviate/filters"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/internal/storetest"
	"github.com/Tangerg/lynx/vectorstores/weaviate"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.Parse(src)
		if err != nil {
			return err
		}
		v := weaviate.NewVisitor()
		return v.Visit(expr)
	})
}

func TestVisitor_IsNull(t *testing.T) {
	expr, err := filter.Parse("author is null")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got, err := weaviate.ToFilter(expr)
	if err != nil {
		t.Fatalf("ToFilter: %v", err)
	}
	if got == nil {
		t.Fatal("expected a filter, got nil")
	}

	wf := got.Build()
	if wf.Operator != string(filters.IsNull) {
		t.Errorf("operator = %q, want %q", wf.Operator, filters.IsNull)
	}
	if len(wf.Path) != 1 || wf.Path[0] != "author" {
		t.Errorf("path = %v, want [author]", wf.Path)
	}
	if wf.ValueBoolean == nil || *wf.ValueBoolean != true {
		t.Errorf("valueBoolean = %v, want true", wf.ValueBoolean)
	}
}

func TestVisitor_IsNotNull(t *testing.T) {
	expr, err := filter.Parse("author is not null")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	got, err := weaviate.ToFilter(expr)
	if err != nil {
		t.Fatalf("ToFilter: %v", err)
	}
	if got == nil {
		t.Fatal("expected a filter, got nil")
	}

	// IS NOT NULL is rendered as NOT(author IS NULL): an outer Not operator
	// wrapping the IsNull operand produced by the IS NULL handler.
	wf := got.Build()
	if wf.Operator != string(filters.Not) {
		t.Fatalf("outer operator = %q, want %q", wf.Operator, filters.Not)
	}
	if len(wf.Operands) != 1 {
		t.Fatalf("operands = %d, want 1", len(wf.Operands))
	}

	inner := wf.Operands[0]
	if inner.Operator != string(filters.IsNull) {
		t.Errorf("inner operator = %q, want %q", inner.Operator, filters.IsNull)
	}
	if len(inner.Path) != 1 || inner.Path[0] != "author" {
		t.Errorf("inner path = %v, want [author]", inner.Path)
	}
	if inner.ValueBoolean == nil || *inner.ValueBoolean != true {
		t.Errorf("inner valueBoolean = %v, want true", inner.ValueBoolean)
	}
}

func TestVisitor_RejectsIntegerThatNumberFilterCannotRepresent(t *testing.T) {
	_, err := weaviate.ToFilter(filter.EQ("id", uint64(1<<53+1)))
	if err == nil {
		t.Fatal("Weaviate silently rounded a large integer")
	}
}
