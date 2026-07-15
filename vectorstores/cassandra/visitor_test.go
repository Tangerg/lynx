package cassandra_test

import (
	"math"
	"testing"

	"github.com/Tangerg/lynx/core/vectorstore/filter"
	"github.com/Tangerg/lynx/vectorstores/cassandra"
)

func TestVisitor_PreservesUnsignedIntegerList(t *testing.T) {
	visitor := cassandra.NewVisitor()
	if err := visitor.Visit(filter.In("id", []uint64{math.MaxUint64})); err != nil {
		t.Fatal(err)
	}
	_, args := visitor.Result()
	if len(args) != 1 {
		t.Fatalf("args = %#v", args)
	}
	values, ok := args[0].([]uint64)
	if !ok || len(values) != 1 || values[0] != math.MaxUint64 {
		t.Fatalf("IN argument = %#v (%T)", args[0], args[0])
	}
}
