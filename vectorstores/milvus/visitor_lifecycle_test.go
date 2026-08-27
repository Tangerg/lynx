package milvus_test

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/storetest"
	"github.com/Tangerg/scope/vectorstores/milvus"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := milvus.NewVisitor()
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
