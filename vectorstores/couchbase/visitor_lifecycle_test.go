package couchbase_test

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/storetest"
	"github.com/Tangerg/scope/vectorstores/couchbase"
)

func TestVisitorLifecycle(t *testing.T) {
	t.Parallel()
	storetest.VisitorLifecycle(t, func() storetest.Compiler {
		visitor := couchbase.NewVisitor("metadata")
		return storetest.Compiler{Visit: visitor.Visit, Snapshot: func() any { return visitor.Result() }}
	})
}
