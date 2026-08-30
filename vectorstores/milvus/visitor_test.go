package milvus_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
	"github.com/Tangerg/scope/core/vectorstore/storetest"
	"github.com/Tangerg/scope/vectorstores/milvus"
)

func TestVisitor_Conformance(t *testing.T) {
	storetest.VisitorConformance(t, func(src string) error {
		expr, err := filter.Parse(src)
		if err != nil {
			return err
		}
		v := milvus.NewVisitor()
		return expr.Accept(v)
	})
}

func TestVisitor_PreservesLargeIntegerText(t *testing.T) {
	visitor := milvus.NewVisitor()
	if err := filter.EQ("id", uint64(math.MaxUint64)).Accept(visitor); err != nil {
		t.Fatal(err)
	}
	actual := visitor.Result()
	if actual != "id == 18446744073709551615" {
		t.Fatalf("filter = %q", actual)
	}
}

func TestVisitor_HasUsesArrayContains(t *testing.T) {
	expr, err := filter.Parse(`tags has 'rag'`)
	if err != nil {
		t.Fatal(err)
	}
	v := milvus.NewVisitor()
	if err := expr.Accept(v); err != nil {
		t.Fatal(err)
	}
	if got := v.Result(); got != `ARRAY_CONTAINS(tags, "rag")` {
		t.Fatalf("Result() = %q", got)
	}
}

func TestVisitor_QuotesCompleteStringLiteral(t *testing.T) {
	value := "line one\nline two\\path\"quoted"
	visitor := milvus.NewVisitor()
	if err := filter.EQ("value", value).Accept(visitor); err != nil {
		t.Fatal(err)
	}
	if got, want := visitor.Result(), "value == "+strconv.Quote(value); got != want {
		t.Fatalf("Result() = %q, want %q", got, want)
	}
}
